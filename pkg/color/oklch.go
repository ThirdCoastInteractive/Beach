// Package color is Beach's color math: OKLCH, sRGB, and the WCAG contrast
// the framework's palette is derived under.
//
// It exists because the design tokens are *computed*, not picked, and computing
// them needs a color space where the numbers mean what they appear to mean.
// OKLCH is that space:
//
//   - L is perceived lightness. Equal steps look equal, across every hue. In HSL
//     they emphatically do not — hsl(60 100% 50%) and hsl(240 100% 50%) share a
//     lightness number and differ enormously in apparent brightness.
//   - H is stable. An HSL blue drifts ~18° toward purple as it lightens; an
//     OKLCH hue stays put, which is what lets a ramp be one hue at several
//     lightnesses rather than several hues by accident.
//   - C is absolute colorfulness, independent of L, so "as vivid as this hue
//     gets here" is a question with an answer ([MaxChroma]).
//
// That last point is the one that matters most here, and it is why the
// framework's first attempt at a derived palette — through the RYB cube in
// pkg/rybitten — kept producing muddy colors. An RYB cube is a lookup: a hue is
// a trilinear interpolation between eight fixed corners, so you cannot ask it
// for a vivid teal, only for a position on an arc and whatever the cube happens
// to hold there. In OKLCH you ask for the hue and take the most chroma sRGB will
// give you at that lightness.
//
// The package is a leaf: no dependencies, importable by anything.
package color

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// OKLCH is a color in the OKLCH space: lightness 0–1, chroma 0–~0.4, hue in
// degrees. A color may be *outside* the sRGB gamut — high chroma is not
// available at every lightness and hue — so a value here is a request, and
// [OKLCH.InGamut] or [OKLCH.Clamp] is how you find out whether it can be shown.
type OKLCH struct {
	L, C, H float64
}

// SRGB is a color with channels in 0–1, gamma-encoded (the values a CSS hex
// literal holds).
type SRGB struct{ R, G, B float64 }

// The Oklab matrices, from Björn Ottosson's derivation. They are transcribed
// constants, not tuning: changing one does not adjust the color space, it breaks
// it.
//
// Oklab reaches sRGB through an LMS cone response with a cube-root
// nonlinearity, which is where the perceptual uniformity comes from.

func (c OKLCH) oklab() (l, a, b float64) {
	rad := c.H * math.Pi / 180
	return c.L, c.C * math.Cos(rad), c.C * math.Sin(rad)
}

// LinearSRGB converts to linear-light sRGB. Channels may fall outside 0–1, which
// is exactly how an out-of-gamut color announces itself.
func (c OKLCH) LinearSRGB() (r, g, b float64) {
	ll, aa, bb := c.oklab()
	return linearFromLab(ll, aa, bb)
}

// linearFromLab is [OKLCH.LinearSRGB] with the polar-to-rectangular step already
// done. It is separate because hue is *constant* through a gamut search, and the
// sine and cosine that turn hue into Oklab's a and b are by far the most
// expensive arithmetic in the conversion — recomputing them on every probe costs
// more than the rest of the palette derivation put together.
func linearFromLab(ll, aa, bb float64) (r, g, b float64) {
	l_ := ll + 0.3963377774*aa + 0.2158037573*bb
	m_ := ll - 0.1055613458*aa - 0.0638541728*bb
	s_ := ll - 0.0894841775*aa - 1.2914855480*bb

	lc := l_ * l_ * l_
	mc := m_ * m_ * m_
	sc := s_ * s_ * s_

	r = 4.0767416621*lc - 3.3077115913*mc + 0.2309699292*sc
	g = -1.2684380046*lc + 2.6097574011*mc - 0.3413193965*sc
	b = -0.0041960863*lc - 0.7034186147*mc + 1.7076147010*sc
	return r, g, b
}

// inGamutLab is [OKLCH.InGamut] on rectangular coordinates.
func inGamutLab(ll, aa, bb float64) bool {
	r, g, b := linearFromLab(ll, aa, bb)
	return r >= -gamutEpsilon && r <= 1+gamutEpsilon &&
		g >= -gamutEpsilon && g <= 1+gamutEpsilon &&
		b >= -gamutEpsilon && b <= 1+gamutEpsilon
}

// FromLinearSRGB converts linear-light sRGB back to OKLCH. It is the inverse of
// [OKLCH.LinearSRGB] and the front half of [FromSRGB].
func FromLinearSRGB(r, g, b float64) OKLCH {
	l := 0.4122214708*r + 0.5363325363*g + 0.0514459929*b
	m := 0.2119034982*r + 0.6806995451*g + 0.1073969566*b
	s := 0.0883024619*r + 0.2817188376*g + 0.6299787005*b

	l_ := cbrt(l)
	m_ := cbrt(m)
	s_ := cbrt(s)

	ll := 0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_
	aa := 1.9779984951*l_ - 2.4285922050*m_ + 0.4505937099*s_
	bb := 0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_

	h := math.Atan2(bb, aa) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return OKLCH{L: ll, C: math.Hypot(aa, bb), H: h}
}

// cbrt is the real cube root, sign-preserving. math.Cbrt already handles
// negatives; this wrapper exists only to name what the nonlinearity is.
func cbrt(x float64) float64 { return math.Cbrt(x) }

// --- sRGB transfer ------------------------------------------------------------

// gamma encodes a linear channel to sRGB. The 0.0031308 knee and the 2.4
// exponent are the sRGB specification's own constants.
func gamma(x float64) float64 {
	if x <= 0.0031308 {
		return 12.92 * x
	}
	return 1.055*math.Pow(x, 1/2.4) - 0.055
}

// linearize decodes an sRGB channel back to linear light.
func linearize(x float64) float64 {
	if x <= 0.04045 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

// SRGB converts to displayable sRGB, clamping each channel. A color outside the
// gamut is clipped rather than rejected — call [OKLCH.Clamp] first if you want
// the in-gamut color of the same hue and lightness instead of a clipped one.
func (c OKLCH) SRGB() SRGB {
	r, g, b := c.LinearSRGB()
	return SRGB{
		R: clamp01(gamma(r)),
		G: clamp01(gamma(g)),
		B: clamp01(gamma(b)),
	}
}

// FromSRGB converts a gamma-encoded sRGB color to OKLCH.
func FromSRGB(c SRGB) OKLCH {
	return FromLinearSRGB(linearize(c.R), linearize(c.G), linearize(c.B))
}

func clamp01(x float64) float64 { return math.Min(1, math.Max(0, x)) }

// --- gamut --------------------------------------------------------------------

// gamutEpsilon absorbs floating-point error at the boundary. Without it a color
// that lands exactly on a channel limit reads as out of gamut, and [MaxChroma]
// returns a value one step short of the real edge on every hue.
const gamutEpsilon = 1e-6

// InGamut reports whether the color is displayable in sRGB.
func (c OKLCH) InGamut() bool {
	r, g, b := c.LinearSRGB()
	for _, v := range [3]float64{r, g, b} {
		if v < -gamutEpsilon || v > 1+gamutEpsilon {
			return false
		}
	}
	return true
}

// MaxChroma returns the highest chroma sRGB can show at this lightness and hue.
//
// The gamut boundary is irregular and this is not a detail — it is the single
// most important fact for building a palette. At L=0.5 purple reaches C≈0.29
// while cyan manages only C≈0.09, so asking every hue for the same absolute
// chroma makes some colors shout and others look dirty. Ask each hue for the
// same *fraction* of this instead, and they read as equally vivid.
func MaxChroma(l, h float64) float64 {
	if l <= 0 || l >= 1 {
		return 0 // black and white hold no chroma
	}
	// The boundary is monotonic in chroma for a fixed L and H — once a channel
	// leaves the cube it does not come back — so it bisects cleanly. Twenty
	// halvings of a 0.5 range land within a millionth, which is far finer than
	// the eight bits per channel this is ultimately rendered at; the function is
	// called often enough in a derivation that the extra dozen would show.
	// Hue is fixed for the whole search, so its sine and cosine are computed
	// once here rather than twenty times inside the loop.
	rad := h * math.Pi / 180
	ca, sa := math.Cos(rad), math.Sin(rad)

	lo, hi := 0.0, 0.5
	for range 20 {
		mid := (lo + hi) / 2
		if inGamutLab(l, mid*ca, mid*sa) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

// Clamp reduces chroma until the color fits sRGB, keeping lightness and hue.
// Reducing chroma is the correct repair: clipping the channels instead shifts
// both the hue and the lightness, which is how a "vivid" palette ends up with
// one member that is a slightly different color than it was asked to be.
func (c OKLCH) Clamp() OKLCH {
	if c.InGamut() {
		return c
	}
	c.C = MaxChroma(c.L, c.H)
	return c
}

// --- contrast -----------------------------------------------------------------

// Luminance returns WCAG relative luminance. The coefficients apply to
// linear-light sRGB, which is what [OKLCH.LinearSRGB] already produces — so this
// is a dot product, not another conversion.
//
// Channels are clamped first: luminance is a property of the color as displayed,
// and an out-of-gamut request is displayed clipped.
func (c OKLCH) Luminance() float64 {
	r, g, b := c.LinearSRGB()
	// Clamp in linear space to match what the screen will actually emit.
	r, g, b = clamp01(r), clamp01(g), clamp01(b)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// Contrast returns the WCAG 2.1 contrast ratio between two colors, 1 (identical)
// to 21 (black on white). It is symmetric.
//
// The thresholds Beach holds itself to, all Level AA:
//
//   - [AAText] 4.5 — normal-size text (SC 1.4.3)
//   - [AANonText] 3.0 — component boundaries and focus indicators (SC 1.4.11),
//     and large text
//
// Both colors must be opaque and composited: a translucent wash has to be mixed
// over its backdrop first (see [OKLCH.Over]).
func Contrast(a, b OKLCH) float64 {
	la, lb := a.Luminance(), b.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// The two WCAG 2.1 Level AA thresholds the framework derives against.
const (
	AAText    = 4.5
	AANonText = 3.0
)

// Over composites c onto bg at the given alpha and returns the resulting opaque
// color — what a contrast check needs for a translucent fill.
//
// The mix happens in linear light, not in OKLCH: alpha compositing is a physical
// operation on emitted light, and interpolating the polar coordinates instead
// would send the result on a detour around the hue circle.
func (c OKLCH) Over(bg OKLCH, alpha float64) OKLCH {
	alpha = clamp01(alpha)
	fr, fg, fb := c.LinearSRGB()
	br, bg2, bb := bg.LinearSRGB()
	return FromLinearSRGB(
		fr*alpha+br*(1-alpha),
		fg*alpha+bg2*(1-alpha),
		fb*alpha+bb*(1-alpha),
	)
}

// --- text ---------------------------------------------------------------------

// CSS renders the color as an oklch() function — the form the stylesheet gets.
// Emitting OKLCH rather than hex keeps the sheet readable as *intent* ("this
// hue, this vividness, this lightness") instead of as six opaque digits.
func (c OKLCH) CSS() string {
	return fmt.Sprintf("oklch(%s %s %s)", num(c.L, 4), num(c.C, 4), num(c.H, 2))
}

// num formats without trailing zeros, and normalises -0 to 0.
func num(v float64, places int) string {
	if v == 0 {
		return "0"
	}
	s := strconv.FormatFloat(v, 'f', places, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

// Hex renders the clamped color as "#rrggbb".
//
// The framework serves oklch() and does not need this to paint, but a hex value
// is what a contrast checker, a screenshot diff and a person reading a test
// failure all expect, so the package owes one.
func (c OKLCH) Hex() string {
	s := c.SRGB()
	return fmt.Sprintf("#%02x%02x%02x", round8(s.R), round8(s.G), round8(s.B))
}

func round8(v float64) uint8 { return uint8(math.Round(clamp01(v) * 255)) }

// ParseHex parses "#rgb" or "#rrggbb", with or without the leading '#'. An alpha
// channel is rejected: contrast is measured against composited, opaque colors,
// so an alpha value here is nearly always a mistake.
func ParseHex(s string) (OKLCH, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return OKLCH{}, fmt.Errorf("color: %q is not a #rgb or #rrggbb color", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return OKLCH{}, fmt.Errorf("color: %q is not a hex color: %w", s, err)
	}
	return FromSRGB(SRGB{
		R: float64((v>>16)&0xff) / 255,
		G: float64((v>>8)&0xff) / 255,
		B: float64(v&0xff) / 255,
	}), nil
}
