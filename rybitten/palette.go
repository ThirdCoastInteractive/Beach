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

// SkyStops are the Tailwind --color-sky-* stops the driftwood kit actually
// consumes for its accent, paired with the lightness used to sample each from a
// gamut. Light→dark, matched loosely to Tailwind's own sky ramp so contrast
// relationships on the dark surface survive the swap.
var SkyStops = []struct {
	Stop  int
	Light float64
}{
	{300, 0.82}, {400, 0.66}, {500, 0.55}, {600, 0.45}, {950, 0.14},
}

// AccentVars renders :root overrides for the --color-sky-* stops in [SkyStops],
// sampling the gamut's blue axis (hue 240, full saturation) at each stop's
// lightness. Because driftwood's @apply rules compile to var(--color-sky-N), and
// an unlayered :root beats Tailwind's @layer theme, these repaint every sky-*
// accent in the kit with no markup change.
func AccentVars(g Gamut) string {
	var b strings.Builder
	fmt.Fprintf(&b, "/* accent ← rybitten gamut %q (%s) blue axis */\n", g.Key, g.Title)
	b.WriteString(":root {\n")
	for _, s := range SkyStops {
		c := RYBHSL2RGB([3]float64{240, 1, s.Light}, g.Cube, true)
		fmt.Fprintf(&b, "  --color-sky-%d: %s;\n", s.Stop, c.Hex())
	}
	b.WriteString("}\n")
	return b.String()
}

// CSSVars renders a :root block defining --chart-qualitative-1…count from the
// gamut's hue wheel, ready to drop into the framework's CSS so charts (and any
// token consumer) repaint in this gamut. A provenance comment names the source.
// count is clamped to at least 1.
func CSSVars(g Gamut, count int) string {
	if count < 1 {
		count = 1
	}
	pal := QualitativePalette(g.Cube, count)
	var b strings.Builder
	fmt.Fprintf(&b, "/* %s — %s (%d). Generated from rybitten gamut %q. */\n",
		g.Title, g.Author, g.Year, g.Key)
	b.WriteString(":root {\n")
	for i, c := range pal {
		fmt.Fprintf(&b, "  --chart-qualitative-%d: %s;\n", i+1, c.Hex())
	}
	b.WriteString("}\n")
	return b.String()
}
