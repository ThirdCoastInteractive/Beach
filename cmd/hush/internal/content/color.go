package content

import (
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/store"
)

// hue band kept fixed and chroma/lightness held constant so every post is a
// bright, saturated color at the *same* perceived brightness — a clean, cohesive
// palette where only the hue changes. Black tile text reads on all of them.
const (
	tileLightness = 0.70 // bright
	tileChroma    = 0.15 // saturated, still in sRGB across hues
)

// ColorFor returns a deterministic background color for a post: a hash of the
// post's content picks an OKLCH hue, with lightness and chroma fixed. Same
// content → same color, always; OKLCH equal-lightness means no hue comes out
// muddier or harsher than the rest.
func ColorFor(p store.Post) string {
	seed := p.BodyHTML
	if p.Kind == "image" {
		seed = p.ImageURL
	}
	if seed == "" {
		seed = strconv.FormatInt(p.ID, 10)
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	hue := h.Sum32() % 360 // 0–359, well spread by the hash
	return fmt.Sprintf("oklch(%.2f %.2f %d)", tileLightness, tileChroma, hue)
}
