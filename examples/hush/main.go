// Command hush is the-beach's custom-layout + client-JS showcase: a gated,
// anonymous content board rendered as a gapless wall of square tiles. See
// docs/architecture/18-hush.md for the full plan.
//
// Run it:
//
//	go build -o bin/hush ./examples/hush
//	PORT=8080 ./bin/hush
//
// Then open http://localhost:8080/. The wall itself is in-memory (text posts
// through the 512-character limited-markdown text path, image posts through
// local file storage); the signup gate — applications, accounts, RBAC,
// sessions — needs Postgres and switches on with DATABASE_URL. The /compose
// page hosts the WebGPU camera island, and Repute scores posting activity.
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/auth"
	"github.com/ThirdCoastInteractive/the-beach/session"
	"github.com/ThirdCoastInteractive/the-beach/storage"
	"github.com/ThirdCoastInteractive/the-beach/ui/specimen"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed static
var staticFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

// app holds the wired dependencies the handlers close over. The post wall stays
// in-memory (like pantry's domain data); the signup gate — applications, accounts,
// roles, sessions — is Postgres-backed when DATABASE_URL is set. Without a DB the
// app runs open: anyone can post, there's no gate.
type app struct {
	store    *store
	files    *storage.Local
	rep      *reputation
	authn    *auth.Authenticator
	sessions *session.Store
	pool     *pgxpool.Pool
	release  bool

	// The signup modes beyond the queue (see invites.go). openSlots is
	// HUSH_OPEN_SLOTS; chains is the in-memory invite graph (invitee → inviter),
	// hydrated from hush_invites at boot.
	openSlots int
	chainMu   sync.Mutex
	chains    map[string]string
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("hush: static sub-fs: %v", err)
	}

	// Local file store under the OS temp dir, served at /files. Ephemeral to match
	// the in-memory posts; Block D swaps in a persistent backend alongside Postgres.
	filesDir := os.Getenv("HUSH_FILES_DIR")
	if filesDir == "" {
		filesDir = filepath.Join(os.TempDir(), "hush-files")
	}
	files, err := storage.NewLocal(filesDir, "/files")
	if err != nil {
		log.Fatalf("hush: file store: %v", err)
	}

	a := &app{store: newStore(), files: files, rep: newReputation(), release: os.Getenv("RELEASE") != ""}
	// HUSH_OPEN_SLOTS=N opens the first N registrations with no review — the
	// switch-on-later mode. Unset/0 keeps the application queue as the only door
	// (besides invites).
	a.openSlots, _ = strconv.Atoi(os.Getenv("HUSH_OPEN_SLOTS"))

	cfg := beach.Config{
		Service: "hush",
		Release: a.release,
		Static:  sub,
		Logger:  logger,
	}

	// Optional Postgres: enables the signup gate (applications, accounts, RBAC,
	// sessions). Without it the wall runs open and database-less.
	ctx := context.Background()
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pool, perr := openPool(ctx, dsn)
		if perr != nil {
			logger.Error("hush: database connect failed; running database-less", "err", perr)
		} else {
			a.pool = pool
			if err := a.migrate(ctx); err != nil {
				logger.Error("hush: migrate failed; running database-less", "err", err)
				pool.Close()
				a.pool = nil
			} else {
				a.sessions = session.NewStore(pool, session.Config{CookieName: sessionCookie})
				a.authn = auth.NewAuthenticator(pool, a.sessions, auth.LockoutConfig{})
				cfg.Sessions = a.sessions
				cfg.Principals = a.principalResolver
				if err := a.seedAdmin(ctx); err != nil {
					logger.Error("hush: seed admin failed", "err", err)
				}
				if err := a.hydrateChains(ctx); err != nil {
					logger.Error("hush: hydrate invite chains failed", "err", err)
				}
				logger.Info("hush: database mode (gate enabled)", "login", "admin / password", "openSlots", a.openSlots)
			}
		}
	}

	beachApp := beach.New(cfg)
	a.routes(beachApp)

	logger.Info("hush: starting", "addr", addr, "db", a.pool != nil)
	if err := beachApp.Start(addr); err != nil {
		log.Fatalf("hush: %v", err)
	}
}

// routes registers the wall, the post action, image upload, and file serving.
// The gate, camera, and scoring routes are added in their respective blocks.
func (a *app) routes(app *beach.App) {
	app.Page("/", a.home)
	app.Page("/compose", a.compose)
	app.Page("/p/{id}", a.postView)
	// Datastar-only feed batches, fetched by the intersect sentinels; a direct
	// navigation 404s (no Page on the View).
	app.Page("/feed/newer", a.feedMore(true))
	app.Page("/feed/older", a.feedMore(false))
	app.Page("/apply", a.applyPage)
	app.Page("/login", a.loginPage)
	app.Page("/u/{handle}", a.profileView)

	// The framework's living showcase — every driftwood component and chart
	// type with sample data — mounted so a hush checkout doubles as a styling
	// reference.
	app.Page("/specimen", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: specimen.Page()}, nil
	})

	// Posting is gated on the "post" permission when the gate (DB) is on; without
	// a DB there's no principal and the wall is open.
	var postGuard []beach.Guard
	if a.authn != nil {
		postGuard = append(postGuard, app.Can("post"))
	}
	app.Action("/posts", a.createPost, postGuard...)
	app.Action("/p/{id}/vote", a.votePost, postGuard...)
	app.Raw(http.MethodPost, "/upload", a.uploadImage, postGuard...)

	// Admin queue: approve/reject applications.
	var adminGuard []beach.Guard
	if a.authn != nil {
		adminGuard = append(adminGuard, app.Can("admin"))
	}
	app.Page("/admin/queue", a.queuePage, adminGuard...)
	app.Raw(http.MethodPost, "/admin/decide", a.doDecide, adminGuard...)

	// Application + auth (plain form posts → 303).
	app.Raw(http.MethodPost, "/apply", a.doApply)
	app.Raw(http.MethodPost, "/login", a.doLogin)
	app.Raw(http.MethodGet, "/logout", a.logout)

	// The other doors (invites.go): capped open registration and invite
	// redemption — both create accounts directly; minting is member-gated.
	app.Raw(http.MethodPost, "/register", a.doRegister)
	app.Page("/invite/{code}", a.invitePage)
	app.Raw(http.MethodPost, "/invite/{code}", a.doRedeem)
	app.Raw(http.MethodPost, "/invites", a.doMint, postGuard...)

	app.Raw(http.MethodGet, "/files/", http.StripPrefix("/files/", a.files).ServeHTTP)
}

// compose serves the WebGPU camera page.
func (a *app) compose(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: composePage()}, nil
}

// profileView is the tier-gated profile. Your own profile unlocks at tier 1;
// below that it shows a locked notice (the Repute tier doing real gating
// work). Your own unlocked profile also carries the invite panel.
func (a *app) profileView(c *beach.Ctx) (beach.View, error) {
	handle := c.Param("handle")
	me, _, _ := a.acct(c)
	score := a.rep.score(handle)
	tier := tierOf(score)
	if handle == me && tier < 1 {
		return beach.View{Page: noticePage("your profile is locked.",
			"post things people like.")}, nil
	}
	var panel invitePanel
	if handle == me {
		panel = a.invitesFor(c.Context(), handle, tier)
	}
	return beach.View{Page: profilePage(handle, tier, a.store.countByAuthor(handle), panel)}, nil
}

// The feed renders feedSpan neighbors each way up front — enough that the top
// sentinel starts offscreen — and each sentinel pulls feedChunk more.
const (
	feedSpan  = 2
	feedChunk = 4
)

// postView is a post's page: the focused post expanded, its nearest neighbors
// around it, and intersect sentinels where the timeline continues. Opening a
// post is a real navigation — the browser owns the history stack, and the
// cross-document View Transition morphs the clicked tile into the focused
// card. The link's #p{id} fragment lands the viewport on the post even with
// newer neighbors rendered above it.
func (a *app) postView(c *beach.Ctx) (beach.View, error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	p, ok := a.store.get(id)
	if !ok {
		return beach.View{Page: notFoundPage(), Status: http.StatusNotFound}, nil
	}
	newer, moreNewer := a.store.neighbors(id, true, feedSpan)
	older, moreOlder := a.store.neighbors(id, false, feedSpan)
	handle, canPost, admin := a.acct(c)
	myVote := a.store.voteOf(id, voterKey(handle))
	return beach.View{
		Page: feedPage(newer, p, older, moreNewer, moreOlder, canPost, myVote, handle, admin, tierOf(a.rep.score(handle))),
	}, nil
}

// voterKey is the one-reaction-per-person key: the handle, or "anon" in
// database-less open mode (a single-user wall).
func voterKey(handle string) string {
	if handle == "" {
		return "anon"
	}
	return handle
}

// feedMore returns the sentinel handler for one feed direction: read the
// cursor, fetch the next chunk past it, and replace the sentinel with the
// batch (which carries a fresh sentinel while more remain).
func (a *app) feedMore(newer bool) beach.PageFunc {
	dir := "older"
	if newer {
		dir = "newer"
	}
	return func(c *beach.Ctx) (beach.View, error) {
		cursor, _ := strconv.ParseInt(c.R.URL.Query().Get("cursor"), 10, 64)
		posts, more := a.store.neighbors(cursor, newer, feedChunk)
		return beach.View{
			Fragment: feedBatch(dir, posts, more),
			Target:   "feed-" + dir,
			Mode:     beach.PatchReplace,
		}, nil
	}
}

// home renders the wall on navigation and patches the grid on a Datastar request.
func (a *app) home(c *beach.Ctx) (beach.View, error) {
	posts := a.store.list()
	handle, canPost, admin := a.acct(c)
	return beach.View{
		Page:     homePage(posts, handle, canPost, admin, tierOf(a.rep.score(handle)), a.openLeft(c.Context())),
		Fragment: grid(posts),
		Target:   "hush-grid",
	}, nil
}

// createPost binds the `body` signal, renders + length-checks it through the text
// path, appends the post, and prepends the new tile to the wall (clearing the
// composer). An unpostable body patches the status line instead.
func (a *app) createPost(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		Body string `json:"body"`
	}](c)
	if err != nil {
		return nil, err
	}

	bodyHTML, ok := renderPost(in.Body)
	if !ok {
		msg := "say something first."
		if strings.TrimSpace(in.Body) != "" {
			msg = "too long — 512 characters max (formatting doesn't count)."
		}
		return beach.Patches{{Fragment: note(msg), Target: "hush-note", Mode: beach.PatchInner}}, nil
	}

	author, _, _ := a.acct(c)
	p := a.store.add("text", bodyHTML, author)
	a.rep.emit("post", author)
	return beach.Patches{
		{Fragment: tile(p), Target: "hush-grid", Mode: beach.PatchPrepend, Signals: map[string]string{"body": ""}},
		{Fragment: note(""), Target: "hush-note", Mode: beach.PatchInner},
	}, nil
}

// votePost records a like or dislike (?dir=up|down) and credits — or dings —
// the post's author with a Repute "like"/"dislike" event, then patches the
// reaction control: no counts come back, just your own mark inverted. One
// reaction per user, final.
func (a *app) votePost(c *beach.Ctx) (beach.Patches, error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	handle, _, _ := a.acct(c)
	voter := voterKey(handle)
	up := c.R.URL.Query().Get("dir") != "down"
	p, ok := a.store.vote(id, voter, up)
	if ok {
		kind, chainKind := "like", "chain-like"
		if !up {
			kind, chainKind = "dislike", "chain-dislike"
		}
		a.rep.emit(kind, p.Author)
		// The chain echo: the author's inviter answers for them, at half weight.
		if inviter := a.inviterOf(p.Author); inviter != "" {
			a.rep.emit(chainKind, inviter)
		}
	}
	// Outer morph: the fragment IS #vote-<id>, so replace the element by its id
	// (inner mode would nest #vote-<id> inside itself). voteOf (not the ok flag)
	// styles the control, so a repeat click still shows the original mark.
	return beach.Patches{{Fragment: reactions(p, true, a.store.voteOf(id, voter)), Target: "vote-" + c.Param("id"), Mode: beach.PatchOuter}}, nil
}

// uploadImage handles the plain multipart upload form: validate it's an image,
// Put the bytes into the file store under a random key, append an image post, and
// 303 back to the wall (a real navigation — no Datastar, CSP-clean). The camera
// island in Block C adds the fetch-based prepend path against this same store.
func (a *app) uploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil { // 8 MiB cap
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	f, hdr, err := r.FormFile("image")
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	defer f.Close()

	ct := hdr.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	key := "posts/" + randHex() + extFor(ct)
	if _, err := a.files.Put(r.Context(), key, f, ct); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	url, _ := a.files.URL(r.Context(), key)
	author, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	a.store.addImage(url, author)
	a.rep.emit("post", author)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// randHex returns 8 random bytes as hex, a collision-free object key stem.
func randHex() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// extFor maps an image content type to a file extension.
func extFor(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}
