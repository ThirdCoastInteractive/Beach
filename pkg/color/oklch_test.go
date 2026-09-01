package color

import (
	"math"
	"testing"
)

// The conversion is a transcription of published matrices, so the test that
// matters is not "does it compute something" but "does it agree with the
// reference values everyone else's implementation agrees with". These come from
// Ottosson's own worked examples and from CSS Color 4.

func TestKnownConversions(t *testing.T) {
	cases := []struct {
		name    string
		hex     string
		l, c, h float64
	}{
		// The achromatic anchors: white and black pin the lightness axis, and
		// mid grey proves the cube root rather than a linear ramp.
		{"white", "#ffffff", 1.0, 0, 0},
		{"black", "#000000", 0, 0, 0},
		// The sRGB primaries and secondaries, from the CSS Color 4 samples.
		{"red", "#ff0000", 0.6280, 0.2577, 29.23},
		{"green", "#00ff00", 0.8664, 0.2948, 142.50},
		{"blue", "#0000ff", 0.4520, 0.3132, 264.05},
		{"cyan", "#00ffff", 0.9054, 0.1546, 194.77},
		{"magenta", "#ff00ff", 0.7017, 0.3225, 328.36},
		{"yellow", "#ffff00", 0.9680, 0.2110, 109.77},
	}
	for _, tc := range cases {
		got, err := ParseHex(tc.hex)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if math.Abs(got.L-tc.l) > 0.002 {
			t.Errorf("%s: L = %.4f, want %.4f", tc.name, got.L, tc.l)
		}
		if math.Abs(got.C-tc.c) > 0.002 {
			t.Errorf("%s: C = %.4f, want %.4f", tc.name, got.C, tc.c)
		}
		// Hue is meaningless on an achromatic color, so only check it when
		// there is chroma to carry it.
		if tc.c > 0.01 && math.Abs(got.H-tc.h) > 0.5 {
			t.Errorf("%s: H = %.2f, want %.2f", tc.name, got.H, tc.h)
		}
	}
}

// TestRoundTrip walks a spread of real colors out to OKLCH and back. A
// conversion that is subtly wrong in one direction usually still round-trips,
// so this is not a substitute for the reference values above — it catches the
// other failure, where one direction is right and the inverse is not.
func TestRoundTrip(t *testing.T) {
	for r := 0; r <= 255; r += 51 {
		for g := 0; g <= 255; g += 51 {
			for b := 0; b <= 255; b += 51 {
				in := SRGB{float64(r) / 255, float64(g) / 255, float64(b) / 255}
				out := FromSRGB(in).SRGB()
				for i, pair := range [3][2]float64{{in.R, out.R}, {in.G, out.G}, {in.B, out.B}} {
					if math.Abs(pair[0]-pair[1]) > 0.001 {
						t.Fatalf("rgb(%d,%d,%d) channel %d: %.4f round-tripped to %.4f",
							r, g, b, i, pair[0], pair[1])
					}
				}
			}
		}
	}
}

// TestMaxChromaIsAtTheBoundary checks both halves of the claim: the returned
// chroma is displayable, and it is genuinely the most that is — a hair more is
// not.
//
// This is the function the whole palette leans on, and a version that returned
// something merely safe rather than maximal would fail nothing else in the suite
// while quietly draining the color out of every theme.
func TestMaxChromaIsAtTheBoundary(t *testing.T) {
	for _, h := range []float64{0, 30, 90, 145, 195, 240, 285, 330} {
		for _, l := range []float64{0.2, 0.4, 0.5, 0.7, 0.9} {
			max := MaxChroma(l, h)
			if max <= 0 {
				t.Errorf("L=%.1f H=%.0f: max chroma is %.4f", l, h, max)
				continue
			}
			if !(OKLCH{L: l, C: max, H: h}).InGamut() {
				t.Errorf("L=%.1f H=%.0f: max chroma %.4f is not in gamut", l, h, max)
			}
			if (OKLCH{L: l, C: max + 0.01, H: h}).InGamut() {
				t.Errorf("L=%.1f H=%.0f: max chroma %.4f is not maximal", l, h, max)
			}
		}
	}
}

// TestCyanIsTheNarrowestHue is the fact that sank the previous RYB-derived
// palette, kept as a test so the reasoning survives in the repo.
//
// Maximum chroma varies by roughly 3x across hues at a fixed lightness. A
// palette that hands every hue the same absolute chroma is therefore asking
// purple for something easy and cyan for something impossible — the cyan comes
// back clipped and grey. Every chromatic token in pkg/theme takes a *fraction*
// of this number instead, which is what makes the roles read as equally vivid.
func TestCyanIsTheNarrowestHue(t *testing.T) {
	const l = 0.5
	cyan := MaxChroma(l, 195)
	purple := MaxChroma(l, 285)
	if cyan >= purple {
		t.Fatalf("expected cyan (%.3f) to be narrower than purple (%.3f) at L=%.1f", cyan, purple, l)
	}
	if ratio := purple / cyan; ratio < 2 {
		t.Errorf("cyan-to-purple chroma ratio is %.2fx; the whole same-C%%-not-same-C rule "+
			"exists because this spread is large", ratio)
	}
}

// TestContrastMatchesWCAG checks the ratio against the two values every
// implementation is graded on.
func TestContrastMatchesWCAG(t *testing.T) {
	white, _ := ParseHex("#ffffff")
	black, _ := ParseHex("#000000")
	if got := Contrast(white, black); math.Abs(got-21) > 0.01 {
		t.Errorf("white on black is %.2f:1, want 21:1", got)
	}
	if got := Contrast(white, white); math.Abs(got-1) > 0.001 {
		t.Errorf("white on white is %.2f:1, want 1:1", got)
	}
	// A published mid-case: #767676 is the classic "just passes 4.5:1 on white"
	// grey used in every accessibility tutorial.
	grey, _ := ParseHex("#767676")
	if got := Contrast(grey, white); got < 4.5 || got > 4.6 {
		t.Errorf("#767676 on white is %.2f:1, want just over 4.5:1", got)
	}
}

// TestSolveMeetsItsRatio is the solver's contract: whatever it returns clears
// the ratio it was asked for, on the side it was asked for.
func TestSolveMeetsItsRatio(t *testing.T) {
	paper := Grey(0.16, 70, 0.006)
	white := OKLCH{L: 1}

	for _, h := range []float64{0, 60, 145, 195, 240, 300} {
		for _, ratio := range []float64{AANonText, AAText, 7} {
			got, ok := Solve(paper, h, 0.9, ratio, true)
			if !ok {
				t.Errorf("H=%.0f on dark paper at %.1f:1: no solution", h, ratio)
				continue
			}
			if c := Contrast(got, paper); c < ratio {
				t.Errorf("H=%.0f: solved %s is %.2f:1 on paper, asked for %.1f", h, got.CSS(), c, ratio)
			}
			if got.L <= paper.L {
				t.Errorf("H=%.0f: asked for lighter than paper, got L=%.3f vs %.3f", h, got.L, paper.L)
			}
		}
	}

	// An impossible request has to come back false rather than return something
	// that merely looks like an answer. Two shapes of impossible:
	//
	// Nothing is lighter than white…
	if _, ok := Solve(white, 240, 0.9, AAText, true); ok {
		t.Error("expected no solution lighter than white")
	}
	// …and no pair of colors exceeds 21:1, so a mid grey cannot reach it in
	// either direction.
	mid := Grey(0.5, 70, 0)
	if _, ok := Solve(mid, 240, 0.9, 21, true); ok {
		t.Error("expected no solution at 21:1 above a mid grey")
	}
	if _, ok := Solve(mid, 240, 0.9, 21, false); ok {
		t.Error("expected no solution at 21:1 below a mid grey")
	}
}

// TestSolveRecedes pins the direction of Solve's search. It returns the stop
// closest to the background that still clears the ratio, not the furthest —
// which is what makes a muted stop muted rather than a second primary ink.
func TestSolveRecedes(t *testing.T) {
	paper := Grey(0.16, 70, 0.006)
	muted, ok := Solve(paper, 70, 0.1, AAText, true)
	if !ok {
		t.Fatal("no muted stop")
	}
	// It clears its bar…
	if c := Contrast(muted, paper); c < AAText {
		t.Fatalf("muted is %.2f:1", c)
	}
	// …and does not clear a materially higher one, which is what proves it
	// stopped at the boundary instead of running to white.
	if c := Contrast(muted, paper); c > AAText+0.6 {
		t.Errorf("muted is %.2f:1 — Solve overshot the boundary rather than receding to it", c)
	}
}

// TestSolveVividPrefersChroma is the other objective. Against the same backdrop
// and the same bar, the vivid solver must come back with more chroma than the
// receding one, or the distinction the palette is built on does not exist.
func TestSolveVividPrefersChroma(t *testing.T) {
	paper := Grey(0.16, 70, 0.006)
	needs := []Need{Text(paper)}
	for _, h := range []float64{25, 145, 195, 250} {
		receding, ok1 := Solve(paper, h, 0.9, AAText, true)
		vivid, ok2 := SolveVivid(needs, h, 0.9)
		if !ok1 || !ok2 {
			t.Fatalf("H=%.0f: no solution", h)
		}
		if vivid.C <= receding.C {
			t.Errorf("H=%.0f: vivid chroma %.3f is not above receding %.3f", h, vivid.C, receding.C)
		}
		if c := Contrast(vivid, paper); c < AAText {
			t.Errorf("H=%.0f: vivid stop is %.2f:1, below its bar", h, c)
		}
	}
}

// TestOverCompositesInLinearLight guards the wash used for the ghost button.
// A 0-alpha composite is the backdrop and a 1-alpha composite is the color;
// anything in between has to sit between them in luminance.
func TestOverCompositesInLinearLight(t *testing.T) {
	paper := Grey(0.16, 70, 0.006)
	accent := OKLCH{L: 0.7, C: 0.15, H: 200}
	if got := accent.Over(paper, 0); math.Abs(got.Luminance()-paper.Luminance()) > 1e-6 {
		t.Error("alpha 0 should be the backdrop")
	}
	if got := accent.Over(paper, 1); math.Abs(got.Luminance()-accent.Luminance()) > 1e-6 {
		t.Error("alpha 1 should be the color")
	}
	mid := accent.Over(paper, 0.5).Luminance()
	if mid <= paper.Luminance() || mid >= accent.Luminance() {
		t.Errorf("a half-alpha wash landed at luminance %.4f, outside its endpoints", mid)
	}
}

// TestMixTakesTheShortWayRound guards the classic hue-interpolation bug.
func TestMixTakesTheShortWayRound(t *testing.T) {
	a := OKLCH{L: 0.6, C: 0.15, H: 350}
	b := OKLCH{L: 0.6, C: 0.15, H: 10}
	// The short way is 20° forward through 0, not 340° back through green.
	got := Mix(a, b, 0.5).H
	if !(got > 359.5 || got < 0.5) {
		t.Errorf("mixing H=350 and H=10 landed at H=%.1f; expected it to pass through 0", got)
	}
}

// TestCSSFormatting keeps the emitted values terse and valid.
func TestCSSFormatting(t *testing.T) {
	cases := map[OKLCH]string{
		{L: 0.5, C: 0, H: 0}:              "oklch(0.5 0 0)",
		{L: 0.7231, C: 0.1542, H: 205.25}: "oklch(0.7231 0.1542 205.25)",
		{L: 1, C: 0, H: 0}:                "oklch(1 0 0)",
	}
	for in, want := range cases {
		if got := in.CSS(); got != want {
			t.Errorf("CSS() = %q, want %q", got, want)
		}
	}
}
