package content

import (
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/ThirdCoastInteractive/Beach/cmd/hush/internal/store"
)

// The fancy-spa palette: every post is a soft, airy tint at the *same* high
// lightness and low chroma — pastels, not vivid colors — and the hue is confined
// to a warm, gold-centered arc so the whole wall harmonizes with the goldgirl
// gold (no cold blues/magentas). Only the hue varies within that arc. Black tile
// text reads cleanly on all of them.
const (
	tileLightness = 0.88 // soft and bright; black text reads easily
	tileChroma    = 0.05 // gentle tint, well inside sRGB across the warm arc

	// warm hue arc (OKLCH degrees): ~30 (soft rose/peach) through ~85 (gold) to
	// ~135 (muted sage). hueBase + [0, hueSpan) keeps every tile warm.
	hueBase = 30
	hueSpan = 105
)

// ColorFor returns a deterministic background color for a post: a hash of the
// post's content picks a hue within the warm arc, with lightness and chroma
// fixed. Same content → same color, always; OKLCH equal-lightness means no tile
// comes out muddier or harsher than the rest.
func ColorFor(p store.Post) string {
	seed := p.BodyHTML
	if p.Kind == "image" || p.Kind == "layout" {
		seed = p.ImageURL
	}
	if seed == "" {
		seed = strconv.FormatInt(p.ID, 10)
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	hue := hueBase + int(h.Sum32()%hueSpan) // warm arc, well spread by the hash
	return fmt.Sprintf("oklch(%.2f %.2f %d)", tileLightness, tileChroma, hue)
}
