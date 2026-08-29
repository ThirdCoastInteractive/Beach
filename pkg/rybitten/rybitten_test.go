package rybitten

import (
	"math"
	"regexp"
	"testing"
)

const eps = 1e-12

func approx(a, b float64) bool { return math.Abs(a-b) <= 1e-9 }

func closeRGB(a, b RGB) bool {
	return approx(a[0], b[0]) && approx(a[1], b[1]) && approx(a[2], b[2])
}

// --- math, mirrored from RYBitten's src/__tests__/math.test.ts ---------------

func TestLerp(t *testing.T) {
	cases := []struct{ a, b, tt, want float64 }{
		{3, 7, 0, 3}, {3, 7, 1, 7}, {0, 100, 0.5, 50}, {-10, 10, 0.1, -8},
	}
	for _, c := range cases {
		if got := Lerp(c.a, c.b, c.tt); !approx(got, c.want) {
			t.Errorf("Lerp(%v,%v,%v)=%v want %v", c.a, c.b, c.tt, got, c.want)
		}
	}
}

func TestSmoothstep(t *testing.T) {
	if Smoothstep(0) != 0 || Smoothstep(1) != 1 {
		t.Fatalf("smoothstep endpoints: %v %v", Smoothstep(0), Smoothstep(1))
	}
	if !approx(Smoothstep(0.5), 0.5) {
		t.Errorf("smoothstep(0.5)=%v", Smoothstep(0.5))
	}
	// symmetric around 0.5
	if !approx(Smoothstep(0.3)+Smoothstep(0.7), 1) {
		t.Errorf("smoothstep not symmetric: %v", Smoothstep(0.3)+Smoothstep(0.7))
	}
}

func TestTrilerpCornerSlots(t *testing.T) {
	// a000..a111 must return their own corner value at the matching (tx,ty,tz).
	v := func(tx, ty, tz float64) float64 {
		return trilerp(1, 2, 3, 4, 5, 6, 7, 8, tx, ty, tz)
	}
	cases := []struct {
		tx, ty, tz, want float64
	}{
		{0, 0, 0, 1}, {1, 0, 0, 2}, {0, 1, 0, 3}, {1, 1, 0, 4},
		{0, 0, 1, 5}, {1, 0, 1, 6}, {0, 1, 1, 7}, {1, 1, 1, 8},
	}
	for _, c := range cases {
		if got := v(c.tx, c.ty, c.tz); !approx(got, c.want) {
			t.Errorf("trilerp(%v,%v,%v)=%v want %v", c.tx, c.ty, c.tz, got, c.want)
		}
	}
}

// --- conversion: corners are the golden anchor -------------------------------

// RYB2RGB at an integer corner must return that exact cube corner — smoothstep
// fixes 0 and 1, and trilerp at a corner returns it. This pins both the math and
// the canonical corner ordering (white,red,yellow,orange,blue,violet,green,black)
// to the cube data, for every gamut.
func TestRYB2RGBHitsCorners(t *testing.T) {
	for key, g := range Cubes {
		for i := 0; i < 8; i++ {
			coords := [3]float64{float64(i & 1), float64((i >> 1) & 1), float64((i >> 2) & 1)}
			got := RYB2RGB(coords, g.Cube)
			if !closeRGB(got, g.Cube[i]) {
				t.Errorf("%s corner %d: RYB2RGB(%v)=%v want %v", key, i, coords, got, g.Cube[i])
			}
		}
	}
}

func TestHSLToRGBPrimaries(t *testing.T) {
	cases := []struct {
		hsl, want [3]float64
	}{
		{[3]float64{0, 1, 0.5}, [3]float64{1, 0, 0}},
		{[3]float64{120, 1, 0.5}, [3]float64{0, 1, 0}},
		{[3]float64{240, 1, 0.5}, [3]float64{0, 0, 1}},
		{[3]float64{0, 0, 0.5}, [3]float64{0.5, 0.5, 0.5}},
	}
	for _, c := range cases {
		got := HSLToRGB(c.hsl)
		if !approx(got[0], c.want[0]) || !approx(got[1], c.want[1]) || !approx(got[2], c.want[2]) {
			t.Errorf("HSLToRGB(%v)=%v want %v", c.hsl, got, c.want)
		}
	}
}

// rybHsl2rgb composition equivalence, from src/__tests__/rybHsl2rgb.test.ts.
func TestRYBHSLComposition(t *testing.T) {
	hsl := [3]float64{30, 0.6, 0.4}
	out := RYBHSL2RGB(hsl, Itten, true)
	ref := RYB2RGB(HSLToRGB([3]float64{hsl[0], hsl[1], 1 - hsl[2]}), Itten)
	if !closeRGB(out, ref) {
		t.Errorf("invert=true: %v != %v", out, ref)
	}

	hsl2 := [3]float64{200, 0.5, 0.3}
	out2 := RYBHSL2RGB(hsl2, Itten, false)
	ref2 := RYB2RGB(HSLToRGB(hsl2), Itten)
	if !closeRGB(out2, ref2) {
		t.Errorf("invert=false: %v != %v", out2, ref2)
	}
}

// invertLightness symmetry: f(h,s,l)[true] == f(h,s,1-l)[false].
func TestInvertLightnessSymmetry(t *testing.T) {
	a := RYBHSL2RGB([3]float64{120, 0.7, 0.25}, Itten, true)
	b := RYBHSL2RGB([3]float64{120, 0.7, 0.75}, Itten, false)
	if !closeRGB(a, b) {
		t.Errorf("symmetry broken: %v != %v", a, b)
	}
}

func TestCustomCubeChangesOutput(t *testing.T) {
	a := RYBHSL2RGB([3]float64{0, 1, 0.5}, Itten, true)
	b := RYBHSL2RGB([3]float64{0, 1, 0.5}, Cubes["munsell"].Cube, true)
	if closeRGB(a, b) {
		t.Errorf("expected different cubes to differ, both %v", a)
	}
}

// --- cube data integrity -----------------------------------------------------

func TestCubeDataIntegrity(t *testing.T) {
	if len(Keys) != len(Cubes) {
		t.Fatalf("Keys (%d) and Cubes (%d) out of sync", len(Keys), len(Cubes))
	}
	seen := map[string]bool{}
	for _, k := range Keys {
		if seen[k] {
			t.Errorf("duplicate key in Keys: %s", k)
		}
		seen[k] = true
		g, ok := Cubes[k]
		if !ok {
			t.Errorf("Keys has %q with no Cubes entry", k)
			continue
		}
		if g.Key != k {
			t.Errorf("Cubes[%q].Key = %q, want %q", k, g.Key, k)
		}
		for i, corner := range g.Cube {
			for ch, v := range corner {
				if v < -eps || v > 1+eps {
					t.Errorf("%s corner %d channel %d = %v out of [0,1]", k, i, ch, v)
				}
			}
		}
	}
	if !closeRGB(Cubes["itten"].Cube[1], Itten[1]) {
		t.Error("Itten var and Cubes[\"itten\"] disagree")
	}
}

// --- palette -----------------------------------------------------------------

var hexRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func TestPalette(t *testing.T) {
	if Palette(Itten, 0, 1, 0.5) != nil {
		t.Error("Palette(n=0) should be nil")
	}
	pal := QualitativePalette(Itten, 12)
	if len(pal) != 12 {
		t.Fatalf("len=%d want 12", len(pal))
	}
	// deterministic
	pal2 := QualitativePalette(Itten, 12)
	for i := range pal {
		if !closeRGB(pal[i], pal2[i]) {
			t.Fatalf("palette not deterministic at %d", i)
		}
	}
	// hue 0 is the wheel's red: red channel dominates through Itten.
	r := pal[0]
	if !(r[0] > r[1] && r[0] > r[2]) {
		t.Errorf("hue 0 not red-dominant: %v", r)
	}
	for _, h := range Hexes(pal) {
		if !hexRe.MatchString(h) {
			t.Errorf("bad hex %q", h)
		}
	}
}

func TestSeries(t *testing.T) {
	g := Cubes["munsell"]
	pal := Series(g, SeriesCount)
	if len(pal) != SeriesCount {
		t.Fatalf("len=%d want %d", len(pal), SeriesCount)
	}
	// Series is the values-side twin of SeriesVars: same sampler, same colors.
	ref := QualitativePalette(g.Cube, SeriesCount)
	for i := range pal {
		if !closeRGB(pal[i], ref[i]) {
			t.Errorf("Series[%d]=%v != QualitativePalette %v", i, pal[i], ref[i])
		}
	}
	if Series(g, 0) != nil {
		t.Error("Series(n=0) should be nil")
	}
}

func TestSeriesVars(t *testing.T) {
	css := SeriesVars(Cubes["munsell"])
	if !regexp.MustCompile(`(?s):root \{.*\}`).MatchString(css) {
		t.Fatalf("SeriesVars not a :root block:\n%s", css)
	}
	// All 15 letter slots present, a through o, each a hex color.
	for i := 0; i < SeriesCount; i++ {
		re := regexp.MustCompile(`--color-series-` + string(rune('a'+i)) + `:\s*#[0-9a-f]{6};`)
		if !re.MatchString(css) {
			t.Errorf("SeriesVars missing --color-series-%c:\n%s", 'a'+i, css)
		}
	}
	if regexp.MustCompile(`--color-series-p`).MatchString(css) {
		t.Errorf("SeriesVars emitted a 16th slot:\n%s", css)
	}
	// Tokens carry the gamut's colors in wheel order.
	hexes := Hexes(Series(Cubes["munsell"], SeriesCount))
	if !regexp.MustCompile(`--color-series-a:\s*` + hexes[0]).MatchString(css) {
		t.Errorf("series-a is not the first wheel sample %s:\n%s", hexes[0], css)
	}
	if !regexp.MustCompile(`--color-series-o:\s*` + hexes[14]).MatchString(css) {
		t.Errorf("series-o is not the last wheel sample %s:\n%s", hexes[14], css)
	}
}

// TestContrastKnownPairs pins the WCAG luminance/contrast math against the
// values the specification itself names: black-on-white is 21:1, a color against
// itself is 1:1, and the ratio is symmetric.
func TestContrastKnownPairs(t *testing.T) {
	black, white := RGB{0, 0, 0}, RGB{1, 1, 1}
	if got := Contrast(black, white); math.Abs(got-21) > 0.001 {
		t.Errorf("Contrast(black, white) = %.4f, want 21", got)
	}
	if got := Contrast(white, black); math.Abs(got-21) > 0.001 {
		t.Errorf("Contrast is not symmetric: %.4f", got)
	}
	if got := Contrast(white, white); math.Abs(got-1) > 0.001 {
		t.Errorf("Contrast(white, white) = %.4f, want 1", got)
	}
	// Relative luminance is defined as 0 for black and 1 for white.
	if got := black.Luminance(); got != 0 {
		t.Errorf("black luminance = %v, want 0", got)
	}
	if got := white.Luminance(); math.Abs(got-1) > 1e-12 {
		t.Errorf("white luminance = %v, want 1", got)
	}
	// A mid grey: #767676 is the canonical "smallest grey that passes 4.5:1 on
	// white" from the WCAG techniques, which makes it a good fixed point.
	grey, err := ParseHex("#767676")
	if err != nil {
		t.Fatalf("ParseHex: %v", err)
	}
	if got := Contrast(grey, white); got < 4.5 || got > 4.6 {
		t.Errorf("Contrast(#767676, white) = %.3f, want just over 4.5", got)
	}
}

func TestParseHex(t *testing.T) {
	ok := map[string]RGB{
		"#ffffff": {1, 1, 1},
		"ffffff":  {1, 1, 1},
		"#000":    {0, 0, 0},
		"#fff":    {1, 1, 1},
	}
	for in, want := range ok {
		got, err := ParseHex(in)
		if err != nil {
			t.Errorf("ParseHex(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseHex(%q) = %v, want %v", in, got, want)
		}
	}
	// Alpha forms are rejected: contrast is measured on composited colors, so an
	// alpha value here means the caller has not composited yet.
	for _, in := range []string{"#ffffffff", "#abcd", "#gg0000", "", "#12345"} {
		if _, err := ParseHex(in); err == nil {
			t.Errorf("ParseHex(%q) should have failed", in)
		}
	}
}

// TestOverComposites checks the wash helper the role-color badges need: a
// translucent fill has to be flattened onto its backdrop before its contrast
// means anything.
func TestOverComposites(t *testing.T) {
	white, black := RGB{1, 1, 1}, RGB{0, 0, 0}
	if got := white.Over(black, 1); got != white {
		t.Errorf("fully opaque Over changed the color: %v", got)
	}
	if got := white.Over(black, 0); got != black {
		t.Errorf("fully transparent Over did not yield the backdrop: %v", got)
	}
	half := white.Over(black, 0.5)
	for i, v := range half {
		if math.Abs(v-0.5) > 1e-12 {
			t.Errorf("half-alpha channel %d = %v, want 0.5", i, v)
		}
	}
}
