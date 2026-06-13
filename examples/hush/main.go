// Command hush is the-beach's custom-layout + client-JS showcase: a gated,
// anonymous content board rendered as a gapless wall of square tiles. See
// docs/architecture/18-hush.md for the full plan.
//
// Run it:
//
//	go build -o bin/hush ./examples/hush
//	PORT=8080 ./bin/hush
//
// Then open http://localhost:8080/. Without DATABASE_URL the wall runs open
// and in-memory (text posts through the 512-character limited-markdown text
// path, image posts through local file storage); with it, both the wall and
// the signup gate — applications, accounts, RBAC, sessions — live in Postgres.
// Production media is env-driven too (media.go): R2_* swaps the file store
// for Cloudflare R2, CF_IMAGES_* routes image posts through Cloudflare
// Images. The /compose page hosts the WebGPU camera island, and Repute
// scores posting activity.
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
	"github.com/ThirdCoastInteractive/the-beach/ch"
	beachcfg "github.com/ThirdCoastInteractive/the-beach/config"
	"github.com/ThirdCoastInteractive/the-beach/mailer"
	"github.com/ThirdCoastInteractive/the-beach/session"
	"github.com/ThirdCoastInteractive/the-beach/storage"
	"github.com/ThirdCoastInteractive/the-beach/storage/cfimages"
	"github.com/ThirdCoastInteractive/the-beach/ui/specimen"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed static
var staticFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

// app holds the wired dependencies the handlers close over. With DATABASE_URL
// set, the wall and the signup gate — applications, accounts, roles, sessions —
// are Postgres-backed (pgStore); without it the app runs open and in-memory
// (memStore): anyone can post, there's no gate. files and images are the media
// wiring (media.go): Local or R2 behind the Store interface, plus the optional
// Cloudflare Images client image posts route through.
type app struct {
	cfg      config
	store    wall
	files    storage.Store
	images   *cfimages.Client // nil = image posts go through files
	rep      *reputation
	mail     mailer.Mailer
	authn    *auth.Authenticator
	sessions *session.Store
	pool     *pgxpool.Pool
	release  bool

	// The Cloudflare Images variant names (media.go sets them with the
	// client): the full-size one feed cards render and the square-crop thumb
	// the grid tiles use.
	imgVariant string
	imgThumb   string

	// The observation store (analytics.go). chConn is nil without a
	// CLICKHOUSE_DSN — analytics off — and events is then a no-op batcher.
	chConn ch.Conn
	events *ch.Batcher[event]

	// The invite graph (invitee → inviter), in-memory, hydrated from
	// hush_invites at boot — the chain echoes read it on every vote.
	chainMu sync.Mutex
	chains  map[string]string
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// The .env the README tells you to copy actually loads — from the cwd or
	// the example dir, whichever exists — and real env always wins over the
	// file. Without this, a configured DATABASE_URL silently never arrives and
	// the wall boots open.
	_ = beachcfg.LoadEnvFile(".env")
	_ = beachcfg.LoadEnvFile(filepath.Join("examples", "hush", ".env"))

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("hush: static sub-fs: %v", err)
	}

	a := &app{cfg: loadConfig(), release: os.Getenv("RELEASE") != ""}
	// Media wiring (media.go): R2 or a local temp dir behind storage.Store,
	// plus the optional Cloudflare Images client. With no R2_*/CF_IMAGES_* env
	// the wall runs exactly as before — local ephemeral files served at /files.
	if err := a.initMedia(); err != nil {
		log.Fatalf("hush: media: %v", err)
	}
	a.rep = newReputation(a.cfg)
	// The strongest transport the MAIL_/MAILGUN_/SMTP_ env supports; with none
	// of it set, a LogMailer — outbox sends print to this terminal.
	a.mail = mailer.New(mailer.ConfigFromEnv())

	cfg := beach.Config{
		Service: "hush",
		Release: a.release,
		Static:  sub,
		Logger:  logger,
	}

	// Optional Postgres: the wall persists and the signup gate (applications,
	// accounts, RBAC, sessions) switches on. Without it the wall runs open and
	// database-less — a DEV convenience only. In RELEASE the degrade paths are
	// fatal instead: a production wall that loses its database must die loudly,
	// never quietly reopen with no gate, no accounts, and anonymous posting.
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if a.release && dsn == "" {
		log.Fatal("hush: RELEASE requires DATABASE_URL — the wall must not run open in production")
	}
	if dsn != "" {
		pool, perr := openPool(ctx, dsn)
		if perr != nil {
			if a.release {
				log.Fatalf("hush: database connect failed: %v", perr)
			}
			logger.Error("hush: database connect failed; running database-less — THE WALL IS OPEN", "err", perr)
		} else {
			a.pool = pool
			if err := a.migrate(ctx); err != nil {
				if a.release {
					log.Fatalf("hush: migrate failed: %v", err)
				}
				logger.Error("hush: migrate failed; running database-less — THE WALL IS OPEN", "err", err)
				pool.Close()
				a.pool = nil
			} else if wallStore, werr := newPGStore(ctx, pool); werr != nil {
				if a.release {
					log.Fatalf("hush: wall init failed: %v", werr)
				}
				logger.Error("hush: wall init failed; running database-less — THE WALL IS OPEN", "err", werr)
				pool.Close()
				a.pool = nil
			} else {
				a.store = wallStore
				a.sessions = session.NewStore(pool, session.Config{CookieName: sessionCookie})
				a.authn = auth.NewAuthenticator(pool, a.sessions, auth.LockoutConfig{})
				cfg.Sessions = a.sessions
				cfg.Principals = a.principalResolver
				if err := a.seedAdmin(ctx); err != nil {
					logger.Error("hush: seed admin failed", "err", err)
				}
				// chains feed the replay's echoes, so hydrate them first
				if err := a.hydrateChains(ctx); err != nil {
					logger.Error("hush: hydrate invite chains failed", "err", err)
				}
				if err := a.replayReputation(ctx); err != nil {
					logger.Error("hush: reputation replay failed", "err", err)
				}
				logger.Info("hush: database mode (gate enabled)", "login", "admin / password", "openSlots", a.cfg.OpenSlots)
			}
		}
	}
	if a.store == nil {
		a.store = newMemStore()
	}

	// The outbox pump — transactional email leaves the process only here, and
	// only in database mode (the queue is a table). Boot's reputation replay
	// never enqueues, so a restart sends nothing twice.
	if a.pool != nil && a.mail != nil {
		go a.pumpOutbox(ctx)
	}

	// Optional ClickHouse: the observation store behind /admin/analytics. An
	// empty CLICKHOUSE_DSN returns a nil conn — analytics off; track() and the
	// batcher are nil-safe, so the wall runs identically either way. A bad DSN
	// or migration also degrades to off rather than killing the wall.
	if chDSN := os.Getenv("CLICKHOUSE_DSN"); chDSN != "" {
		if conn, cerr := ch.Connect(ctx, chDSN); cerr != nil {
			logger.Error("hush: clickhouse connect failed; analytics off", "err", cerr)
		} else {
			a.chConn = conn
		}
	}
	if err := ch.Migrate(ctx, a.chConn, chMigrations()); err != nil {
		logger.Error("hush: clickhouse migrate failed; analytics off", "err", err)
		a.chConn = nil
	}
	a.events = ch.NewBatcher[event](a.chConn, "hush_events", ch.Batch{})

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
	app.Page("/terms", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: termsPage(a.cfg.AdminEmail)}, nil
	})

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
	app.Action("/p/{id}/flag", a.flagPost, postGuard...)
	app.Raw(http.MethodPost, "/upload", a.uploadImage, postGuard...)

	// Admin: the application queue, plus the suite (admin.go) — the hub, the
	// moderation review, the everything browser, the member ledger, and the
	// one moderation action their forms post to.
	var adminGuard []beach.Guard
	if a.authn != nil {
		adminGuard = append(adminGuard, app.Can("admin"))
	}
	app.Page("/admin/queue", a.queuePage, adminGuard...)
	app.Raw(http.MethodPost, "/admin/decide", a.doDecide, adminGuard...)
	app.Page("/admin", a.adminHub, adminGuard...)
	app.Page("/admin/review", a.adminReview, adminGuard...)
	app.Page("/admin/posts", a.adminPosts, adminGuard...)
	app.Page("/admin/members", a.adminMembers, adminGuard...)
	app.Page("/admin/outbox", a.adminOutbox, adminGuard...)
	app.Raw(http.MethodPost, "/admin/moderate", a.doModerate, adminGuard...)

	// Admin analytics (analytics.go): the dashboard page plus its deferred
	// widget fragments — every route behind the same admin guard, since the
	// charts and tables show everything the wall hides. Registered only when
	// the gate exists at all: in database-less mode adminGuard would be empty,
	// and an unguarded IP firehose is not a degraded mode, it's a leak.
	if a.authn != nil {
		app.Page("/admin/analytics", a.analyticsPage, adminGuard...)
		app.Page("/admin/analytics/events", anlWidget("anl-events", a.anlEvents), adminGuard...)
		app.Page("/admin/analytics/posts", anlWidget("anl-posts", a.anlPosts), adminGuard...)
		app.Page("/admin/analytics/votes", anlWidget("anl-votes", a.anlVotes), adminGuard...)
		app.Page("/admin/analytics/countries", anlWidget("anl-countries", a.anlCountries), adminGuard...)
		app.Page("/admin/analytics/ips", anlWidget("anl-ips", a.anlIPs), adminGuard...)
		app.Page("/admin/analytics/recent", anlWidget("anl-recent", a.anlRecent), adminGuard...)
	}

	// Application + auth (plain form posts → 303).
	app.Raw(http.MethodPost, "/apply", a.doApply)
	app.Raw(http.MethodPost, "/login", a.doLogin)
	app.Raw(http.MethodGet, "/logout", a.logout)

	// Member email (verify.go): the verification link is public — it arrives
	// by mail, signed in or not; the set/resend forms check the session inside.
	app.Page("/email/verify/{token}", a.verifyEmailPage)
	app.Raw(http.MethodPost, "/email/set", a.doSetEmail)
	app.Raw(http.MethodPost, "/email/resend", a.doResendEmail)

	// The other doors (invites.go): capped open registration and invite
	// redemption — both create accounts directly; minting is member-gated.
	app.Raw(http.MethodPost, "/register", a.doRegister)
	app.Page("/invite/{code}", a.invitePage)
	app.Raw(http.MethodPost, "/invite/{code}", a.doRedeem)
	app.Raw(http.MethodPost, "/invites", a.doMint, postGuard...)

	// Only the local store needs an app route to serve its bytes — R2 URLs are
	// absolute, pointing at the bucket's public/custom domain (or presigned).
	if local, ok := a.files.(*storage.Local); ok {
		app.Raw(http.MethodGet, "/files/", http.StripPrefix("/files/", local).ServeHTTP)
	}
}

// compose serves the WebGPU camera page.
func (a *app) compose(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: composePage()}, nil
}

// profileView is the tier-gated profile. A profile below tier 1 is locked for
// everyone — its owner gets the nudge, strangers get nothing (a viewable
// sub-tier profile would leak standing on a wall that never prints it). Your
// own unlocked profile carries the invite panel and the email section.
func (a *app) profileView(c *beach.Ctx) (beach.View, error) {
	handle := c.Param("handle")
	me, _, _ := a.acct(c)
	score := a.rep.score(handle)
	tier := a.tierOf(score)
	if tier < 1 {
		if handle == me {
			return beach.View{Page: noticePage("your profile is locked.",
				"post things people like.")}, nil
		}
		return beach.View{Page: notFoundPage(), Status: http.StatusNotFound}, nil
	}
	var panel invitePanel
	var email emailSection
	if handle == me {
		panel = a.invitesFor(c.Context(), handle, tier)
		email = a.emailFor(c.Context(), handle)
	}
	return beach.View{Page: profilePage(handle, tier, a.store.countByAuthor(c.Context(), handle), panel, email, handle == me)}, nil
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
	p, ok := a.store.get(c.Context(), id)
	if !ok {
		return beach.View{Page: notFoundPage(), Status: http.StatusNotFound}, nil
	}
	newer, moreNewer := a.store.neighbors(c.Context(), id, true, feedSpan)
	older, moreOlder := a.store.neighbors(c.Context(), id, false, feedSpan)
	handle, canPost, admin := a.acct(c)
	a.track(c.R, "view_post", handle, id)
	myVote := a.store.voteOf(c.Context(), id, voterKey(handle))
	// the flag control's three states: absent on your own post, inert once
	// you've flagged, live otherwise — persisted, not just patch-deep
	flagState := ""
	if canPost && !(p.Author != "" && p.Author == handle) {
		flagState = "live"
		if a.store.flagOf(c.Context(), id, voterKey(handle)) {
			flagState = "done"
		}
	}
	return beach.View{
		Page: a.feedPage(newer, p, older, moreNewer, moreOlder, canPost, myVote, flagState, handle, admin, a.tierOf(a.rep.score(handle))),
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
		posts, more := a.store.neighbors(c.Context(), cursor, newer, feedChunk)
		return beach.View{
			Fragment: a.feedBatch(dir, posts, more),
			Target:   "feed-" + dir,
			Mode:     beach.PatchReplace,
		}, nil
	}
}

// homeNotice maps the wall's ?err code onto the composer's note line.
func homeNotice(code string) string {
	if code == "2257" {
		return "image posts need the § 2257 certification — check the box."
	}
	return ""
}

// home renders the wall on navigation and patches the grid on a Datastar request.
func (a *app) home(c *beach.Ctx) (beach.View, error) {
	posts := a.store.list(c.Context())
	handle, canPost, admin := a.acct(c)
	a.track(c.R, "view_wall", handle, 0)
	return beach.View{
		Page:     a.homePage(posts, handle, canPost, admin, a.tierOf(a.rep.score(handle)), a.openLeft(c.Context()), homeNotice(c.Query("err"))),
		Fragment: a.grid(posts),
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
	p, err := a.store.add(c.Context(), "text", bodyHTML, author, clientMeta(c.R))
	if err != nil {
		return nil, err
	}
	a.rep.emit("post", author)
	a.track(c.R, "post_text", author, p.ID)
	return beach.Patches{
		{Fragment: tile(p, a.postThumbURL(p)), Target: "hush-grid", Mode: beach.PatchPrepend, Signals: map[string]string{"body": ""}},
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
	p, ok := a.store.vote(c.Context(), id, voter, up)
	if ok {
		kind, chainKind, evKind := "like", "chain-like", "vote_up"
		if !up {
			kind, chainKind, evKind = "dislike", "chain-dislike", "vote_down"
		}
		a.rep.emit(kind, p.Author)
		// The chain echo: the author's inviter answers for them, at half weight.
		if inviter := a.inviterOf(p.Author); inviter != "" {
			a.rep.emit(chainKind, inviter)
		}
		a.track(c.R, evKind, handle, id)
	}
	// Outer morph: the fragment IS #vote-<id>, so replace the element by its id
	// (inner mode would nest #vote-<id> inside itself). voteOf (not the ok flag)
	// styles the control, so a repeat click still shows the original mark.
	return beach.Patches{{Fragment: reactions(p, true, a.store.voteOf(c.Context(), id, voter)), Target: "vote-" + c.Param("id"), Mode: beach.PatchOuter}}, nil
}

// flagPost records the viewer's flag — the wall's only report control. Your
// own post is a no-op (the wall doesn't self-report). The flagger's current
// tier rides the flag and the store applies the hiding rules: a FlagTier+
// flag hides alone, anyone else needs FlagQuorum distinct flags to agree. If
// this flag took the post down, the focused card collapses to the one-line
// notice; otherwise only the control goes quiet — no counts, no progress bar,
// a member never learns what their flag did.
func (a *app) flagPost(c *beach.Ctx) (beach.Patches, error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	idStr := c.Param("id")
	handle, _, _ := a.acct(c)
	p, ok := a.store.get(c.Context(), id)
	if !ok {
		// gone or already hidden — show what the wall now shows: nothing
		return beach.Patches{{Fragment: hushedCard(idStr), Target: "p" + idStr, Mode: beach.PatchOuter}}, nil
	}
	if p.Author != "" && p.Author == handle {
		return beach.Patches{}, nil
	}
	tier := a.tierOf(a.rep.score(handle))
	hidden, _ := a.store.flag(c.Context(), id, voterKey(handle), tier, a.cfg.FlagTier, a.cfg.FlagQuorum)
	a.track(c.R, "flag", handle, id)
	if hidden {
		return beach.Patches{{Fragment: hushedCard(idStr), Target: "p" + idStr, Mode: beach.PatchOuter}}, nil
	}
	return beach.Patches{{Fragment: flaggedControl(idStr), Target: "flag-" + idStr, Mode: beach.PatchOuter}}, nil
}

// uploadImage handles the plain multipart upload form (and the camera island's
// fetch, which posts the same field): validate it's an image, hand the bytes to
// storeImage (Cloudflare Images when wired, the file store otherwise — media.go),
// append an image post, and 303 back to the wall (a real navigation — no
// Datastar, CSP-clean).
func (a *app) uploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil { // 8 MiB cap
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// no certification, no image — 18 U.S.C. § 2257 rides every upload, the
	// wall form and the camera alike, and each assent leaves a row
	if !agreed(r, "cert2257") {
		http.Redirect(w, r, "/?err=2257", http.StatusSeeOther)
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

	mediaURL, cfID, err := a.storeImage(r.Context(), f, ct)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	author, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	a.recordAgreement(r.Context(), author, agree2257, clientMeta(r))
	p, err := a.store.addImage(r.Context(), mediaURL, cfID, author, clientMeta(r))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.rep.emit("post", author)
	a.track(r, "post_image", author, p.ID)
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
