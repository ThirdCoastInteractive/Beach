// The per-post inline styles and link builders the wall and feed views share.
// These are the only inline styles in hush, and all of them are safe to exempt
// from templ's CSS property sanitizer (which predates oklch()): ColorFor's
// output is a fixed-format oklch() string derived from a content hash, and the
// view-transition-name is "hush-post-" plus a decimal id — no user input
// reaches either.
package content

import (
	"strconv"

	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/store"
	"github.com/a-h/templ"
)

// TileBG paints a text post's computed color (the card hero in the feed).
func TileBG(p store.Post) templ.SafeCSS {
	return templ.SafeCSS("background: " + ColorFor(p))
}

// PostStyle is a tile's inline style: its computed color plus the post
// transition class. The name itself arrives only on focus (the :focus rule in
// hush.css), so leaving the wall morphs one hero square — not seventeen.
func PostStyle(p store.Post) templ.SafeCSS {
	return templ.SafeCSS("background: " + ColorFor(p) + "; view-transition-class: hush-post")
}

// VTName is a post's view-transition-name.
func VTName(p store.Post) string {
	return "hush-post-" + strconv.FormatInt(p.ID, 10)
}

// VTStyle names a feed card for the morph, unconditionally — consecutive
// posts share most of their neighbors, so a feed→feed navigation pairs the
// overlap and the column slides. side ("hush-above"/"hush-below"/"" for the
// focused card) classes the card by its end of the feed, so a card with no
// pair on the other page enters or exits along its own side instead of
// popping or fading.
func VTStyle(p store.Post, side string) templ.SafeCSS {
	s := "view-transition-name: " + VTName(p) + "; view-transition-class: hush-post"
	if side != "" {
		s += " " + side
	}
	return templ.SafeCSS(s)
}

// PostHref is the real link to a post's feed page. The fragment lands the
// browser on the focused card even when newer neighbors render above it —
// native scroll-to-fragment, no script.
func PostHref(id int64) string {
	s := strconv.FormatInt(id, 10)
	return "/p/" + s + "#p" + s
}

// MoreGet is a sentinel's intersect expression: fetch the next batch of feed
// neighbors past cursor in the given direction.
func MoreGet(dir string, cursor int64) string {
	return "@get('/feed/" + dir + "?cursor=" + strconv.FormatInt(cursor, 10) + "')"
}
