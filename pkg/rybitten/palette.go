package rybitten

import (
	"fmt"
	"strings"
)

// Palette samples n colors at evenly spaced hues around the RYB wheel — hue
// i·360/n for i in 0…n-1 — at a fixed saturation and lightness, all run through
// cube. This is the qualitative-series generator: equal-spaced hues give maximal
// separation, and a single cube keeps them in one palette's character.
//
// sat and light are 0–1; lightness reads intuitively (higher is lighter), as the
// conversion inverts it internally for RYB's subtractive axis.
func Palette(cube Cube, n int, sat, light float64) []RGB {
	if n <= 0 {
		return nil
	}
	out := make([]RGB, n)
	for i := range out {
		hue := 360 * float64(i) / float64(n)
		out[i] = RYBHSL2RGB([3]float64{hue, sat, light}, cube, true)
	}
	return out
}

// QualitativePalette is [Palette] with defaults tuned for series colors on a dark
// surface: full saturation at mid lightness, where RYB hues come out vivid but
// not glaring.
func QualitativePalette(cube Cube, n int) []RGB {
	return Palette(cube, n, 1.0, 0.5)
}

// Ramp samples n colors along a single hue from light to dark — a sequential
// scale. hue is in degrees; lightness sweeps lightHi→lightLo so out[0] is the
// lightest. Useful for heatmaps and sequential encodings.
func Ramp(cube Cube, n int, hue, sat, lightHi, lightLo float64) []RGB {
	if n <= 0 {
		return nil
	}
	out := make([]RGB, n)
	for i := range out {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		l := lightHi + (lightLo-lightHi)*t
		out[i] = RYBHSL2RGB([3]float64{hue, sat, l}, cube, true)
	}
	return out
}

// Hexes maps a slice of colors to their CSS hex strings.
func Hexes(colors []RGB) []string {
	out := make([]string, len(colors))
	for i, c := range colors {
		out[i] = c.Hex()
	}
	return out
}

// SeriesCount is how many series slots the framework's CSS token vocabulary
// defines: --color-series-a through --color-series-o, one per letter.
const SeriesCount = 15

// Series samples n colors from the gamut's hue wheel with the qualitative
// defaults — the same colors [SeriesVars] emits when n is [SeriesCount]. Use it
// when the palette is needed as values (image rendering, server-side swatches)
// rather than as CSS.
func Series(g Gamut, n int) []RGB {
	return QualitativePalette(g.Cube, n)
}

// SeriesVars renders a :root block defining the framework's chart series tokens
// --color-series-a…--color-series-o from the gamut's hue wheel, ready to drop
// into input.css so charts repaint in this gamut. A provenance comment names
// the source.
func SeriesVars(g Gamut) string {
	pal := Series(g, SeriesCount)
	var b strings.Builder
	fmt.Fprintf(&b, "/* %s — %s (%d). Generated from rybitten gamut %q. */\n",
		g.Title, g.Author, g.Year, g.Key)
	b.WriteString(":root {\n")
	for i, c := range pal {
		fmt.Fprintf(&b, "  --color-series-%c: %s;\n", 'a'+i, c.Hex())
	}
	b.WriteString("}\n")
	return b.String()
}
