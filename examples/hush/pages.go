package main

import (
	"context"
	"html"
	"io"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// shell renders the full hush document. hush is the custom-layout example, so it
// ships its own page shell rather than the driftwood one: it links the kit's
// stylesheet (so dw-* surfaces still resolve) AND its own hush.css, keeps the
// theme class on <html> for no pop-in, and defers Datastar per the no-blocking
// law. No inline script — the strict CSP forbids it.
func shell(title string, body ...templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		head := `<!DOCTYPE html><html lang="en" class="dw-root"><head>` +
			`<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">` +
			`<title>` + html.EscapeString(title) + `</title>` +
			`<link rel="stylesheet" href="/static/css/app.css">` +
			`<link rel="stylesheet" href="/static/css/hush.css">` +
			`<script type="module" src="/static/js/datastar.js" defer></script>` +
			`</head><body class="dw-body hush-body">`
		if _, err := io.WriteString(w, head); err != nil {
			return err
		}
		for _, c := range body {
			if c == nil {
				continue
			}
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</body></html>`)
		return err
	})
}

// homePage is the wall: a centered header + composer, the full-bleed gapless grid,
// and the slide-in detail drawer. The whole page shares two Datastar signals:
// `open` drives the drawer/backdrop slide (pure CSS transition bound via
// data-class), `reply` backs the comment box. Clicking a tile sets open, resets
// reply, and fetches the card into the drawer over SSE.
func homePage(posts []Post, handle string, canPost, admin bool, score float64, tier int) templ.Component {
	var b strings.Builder
	b.WriteString(`<div data-signals="{reply: ''}">`)
	b.WriteString(accountStrip(handle, admin, score, tier))
	b.WriteString(`<div class="hush-top">`)
	b.WriteString(`<header class="hush-head"><h1 class="hush-title">hush</h1>`)
	b.WriteString(`<p class="hush-sub">a quiet wall — post into it.</p></header>`)
	if canPost {
		b.WriteString(composerHTML())
	} else {
		// Logged-out, DB mode: the wall is public to read, invite-only to post.
		b.WriteString(`<p class="hush-sub hush-gateline">posting is invite-only — ` +
			`<a class="hush-link" href="/apply">apply</a> or <a class="hush-link" href="/login">sign in</a>.</p>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(gridHTML(posts))
	// The sheet host. Clicking a tile streams a sheet into it inside a View
	// Transition, so it slides in from the edge over the (undimmed) wall. No
	// backdrop — the sheet itself sits on top.
	b.WriteString(`<div id="hush-sheet"></div>`)
	b.WriteString(`</div>`)
	return shell("hush", templ.Raw(b.String()))
}

// accountStrip is the top-right account line: who you are, the admin queue link,
// and log out. Empty when signed out (the gate line carries the call to action)
// or in database-less open mode.
func accountStrip(handle string, admin bool, score float64, tier int) string {
	if handle == "" {
		return ""
	}
	s := `<div class="hush-acct">you are <strong>` + html.EscapeString(handle) + `</strong>` +
		` · <span class="hush-score">score ` + strconv.Itoa(int(score+0.5)) + ` · tier ` + strconv.Itoa(tier) + `</span>`
	if tier >= 1 {
		s += ` · <a class="hush-link" href="/u/` + html.EscapeString(handle) + `">profile</a>`
	}
	if admin {
		s += ` · <a class="hush-link" href="/admin/queue">queue</a>`
	}
	return s + ` · <a class="hush-link" href="/logout">log out</a></div>`
}

// voteControl is the upvote affordance in a post card. With canPost the arrow is
// a live @post button; otherwise it's a read-only count.
func voteControl(id int64, count int, canPost bool) templ.Component {
	return templ.Raw(voteControlHTML(id, count, canPost))
}

func voteControlHTML(id int64, count int, canPost bool) string {
	idStr := strconv.FormatInt(id, 10)
	inner := ""
	if canPost {
		inner = `<button class="hush-vote-btn" data-on:click="@post('/p/` + idStr + `/vote')">▲</button>` +
			`<span class="hush-vote-count">` + strconv.Itoa(count) + `</span>`
	} else {
		inner = `<span class="hush-vote-btn hush-vote-btn--off">▲</span>` +
			`<span class="hush-vote-count">` + strconv.Itoa(count) + `</span>`
	}
	return `<div id="vote-` + idStr + `" class="hush-vote">` + inner + `</div>`
}

// profilePage is the tier-1 reward: a member's standing and post count. The wall
// stays anonymous, so this is the one place a handle and score are shown together.
func profilePage(handle string, score float64, tier, posts int) templ.Component {
	body := `<div class="hush-top"><header class="hush-head"><h1 class="hush-title">` +
		html.EscapeString(handle) + `</h1><p class="hush-sub">tier ` + strconv.Itoa(tier) + `</p></header>` +
		`<div class="hush-profile">` +
		`<div class="hush-stat"><span class="hush-stat-n">` + strconv.Itoa(int(score+0.5)) + `</span> score</div>` +
		`<div class="hush-stat"><span class="hush-stat-n">` + strconv.Itoa(posts) + `</span> posts</div>` +
		`</div>`
	if tier < 2 {
		body += `<p class="hush-sub">reach tier 2 (score ` + strconv.Itoa(int(tier2Score)) + `) to unlock DMs.</p>`
	} else {
		body += `<p class="hush-sub">DMs unlocked.</p>`
	}
	body += `<p class="hush-back"><a class="hush-link" href="/">← back to the wall</a></p></div>`
	return shell("hush — "+handle, templ.Raw(body))
}

// noticePage is a centered one-line message page (e.g. "application submitted").
func noticePage(title, sub string) templ.Component {
	body := `<div class="hush-top"><header class="hush-head"><h1 class="hush-title">hush</h1>` +
		`<p class="hush-sub">` + html.EscapeString(title) + `</p>`
	if sub != "" {
		body += `<p class="hush-sub">` + html.EscapeString(sub) + `</p>`
	}
	body += `</header><p class="hush-back"><a class="hush-link" href="/">← back to the wall</a></p></div>`
	return shell("hush", templ.Raw(body))
}

// formPage centers a form (login, apply) under the hush header.
func formPage(title string, form templ.Component) templ.Component {
	head := templ.Raw(`<div class="hush-top"><header class="hush-head"><h1 class="hush-title">hush</h1>` +
		`<p class="hush-sub">` + html.EscapeString(title) + `</p></header><div class="hush-formwrap">`)
	tail := templ.Raw(`</div><p class="hush-back"><a class="hush-link" href="/">← back to the wall</a></p></div>`)
	return shell("hush — "+title, head, form, tail)
}

// composePage hosts the WebGPU camera island. It is the dedicated client-JS page
// — a canvas the camera renders into through a WGSL filter pipeline, and a capture
// button that posts the filtered still. The script is an external module (CSP:
// script-src 'self'); the page degrades to the wall's plain upload without WebGPU.
func composePage() templ.Component {
	body := `<div class="hush-top">` +
		`<header class="hush-head"><h1 class="hush-title">hush</h1>` +
		`<p class="hush-sub">capture something — filtered, on-device.</p></header>` +
		`<div class="hush-cam">` +
		`<canvas id="cam-canvas" class="cam-canvas" width="720" height="720"></canvas>` +
		`<div class="cam-controls">` +
		`<select id="cam-filter" class="cam-filter" aria-label="filter">` +
		`<option value="none">none</option>` +
		`<option value="noir">noir</option>` +
		`<option value="bleach">bleach</option>` +
		`<option value="dream">dream</option>` +
		`</select>` +
		`<button id="cam-shoot" class="hush-post" type="button">capture</button>` +
		`</div>` +
		`<p id="cam-status" class="cam-status" aria-live="polite"></p>` +
		`</div>` +
		`<p class="hush-back"><a class="hush-link" href="/">← back to the wall</a></p>` +
		`</div>`
	return shell("hush — capture",
		templ.Raw(body),
		templ.Raw(`<script type="module" src="/static/js/camera.js"></script>`),
	)
}

// composerHTML is the post box: a textarea bound to the `body` signal and a post
// button that fires @post('/posts'). No <form> — a bare button avoids needing a
// submit-prevent modifier, and Datastar carries the signal as JSON.
func composerHTML() string {
	return `<div class="hush-compose">` +
		`<textarea id="hush-body" name="body" class="hush-input" data-bind:body ` +
		`aria-label="compose a post" ` +
		`placeholder="say something — **bold**, *italics*, - lists"></textarea>` +
		`<div class="hush-compose-row">` +
		`<span class="hush-hint">bold · italics · lists · 512 characters</span>` +
		`<button class="hush-post" data-on:click="@post('/posts')">post</button>` +
		`</div>` +
		`<div id="hush-note" class="hush-note" aria-live="polite"></div>` +
		// Image upload is a plain multipart form (303 back to the wall) — no
		// Datastar, no inline script, CSP-clean. The camera island (Block C) adds
		// the fetch-based prepend path.
		`<form class="hush-upload" method="post" action="/upload" enctype="multipart/form-data">` +
		`<input type="file" id="hush-image" name="image" accept="image/*" class="hush-file">` +
		`<button class="hush-post hush-post--ghost" type="submit">upload image</button>` +
		`<a class="hush-link hush-cam-link" href="/compose">or open the camera →</a>` +
		`</form>` +
		`</div>`
}

// gridHTML is the full #hush-grid container; gridFragment wraps it as a component
// for the page's Datastar branch.
func gridHTML(posts []Post) string {
	var b strings.Builder
	b.WriteString(`<div id="hush-grid" class="hush-grid">`)
	for _, p := range posts {
		b.WriteString(tileHTML(p))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func gridFragment(posts []Post) templ.Component { return templ.Raw(gridHTML(posts)) }

// tileHTML is one square. Clicking it opens the drawer and streams the post's
// detail card in. A text tile's BodyHTML is already sanitized by renderPost; an
// image tile renders the stored object's URL (escaped).
func tileHTML(p Post) string {
	id := strconv.FormatInt(p.ID, 10)
	// Clear any stale reply, then stream the sheet in (the View Transition does
	// the slide; see /p/{id} → ViewTransition).
	open := `data-on:click="$reply = ''; @get('/p/` + id + `')" role="button" tabindex="0"`
	style := ` style="background:` + colorFor(p) + `"`
	if p.Kind == "image" {
		return `<div class="hush-tile hush-tile--image"` + style + ` ` + open + `>` +
			`<img src="` + html.EscapeString(p.ImageURL) + `" alt="" loading="lazy"></div>`
	}
	return `<div class="hush-tile hush-tile--text"` + style + ` ` + open + `>` +
		`<div class="hush-tile-body">` + p.BodyHTML + `</div></div>`
}

func tile(p Post) templ.Component { return templ.Raw(tileHTML(p)) }

// cardInnerHTML is the shared body of a post's detail view — the media, meta,
// the comment thread, and the reply box. Both the slide-in drawer and the
// permalink page wrap this.
func cardInnerHTML(p Post, comments []Comment, canPost bool) string {
	id := strconv.FormatInt(p.ID, 10)
	var b strings.Builder

	if p.Kind == "image" {
		b.WriteString(`<div class="hush-card-media hush-card-media--image">`)
		b.WriteString(`<img src="` + html.EscapeString(p.ImageURL) + `" alt="">`)
	} else {
		// The card's hero uses the post's own computed color.
		b.WriteString(`<div class="hush-card-media hush-card-media--text" style="background:` + colorFor(p) + `">`)
		b.WriteString(`<div class="hush-card-text">` + p.BodyHTML + `</div>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`<div class="hush-card-meta"><a class="hush-link" href="/p/` + id + `">#` + id + `</a> · ` +
		html.EscapeString(p.CreatedAt.Format("Jan 2 · 15:04")) + `</div>`)

	b.WriteString(voteControlHTML(p.ID, p.Votes, canPost))

	b.WriteString(`<section class="hush-comments" id="comments-` + id + `">`)
	for _, cm := range comments {
		b.WriteString(commentHTML(cm))
	}
	b.WriteString(`</section>`)

	if canPost {
		b.WriteString(commentComposerHTML(id))
	} else {
		b.WriteString(`<p class="hush-sub"><a class="hush-link" href="/login">sign in</a> to reply.</p>`)
	}
	return b.String()
}

// commentHTML is one reply.
func commentHTML(c Comment) string {
	return `<div class="hush-comment"><div class="hush-comment-body">` + c.BodyHTML + `</div></div>`
}

func comment(c Comment) templ.Component { return templ.Raw(commentHTML(c)) }

// commentComposerHTML is the reply box, bound to the shared `reply` signal.
func commentComposerHTML(id string) string {
	return `<div class="hush-reply">` +
		`<textarea id="hush-reply" name="reply" class="hush-reply-input" data-bind:reply aria-label="reply" placeholder="reply…"></textarea>` +
		`<div class="hush-reply-row">` +
		`<span class="hush-hint">bold · italics · lists</span>` +
		`<button class="hush-post" data-on:click="@post('/p/` + id + `/comments')">reply</button>` +
		`</div>` +
		`<div id="reply-note-` + id + `" class="hush-note"></div>` +
		`</div>`
}

// drawerCard is the fragment streamed into #drawer-content when a tile is clicked.
// It adds the close affordance around the shared card body.
// sheetCard is the fragment streamed into #hush-sheet. The outer .hush-sheet
// carries the view-transition-name, so patching it in (or out, on close) slides
// it from the edge. Close is an SSE round-trip that empties the host.
func sheetCard(p Post, comments []Comment, canPost bool) templ.Component {
	return templ.Raw(`<div class="hush-sheet"><article class="hush-card hush-card--sheet">` +
		`<button class="hush-card-close" data-on:click="@get('/sheet/close')" aria-label="close">×</button>` +
		cardInnerHTML(p, comments, canPost) + `</article></div>`)
}

// postPage is the permalink: the same card centered on its own page, reachable by
// navigating straight to /p/{id} (or the #id link in the card meta).
func postPage(p Post, comments []Comment, canPost bool) templ.Component {
	body := `<div data-signals="{reply: ''}"><div class="hush-top">` +
		`<p class="hush-back"><a class="hush-link" href="/">← back to the wall</a></p>` +
		`<article class="hush-card hush-card--page">` + cardInnerHTML(p, comments, canPost) + `</article>` +
		`</div></div>`
	return shell("hush — post #"+strconv.FormatInt(p.ID, 10), templ.Raw(body))
}

// notFoundPage is shown when a permalink points at a post that doesn't exist.
func notFoundPage() templ.Component {
	return shell("hush — not found", templ.Raw(
		`<div class="hush-top"><header class="hush-head"><h1 class="hush-title">hush</h1>`+
			`<p class="hush-sub">that post is gone.</p></header>`+
			`<p class="hush-back"><a class="hush-link" href="/">← back to the wall</a></p></div>`))
}

// note renders the composer's status line. An empty message clears it.
func note(msg string) templ.Component {
	return templ.Raw(html.EscapeString(msg))
}
