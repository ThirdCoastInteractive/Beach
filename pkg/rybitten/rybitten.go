// Package rybitten ports meodai's RYBitten (https://rybitten.space) to Go: a
// pseudo-RYB color model derived from Johannes Itten's chromatic circle.
//
// Where digital tools mix in additive RGB — which flattens the warm neutrals and
// muddy intermediates that painters get for free — RYBitten interpolates through
// an 8-corner RYB cube. Each cube reproduces a historical or artist palette (see
// [Cubes]); feeding the same hue wheel through a different cube repaints it in
// that palette's character. The library is the headline value: a gamut is just
// eight RGB corners, so the whole look of a UI swaps by swapping one [Cube].
//
// The model is subtractive. In RYB space [0,0,0] is the cube's *white* corner and
// [1,1,1] is its *black* corner — the opposite of additive RGB.
//
//	rgb := rybitten.RYBHSL2RGB([3]float64{30, 1, 0.5}, rybitten.Itten, true)
//	css := rgb.Hex() // "#f08e1c"-ish, an Itten orange
//
// The math is a faithful port of the upstream main.ts: smoothstep easing on each
// axis, then trilinear interpolation across the cube corners. No dependencies.
package rybitten

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RGB is a color with red, green and blue channels, each nominally in [0,1].
// Conversions can land just outside that range; [RGB.Hex] and [RGB.Clamp] fold
// values back into gamut.
type RGB [3]float64

// Cube is the eight corner colors of an RYB color cube, in the canonical order
// white, red, yellow, orange, blue, violet, green, black. Index i is the corner
// at RYB coordinate (i&1, i>>1&1, i>>2&1) — i.e. the bits select red, yellow,
// blue in that order. Itten is the default; [Cubes] holds the rest.
type Cube [8]RGB

// Easing maps an axis coordinate in [0,1] through a shaping curve before
// interpolation. [Smoothstep] is the default; pass your own to [RYB2RGBEased].
type Easing func(float64) float64

// Smoothstep is the default easing: the classic S-curve t²(3−2t). It softens the
// approach to each cube corner so blends read like pigment rather than a linear
// ramp.
func Smoothstep(t float64) float64 { return t * t * (3 - 2*t) }

// Lerp is the linear interpolation a + t·(b−a).
func Lerp(a, b, t float64) float64 { return a + t*(b-a) }

// blerp is bilinear interpolation across four corners of a unit square. Corner
// names use row-major indexing (aᵢⱼ with i=ty, j=tx), so a01 sits at (tx=1,ty=0).
func blerp(a00, a01, a10, a11, tx, ty float64) float64 {
	return Lerp(Lerp(a00, a01, tx), Lerp(a10, a11, tx), ty)
}

// trilerp is trilinear interpolation across the eight corners of a unit cube.
// Corner names use row-major indexing (aᵢⱼₖ with i=ty, j=tx, k=tz), so a010 sits
// at (tx=1, ty=0, tz=0). This matches RYBitten's (and Culori's) corner contract.
func trilerp(a000, a010, a100, a110, a001, a011, a101, a111, tx, ty, tz float64) float64 {
	return Lerp(
		blerp(a000, a010, a100, a110, tx, ty),
		blerp(a001, a011, a101, a111, tx, ty),
		tz,
	)
}

// RYB2RGB converts RYB coordinates (red, yellow, blue, each in [0,1]) to RGB
// through cube, using [Smoothstep] easing. Remember the model is subtractive:
// [0,0,0] yields the cube's white corner, [1,1,1] its black corner.
func RYB2RGB(ryb [3]float64, cube Cube) RGB {
	return RYB2RGBEased(ryb, cube, Smoothstep)
}

// RYB2RGBEased is [RYB2RGB] with a caller-supplied easing function.
func RYB2RGBEased(ryb [3]float64, cube Cube, ease Easing) RGB {
	r, y, b := ease(ryb[0]), ease(ryb[1]), ease(ryb[2])
	var out RGB
	for c := 0; c < 3; c++ {
		out[c] = trilerp(
			cube[0][c], cube[1][c], cube[2][c], cube[3][c],
			cube[4][c], cube[5][c], cube[6][c], cube[7][c],
			r, y, b,
		)
	}
	return out
}

// wrapAngle normalizes degrees to [0,360), turning negatives positive.
func wrapAngle(a float64) float64 {
	a = math.Mod(a, 360)
	if a < 0 {
		a += 360
	}
	return a
}

// HSLToRGB converts standard HSL (hue 0–360, saturation and lightness 0–1) to
// RGB in [0,1]. It is the plain HSL→RGB used as the front half of [RYBHSL2RGB];
// exposed because callers building cube coordinates from a hue wheel need it.
func HSLToRGB(hsl [3]float64) [3]float64 {
	h := wrapAngle(hsl[0])
	s, l := hsl[1], hsl[2]
	var m1 float64
	if l < 0.5 {
		m1 = l + s*l
	} else {
		m1 = l + s*(1-l)
	}
	m2 := m1 - (m1-l)*2*math.Abs(math.Mod(h/60, 2)-1)
	switch int(math.Floor(h / 60)) {
	case 0:
		return [3]float64{m1, m2, 2*l - m1}
	case 1:
		return [3]float64{m2, m1, 2*l - m1}
	case 2:
		return [3]float64{2*l - m1, m1, m2}
	case 3:
		return [3]float64{2*l - m1, m2, m1}
	case 4:
		return [3]float64{m2, 2*l - m1, m1}
	case 5:
		return [3]float64{m1, 2*l - m1, m2}
	default:
		g := 2*l - m1
		return [3]float64{g, g, g}
	}
}

// RYBHSL2RGB walks an HSL color through RYB space and out to RGB via cube. This
// is the workhorse for generating palettes: sweep hue 0–360 at a fixed
// saturation and lightness and every color comes out in the cube's character.
//
// invertLightness flips the lightness axis before conversion (the upstream
// default is true). Because RYB is subtractive, inverting makes lightness read
// the intuitive way — higher l is lighter — instead of darkening toward white.
func RYBHSL2RGB(hsl [3]float64, cube Cube, invertLightness bool) RGB {
	l := hsl[2]
	if invertLightness {
		l = 1 - l
	}
	return RYB2RGB(HSLToRGB([3]float64{hsl[0], hsl[1], l}), cube)
}

// Clamp folds each channel into [0,1].
func (c RGB) Clamp() RGB {
	for i, v := range c {
		c[i] = math.Min(1, math.Max(0, v))
	}
	return c
}

// RGB255 returns the color as 8-bit channels, clamped and rounded.
func (c RGB) RGB255() (r, g, b uint8) {
	c = c.Clamp()
	return uint8(math.Round(c[0] * 255)), uint8(math.Round(c[1] * 255)), uint8(math.Round(c[2] * 255))
}

// Hex returns the color as a CSS hex string "#rrggbb", clamped and rounded.
func (c RGB) Hex() string {
	r, g, b := c.RGB255()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// ParseHex parses a CSS hex color — "#rgb", "#rrggbb", with or without the
// leading '#' — into an [RGB]. Any alpha channel ("#rgba"/"#rrggbbaa") is
// rejected: contrast is measured against composited, opaque colors, so an alpha
// value here is almost always a mistake.
func ParseHex(s string) (RGB, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return RGB{}, fmt.Errorf("rybitten: %q is not a #rgb or #rrggbb color", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return RGB{}, fmt.Errorf("rybitten: %q is not a hex color: %w", s, err)
	}
	return RGB{
		float64((v>>16)&0xff) / 255,
		float64((v>>8)&0xff) / 255,
		float64(v&0xff) / 255,
	}, nil
}

// Luminance returns the WCAG relative luminance of the color: the linearized,
// human-weighted brightness that [Contrast] is built from. 0 is black, 1 is
// white. Defined by WCAG 2.1 §relative-luminance; the 0.03928 knee and the 2.4
// exponent are the specification's own constants, not tuning.
func (c RGB) Luminance() float64 {
	c = c.Clamp()
	lin := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

// Contrast returns the WCAG contrast ratio between two colors, from 1 (identical)
// to 21 (black on white). It is symmetric — the order of the arguments does not
// matter.
//
// The thresholds Beach holds itself to (WCAG 2.1 Level AA):
//
//   - 4.5 — normal-size text against its background (SC 1.4.3)
//   - 3.0 — large text (≥24px, or ≥18.66px bold) (SC 1.4.3)
//   - 3.0 — UI component boundaries and focus indicators (SC 1.4.11)
//
// Both colors must be the opaque, composited values actually painted: a
// translucent wash has to be mixed over its backdrop first (see [RGB.Over]).
func Contrast(a, b RGB) float64 {
	la, lb := a.Luminance(), b.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Over composites c onto bg at the given alpha (0–1) and returns the resulting
// opaque color — the value a contrast check needs for a translucent fill like
// the kit's color-mix role washes.
func (c RGB) Over(bg RGB, alpha float64) RGB {
	alpha = math.Min(1, math.Max(0, alpha))
	var out RGB
	for i := range out {
		out[i] = c[i]*alpha + bg[i]*(1-alpha)
	}
	return out
}
