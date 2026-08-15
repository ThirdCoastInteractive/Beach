package web

import (
	"net/http"
	"strconv"
	"strings"

	authx "github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/auth"
	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/content"
	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/store"

	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
)

// compose serves the WebGPU camera page.
func (a *App) compose(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: composePage()}, nil
}

// studioNotice maps /studio?err onto the status line (the page itself is
// static; the island reads nothing — we just land them back on the canvas).
func studioNotice(code string) string {
	switch code {
	case "2257":
		return "image posts need the § 2257 certification — check the box."
	case "1":
		return "couldn't publish that layout — check the images are uploaded and try again."
	default:
		return ""
	}
}

// profileView is the tier-gated profile. A profile below tier 1 is locked for
// everyone — its owner gets the nudge, strangers get nothing (a viewable
// sub-tier profile would leak standing on a wall that never prints it). Your
// own unlocked profile carries the invite panel and the email section.
func (a *App) profileView(c *beach.Ctx) (beach.View, error) {
	handle := c.Param("handle")
	me, _, _ := a.acct(c)
	score := a.rep.Score(handle)
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
	return beach.View{Page: profilePage(handle, tier, a.store.CountByAuthor(c.Context(), handle), panel, email, handle == me)}, nil
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
func (a *App) postView(c *beach.Ctx) (beach.View, error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	p, ok := a.store.Get(c.Context(), id)
	if !ok {
		return beach.View{Page: notFoundPage(), Status: http.StatusNotFound}, nil
	}
	newer, moreNewer := a.store.Neighbors(c.Context(), id, true, feedSpan)
	older, moreOlder := a.store.Neighbors(c.Context(), id, false, feedSpan)
	handle, canPost, admin := a.acct(c)
	a.anl.Track(c.R, "view_post", handle, id)
	myVote := a.store.VoteOf(c.Context(), id, voterKey(handle))
	// the flag control's three states: absent on your own post, inert once
	// you've flagged, live otherwise — persisted, not just patch-deep
	flagState := ""
	if canPost && !(p.Author != "" && p.Author == handle) {
		flagState = "live"
		if a.store.FlagOf(c.Context(), id, voterKey(handle)) {
			flagState = "done"
		}
	}
	return beach.View{
		Page: a.feedPage(newer, p, older, moreNewer, moreOlder, canPost, myVote, flagState, handle, admin, a.tierOf(a.rep.Score(handle))),
	}, nil
}

// voterKey is the one-reaction-per-person key: the member's handle, or "anon"
// for a not-signed-in viewer.
func voterKey(handle string) string {
	if handle == "" {
		return "anon"
	}
	return handle
}

// feedMore returns the sentinel handler for one feed direction: read the
// cursor, fetch the next chunk past it, and replace the sentinel with the
// batch (which carries a fresh sentinel while more remain).
func (a *App) feedMore(newer bool) beach.PageFunc {
	dir := "older"
	if newer {
		dir = "newer"
	}
	return func(c *beach.Ctx) (beach.View, error) {
		cursor, _ := strconv.ParseInt(c.R.URL.Query().Get("cursor"), 10, 64)
		posts, more := a.store.Neighbors(c.Context(), cursor, newer, feedChunk)
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
func (a *App) home(c *beach.Ctx) (beach.View, error) {
	posts := a.store.List(c.Context())
	handle, canPost, admin := a.acct(c)
	a.anl.Track(c.R, "view_wall", handle, 0)
	return beach.View{
		Page:     a.homePage(posts, handle, canPost, admin, a.tierOf(a.rep.Score(handle)), a.openLeft(c.Context()), homeNotice(c.Query("err"))),
		Fragment: a.board(posts, canPost),
		Target:   "hush-board",
	}, nil
}

// createPost binds the `body` signal, renders + length-checks it through the text
// path, appends the post, and prepends the new tile to the wall (clearing the
// composer). An unpostable body patches the status line instead.
func (a *App) createPost(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		Body string `json:"body"`
	}](c)
	if err != nil {
		return nil, err
	}

	bodyHTML, ok := content.RenderPost(in.Body)
	if !ok {
		msg := "say something first."
		if strings.TrimSpace(in.Body) != "" {
			msg = "too long — 512 characters max (formatting doesn't count)."
		}
		return beach.Patches{{Fragment: note(msg), Target: "hush-note", Mode: beach.PatchInner}}, nil
	}

	author, _, _ := a.acct(c)
	p, err := a.store.Add(c.Context(), "text", bodyHTML, author, store.ClientMeta(c.R))
	if err != nil {
		return nil, err
	}
	a.rep.Emit("post", author)
	a.anl.Track(c.R, "post_text", author, p.ID)
	return beach.Patches{
		{Fragment: a.print(p), Target: "hush-board", Mode: beach.PatchPrepend, Signals: map[string]string{"body": ""}},
		{Fragment: note(""), Target: "hush-note", Mode: beach.PatchInner},
	}, nil
}

// votePost records a like or dislike (?dir=up|down) and credits — or dings —
// the post's author with a Repute "like"/"dislike" event, then patches the
// reaction control: no counts come back, just your own mark inverted. One
// reaction per user, final.
func (a *App) votePost(c *beach.Ctx) (beach.Patches, error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	handle, _, _ := a.acct(c)
	voter := voterKey(handle)
	up := c.R.URL.Query().Get("dir") != "down"
	p, ok := a.store.Vote(c.Context(), id, voter, up)
	if ok {
		kind, chainKind, evKind := "like", "chain-like", "vote_up"
		if !up {
			kind, chainKind, evKind = "dislike", "chain-dislike", "vote_down"
		}
		a.rep.Emit(kind, p.Author)
		// The chain echo: the author's inviter answers for them, at half weight.
		if inviter := a.inviterOf(p.Author); inviter != "" {
			a.rep.Emit(chainKind, inviter)
		}
		a.anl.Track(c.R, evKind, handle, id)
	}
	// Outer morph: the fragment IS #vote-<id>, so replace the element by its id
	// (inner mode would nest #vote-<id> inside itself). VoteOf (not the ok flag)
	// styles the control, so a repeat click still shows the original mark.
	return beach.Patches{{Fragment: reactions(p, true, a.store.VoteOf(c.Context(), id, voter)), Target: "vote-" + c.Param("id"), Mode: beach.PatchOuter}}, nil
}

// flagPost records the viewer's flag — the wall's only report control. Your
// own post is a no-op (the wall doesn't self-report). The flagger's current
// tier rides the flag and the store applies the hiding rules: a FlagTier+
// flag hides alone, anyone else needs FlagQuorum distinct flags to agree. If
// this flag took the post down, the focused card collapses to the one-line
// notice; otherwise only the control goes quiet — no counts, no progress bar,
// a member never learns what their flag did.
func (a *App) flagPost(c *beach.Ctx) (beach.Patches, error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	idStr := c.Param("id")
	handle, _, _ := a.acct(c)
	p, ok := a.store.Get(c.Context(), id)
	if !ok {
		// gone or already hidden — show what the wall now shows: nothing
		return beach.Patches{{Fragment: hushedCard(idStr), Target: "p" + idStr, Mode: beach.PatchOuter}}, nil
	}
	if p.Author != "" && p.Author == handle {
		return beach.Patches{}, nil
	}
	tier := a.tierOf(a.rep.Score(handle))
	pol := a.live()
	hidden, _ := a.store.Flag(c.Context(), id, voterKey(handle), tier, pol.FlagTier, pol.FlagQuorum)
	a.anl.Track(c.R, "flag", handle, id)
	if hidden {
		return beach.Patches{{Fragment: hushedCard(idStr), Target: "p" + idStr, Mode: beach.PatchOuter}}, nil
	}
	return beach.Patches{{Fragment: flaggedControl(idStr), Target: "flag-" + idStr, Mode: beach.PatchOuter}}, nil
}

// uploadImage handles the plain multipart upload form (and the camera island's
// fetch, which posts the same field): validate it's an image, hand the bytes to
// the media layer (Cloudflare Images), append an image post, and 303 back to the
// wall (a real navigation — no Datastar, CSP-clean).
func (a *App) uploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil { // 8 MiB cap
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// no certification, no image — 18 U.S.C. § 2257 rides every upload, the
	// wall form and the camera alike, and each assent leaves a row
	if !authx.Agreed(r, "cert2257") {
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

	mediaURL, cfID, err := a.media.StoreImage(r.Context(), f, ct)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	author, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	a.auth.RecordAgreement(r.Context(), author, authx.Agree2257, store.ClientMeta(r))
	p, err := a.store.AddImage(r.Context(), mediaURL, cfID, author, store.ClientMeta(r))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.rep.Emit("post", author)
	a.anl.Track(r, "post_image", author, p.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
