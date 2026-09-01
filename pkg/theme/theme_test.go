package theme

import (
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/Beach/pkg/color"
)

// The palette contract, as a test.
//
// Contrast is a property of pairs, not of colours, so a token sheet cannot be
// checked by looking at it. The derivation in theme.go claims to satisfy every
// obligation by construction; this is what holds it to that, across every preset
// and both schemes — because a preset nobody has looked at is exactly where a
// failure would sit unnoticed.

func TestEveryPresetDerives(t *testing.T) {
	for _, p := range Presets {
		if _, err := Build(p.Params); err != nil {
			t.Errorf("%s (%s): %v", p.Key, p.Title, err)
		}
	}
}

func TestEveryPairMeetsAA(t *testing.T) {
	for _, p := range Presets {
		theme, err := Build(p.Params)
		if err != nil {
			continue // TestEveryPresetDerives reports this
		}
		for _, s := range []struct {
			name   string
			scheme Scheme
		}{{"dark", theme.Dark}, {"light", theme.Light}} {
			for _, pair := range s.scheme.Pairs() {
				if !pair.Passes() {
					t.Errorf("%s/%s: %s is %.2f:1, WCAG %s needs %.1f:1 (%s on %s)",
						p.Key, s.name, pair.What, pair.Ratio(), pair.Criterion, pair.Min,
						pair.Fg.Hex(), pair.Bg.Hex())
				}
			}
		}
	}
}

// TestChartSeriesAreDistinguishable holds the series ramp to the non-text bar in
// both schemes. A ramp legible on a near-black page is not automatically legible
// on a near-white one, which is the failure light mode invites.
//
// Note what this does not claim: 3:1 against the page makes each series visible,
// not distinguishable from its neighbours. Telling two series apart by colour
// alone is beyond what a ratio can check, which is why the kit pairs colour with
// a second encoding wherever it can.
func TestChartSeriesAreDistinguishable(t *testing.T) {
	for _, p := range Presets {
		theme, err := Build(p.Params)
		if err != nil {
			continue
		}
		for _, s := range []struct {
			name   string
			scheme Scheme
		}{{"dark", theme.Dark}, {"light", theme.Light}} {
			for _, pair := range s.scheme.SeriesPairs() {
				if !pair.Passes() {
					t.Errorf("%s/%s: %s is %.2f:1 on the page, WCAG 1.4.11 needs %.1f:1",
						p.Key, s.name, pair.What, pair.Ratio(), pair.Min)
				}
			}
		}
	}
}

// TestSeriesShareOneLightness is the balance rule. Fifteen colours at one
// lightness read as equally prominent; solved per hue they would satisfy the
// same ratio and arrive as a ramp where some members shout. That balance is
// invisible to a contrast check, so it needs its own test.
func TestSeriesShareOneLightness(t *testing.T) {
	for _, p := range Presets {
		theme, err := Build(p.Params)
		if err != nil {
			continue
		}
		for _, s := range []struct {
			name   string
			scheme Scheme
		}{{"dark", theme.Dark}, {"light", theme.Light}} {
			want := s.scheme.Series[0].L
			for i, c := range s.scheme.Series {
				if c.L != want {
					t.Errorf("%s/%s: series %c sits at L=%.3f, the ramp is at L=%.3f",
						p.Key, s.name, 'a'+i, c.L, want)
				}
			}
		}
	}
}

// TestChromaticsUseTheSameChromaFraction is the other half of the balance rule,
// and the one that fixes the specific complaint that sank the RYB palette:
// "good" arriving as olive while "warn" arrives loud.
//
// Equal fractions of each hue's own maximum is what makes them read as equally
// vivid. Equal *absolute* chroma does the opposite, because the maximum varies
// by roughly 3x across hues.
func TestChromaticsUseTheSameChromaFraction(t *testing.T) {
	p, _ := ByKey(Default)
	theme, err := Build(p.Params)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct {
		name   string
		scheme Scheme
	}{{"dark", theme.Dark}, {"light", theme.Light}} {
		for _, c := range []struct {
			name string
			col  color.OKLCH
		}{
			{"good", s.scheme.Good}, {"warn", s.scheme.Warn},
			{"bad", s.scheme.Bad}, {"info", s.scheme.Info},
		} {
			max := color.MaxChroma(c.col.L, c.col.H)
			if max <= 0 {
				t.Errorf("%s/%s: no chroma available at L=%.3f", s.name, c.name, c.col.L)
				continue
			}
			got := c.col.C / max
			if got < p.ChromaPct-0.02 || got > p.ChromaPct+0.02 {
				t.Errorf("%s/%s: uses %.0f%% of its hue's max chroma, the theme asks for %.0f%%",
					s.name, c.name, got*100, p.ChromaPct*100)
			}
		}
	}
}

// TestBothSchemesDefineTheSameTokens guards the one failure mode a three-state
// stylesheet has: a token defined in one scheme and missing from the other does
// not fall back, it *inherits* — so switching to light would leave one value
// painted from the dark block.
func TestBothSchemesDefineTheSameTokens(t *testing.T) {
	want := TokenNames()
	for _, p := range Presets {
		theme, err := Build(p.Params)
		if err != nil {
			continue
		}
		for _, s := range []struct {
			name   string
			scheme Scheme
		}{{"dark", theme.Dark}, {"light", theme.Light}} {
			tok := s.scheme.Tokens()
			if len(tok) != len(want) {
				t.Errorf("%s/%s: defines %d tokens, want %d", p.Key, s.name, len(tok), len(want))
			}
			for _, name := range want {
				if _, ok := tok[name]; !ok {
					t.Errorf("%s/%s: missing %s", p.Key, s.name, name)
				}
			}
		}
	}
}

// TestAccentInkIsThePaperColour records the decision that unlocked the palette.
//
// The previous theme pinned white label text on the accent in both directions,
// which forced the accent dark enough to carry white *and* light enough to draw
// a focus ring — a window narrow enough that nothing colourful fit through it.
// Using the page's own colour as the ink lets each scheme push the accent to
// whichever end its focus-ring obligation demands.
func TestAccentInkIsThePaperColour(t *testing.T) {
	for _, p := range Presets {
		theme, err := Build(p.Params)
		if err != nil {
			continue
		}
		for _, s := range []struct {
			name   string
			scheme Scheme
		}{{"dark", theme.Dark}, {"light", theme.Light}} {
			if s.scheme.OnAccent != s.scheme.Paper {
				t.Errorf("%s/%s: accent ink is not the paper colour", p.Key, s.name)
			}
		}
		// And the consequence: on a dark page the accent is lighter than the
		// page, on a light page it is darker. That is the ring obligation
		// showing through, and it is why the two schemes are derived separately
		// rather than one being inverted from the other.
		if theme.Dark.Accent.L <= theme.Dark.Paper.L {
			t.Errorf("%s: dark accent is not lighter than its page", p.Key)
		}
		if theme.Light.Accent.L >= theme.Light.Paper.L {
			t.Errorf("%s: light accent is not darker than its page", p.Key)
		}
	}
}

// TestHoverMovesAwayFromItsInk. SC 1.4.3 applies to text as displayed, hover
// state included. Moving the fill away from its label rather than toward it
// makes the hovered ratio strictly better than the resting one, which is why the
// derivation needs no separate hover check.
func TestHoverMovesAwayFromItsInk(t *testing.T) {
	for _, p := range Presets {
		theme, err := Build(p.Params)
		if err != nil {
			continue
		}
		for _, s := range []struct {
			name   string
			scheme Scheme
		}{{"dark", theme.Dark}, {"light", theme.Light}} {
			rest := color.Contrast(s.scheme.OnAccent, s.scheme.Accent)
			hover := color.Contrast(s.scheme.OnAccent, s.scheme.AccentHover)
			if hover < rest-0.001 {
				t.Errorf("%s/%s: hovering drops the label from %.2f:1 to %.2f:1",
					p.Key, s.name, rest, hover)
			}
		}
	}
}

// TestCSSCoversAllThreeStates checks the emitted block answers every state a
// visitor can be in. The :not() guard is the one that gets forgotten, and
// forgetting it produces a toggle that works in one direction only.
func TestCSSCoversAllThreeStates(t *testing.T) {
	theme, err := BuildPreset(Default)
	if err != nil {
		t.Fatal(err)
	}
	css := theme.CSS()
	for _, want := range []string{
		":root {",
		"@media (prefers-color-scheme: light)",
		`:root:not([data-theme="dark"])`,
		`:root[data-theme="light"]`,
		`:root[data-theme="dark"]`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("emitted CSS is missing %q", want)
		}
	}
	// Every token appears in all four blocks.
	for _, name := range TokenNames() {
		if got := strings.Count(css, name+":"); got != 4 {
			t.Errorf("%s appears %d times, want 4 (base, media, light, dark)", name, got)
		}
	}
}

func TestUnknownPresetIsAnError(t *testing.T) {
	if _, err := BuildPreset("no-such-preset"); err == nil {
		t.Fatal("expected an error for an unknown preset")
	}
}

// BenchmarkBuild is the number that decides whether a theme can be derived on a
// request or has to be cached. The RYB derivation this replaced measured about
// 4.5ms, which was firmly in "cache it" territory.
func BenchmarkBuild(b *testing.B) {
	p, _ := ByKey(Default)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Build(p.Params); err != nil {
			b.Fatal(err)
		}
	}
}
