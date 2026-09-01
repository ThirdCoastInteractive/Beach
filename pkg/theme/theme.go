// Package theme derives Beach's design tokens — every colour the kit paints,
// in both a light and a dark scheme — from a short list of parameters.
//
// The palette is computed, not picked. That is a reaction to how the framework's
// first one went: twenty-odd stops chosen by hand against a contrast matrix, then
// nudged until the tests passed. The result was a primary accent of #3d7c82, a
// muted teal nobody wanted, arrived at because white label text needs 4.5:1 and a
// focus ring needs 3:1 and by the time both held there was nothing left. The
// dullness was not a taste failure. It was the constraints, applied by hand,
// eating the colour.
//
// So the obligations come first. Each token declares the backdrops it will be
// seen against and the ratio it owes there ([color.Need]), and the solver returns
// the colour that satisfies all of them with the most of its hue intact.
// Contrast stops being something a palette is graded on afterwards and becomes
// the thing it is built from.
//
// It sits beside pkg/prefs, and for the same reason: the HTTP layer resolves a
// theme and the component kit has to read one, and the kit cannot import the
// HTTP layer.
package theme

import (
	"fmt"

	"github.com/ThirdCoastInteractive/Beach/pkg/color"
)

// SeriesCount is how many chart series slots the token vocabulary defines —
// --color-series-a through --color-series-o, one per letter.
const SeriesCount = 15

// SchemeParams is the lightness ladder for one scheme.
//
// It is per-scheme because a light theme is **not an inverted dark one**. Light
// interfaces use tighter value steps, tolerate far less tint before reading as
// dirty, and conventionally put the card *above* the page rather than below it —
// white panels on a soft grey ground, where a dark theme does the reverse. Flip
// a dark ladder and you get a light theme that looks like a photographic
// negative of a good one.
type SchemeParams struct {
	// Dark says which way the ink goes. It drives every solver call in the
	// scheme, so it is a declaration rather than something inferred from the
	// lightness values — a ladder in the middle of the range would be ambiguous.
	Dark bool

	// The surface ladder, page first.
	PaperL, PanelL, PanelHoverL, LineSoftL float64

	// The two pinned ink stops. Primary ink is the page's speaking voice and
	// there is nothing to gain by dimming it toward its own floor, so unlike the
	// muted stops it is placed rather than solved — and then checked.
	FgStrongL, FgDefaultL float64

	// NeutralChroma is the whisper of colour in the surfaces and the muted ink.
	// At exactly zero a dark surface reads as the absence of light rather than
	// as a material; much above a hundredth and the mid-tones turn tan.
	NeutralChroma float64

	// SeriesL is the single lightness every chart series sits at. One lightness
	// across all fifteen is what makes them read as equally prominent — the
	// balance a per-series solve would destroy.
	SeriesL float64
}

// Params is a complete theme specification: hues, one chroma fraction, and the
// two ladders.
type Params struct {
	// NeutralHue tints the surfaces and the muted ink.
	NeutralHue float64

	// The accent and the two category hues. The categories are picked for
	// separation from the accent and from each other, since their whole job is
	// to be told apart.
	AccentHue, Accent2Hue, Accent3Hue float64

	// The four role hues. These are the only values in the struct that are not a
	// taste decision — convention is the entire reason they work, since a
	// visitor reads green as good before reading anything else on the page. The
	// theme decides what *kind* of green, never whether it is green.
	GoodHue, WarnHue, BadHue, InfoHue float64

	// ChromaPct is how much of each hue's available chroma to spend, 0–1.
	//
	// A fraction, never an absolute. Maximum chroma varies by roughly 3x across
	// hues — at L=0.5 purple reaches 0.29 and cyan manages 0.09 — so one
	// absolute chroma asks something easy of purple and something impossible of
	// cyan, and the cyan comes back grey. Equal fractions read as equally vivid.
	ChromaPct float64

	// WashAlpha is how much accent goes into the ghost button's hover tint.
	WashAlpha float64

	Dark, Light SchemeParams
}

// Scheme is one fully derived token set.
type Scheme struct {
	Paper, Panel, PanelHover color.OKLCH
	FgStrong, FgDefault      color.OKLCH
	FgMuted, FgDisabled      color.OKLCH
	LineSoft, LineStrong     color.OKLCH

	Accent, AccentHover, AccentText, AccentSoft, OnAccent color.OKLCH
	Accent2, Accent3                                      color.OKLCH

	Good, Warn, Bad, Info color.OKLCH

	Series [SeriesCount]color.OKLCH
}

// Theme is a preset resolved into both schemes.
type Theme struct {
	Preset Preset
	Dark   Scheme
	Light  Scheme
}

// Build derives both schemes. An error names the token and the obligation it
// could not meet, so an unusable parameter set fails where it is chosen rather
// than shipping a palette that fails the build.
func Build(p Params) (Theme, error) {
	dark, err := BuildScheme(p, p.Dark)
	if err != nil {
		return Theme{}, fmt.Errorf("dark scheme: %w", err)
	}
	light, err := BuildScheme(p, p.Light)
	if err != nil {
		return Theme{}, fmt.Errorf("light scheme: %w", err)
	}
	return Theme{Dark: dark, Light: light}, nil
}

// BuildScheme derives one scheme.
func BuildScheme(p Params, sp SchemeParams) (Scheme, error) {
	var s Scheme
	up := sp.Dark // ink and accents move away from the surfaces

	neutral := func(l float64) color.OKLCH {
		return color.Grey(l, p.NeutralHue, sp.NeutralChroma)
	}

	// --- surfaces -----------------------------------------------------------
	// The one genuinely aesthetic choice in the file. Surfaces owe no ratio of
	// their own; they set what everything below has to clear.
	s.Paper = neutral(sp.PaperL)
	s.Panel = neutral(sp.PanelL)
	s.PanelHover = neutral(sp.PanelHoverL)
	s.LineSoft = neutral(sp.LineSoftL) // decorative hairline: no obligation

	surfaces := []color.OKLCH{s.Paper, s.Panel, s.PanelHover}
	onSurfaces := make([]color.Need, 0, 3)
	overSurfaces := make([]color.Need, 0, 3)
	for _, bg := range surfaces {
		onSurfaces = append(onSurfaces, color.Text(bg))
		overSurfaces = append(overSurfaces, color.NonText(bg))
	}

	// --- ink ----------------------------------------------------------------
	s.FgStrong = neutral(sp.FgStrongL)
	s.FgDefault = neutral(sp.FgDefaultL)
	if n, bad := color.Unmet(s.FgDefault, onSurfaces); bad {
		return s, fmt.Errorf("primary ink is %.2f:1 on %s, WCAG 1.4.3 needs %.1f:1 — "+
			"move FgDefaultL further from the surface ladder",
			color.Contrast(s.FgDefault, n.Bg), n.Bg.Hex(), n.Ratio)
	}

	// Muted ink, disabled ink and the strong edge all recede, so each takes the
	// stop *closest to the surfaces* that still clears its bar. Solved for
	// distance instead they would all arrive as second primary inks.
	var ok bool
	if s.FgMuted, ok = color.SolveAgainstAllFixed(onSurfaces, p.NeutralHue, sp.NeutralChroma, up); !ok {
		return s, unmet("muted text", "4.5:1 on every surface")
	}
	if s.FgDisabled, ok = color.SolveAgainstAllFixed(overSurfaces, p.NeutralHue, sp.NeutralChroma, up); !ok {
		return s, unmet("disabled text", "3:1 on every surface")
	}
	// An input's edge identifies the control, which makes it a component
	// boundary under SC 1.4.11 rather than decoration.
	if s.LineStrong, ok = color.SolveAgainstAllFixed(overSurfaces, p.NeutralHue, sp.NeutralChroma, up); !ok {
		return s, unmet("input edge", "3:1 on every surface")
	}

	// --- accent -------------------------------------------------------------
	// The ink on an accent fill is the page colour itself, in both schemes.
	//
	// That is not a shortcut, it is the constraint resolving. On a dark page the
	// accent also draws the focus ring, so it has to be *light* enough to clear
	// 3:1 against a raised panel — which leaves dark ink as the only legible
	// label. On a light page the ring obligation pushes the accent the other
	// way, and light ink is the only one that works. Either way the answer is
	// the paper colour, so the fill reads as the page punched through the
	// accent. The old palette pinned white ink in both directions and paid for
	// it with all of the accent's chroma.
	s.OnAccent = s.Paper

	accentNeeds := append([]color.Need{color.Text(s.Paper)}, color.NonText(s.Panel), color.NonText(s.PanelHover))
	if s.Accent, ok = color.SolveVivid(accentNeeds, p.AccentHue, p.ChromaPct); !ok {
		return s, unmet("accent fill", "4.5:1 for its label and 3:1 as a focus ring")
	}

	// Hover moves the fill *away* from its ink, never toward it, so the label's
	// ratio can only improve — SC 1.4.3 applies to text as displayed, hover
	// state included, and this makes that free rather than a second check.
	hover := s.Accent
	if up {
		hover.L += 0.06
	} else {
		hover.L -= 0.06
	}
	hover.C = p.ChromaPct * color.MaxChroma(hover.L, p.AccentHue)
	if color.Contrast(s.OnAccent, hover) < color.AAText {
		hover = s.Accent // the accent is already at the edge of its range
	}
	s.AccentHover = hover

	// The ghost button's wash, settled before the link colour because it is one
	// of the backdrops that colour has to clear. It is stored composited: a
	// ratio can only be measured against an opaque value.
	s.AccentSoft = s.Accent.Over(s.Paper, p.WashAlpha)

	// The accent as a link — a different job from the fill, so a different stop,
	// with three backdrops rather than two.
	linkNeeds := []color.Need{color.Text(s.Paper), color.Text(s.Panel), color.Text(s.AccentSoft)}
	if s.AccentText, ok = color.SolveVivid(linkNeeds, p.AccentHue, p.ChromaPct); !ok {
		return s, unmet("link text", "4.5:1 on the page, on a card and on the ghost wash")
	}

	// --- categories and roles -----------------------------------------------
	// All six are painted as text somewhere — a badge label, an alert body, a
	// legend entry — so all six are held to the text bar, and all six are solved
	// for presence. Held to recession alongside the neutrals, "good" arrives as
	// olive and "warn" as mud: a status colour that only just clears its ratio
	// has spent all its colour on the ratio.
	for _, f := range []struct {
		name string
		hue  float64
		dst  *color.OKLCH
	}{
		{"secondary category", p.Accent2Hue, &s.Accent2},
		{"tertiary category", p.Accent3Hue, &s.Accent3},
		{"success", p.GoodHue, &s.Good},
		{"warning", p.WarnHue, &s.Warn},
		{"error", p.BadHue, &s.Bad},
		{"info", p.InfoHue, &s.Info},
	} {
		c, found := color.SolveVivid(onSurfaces, f.hue, p.ChromaPct)
		if !found {
			return s, unmet(f.name, "4.5:1 on every surface")
		}
		*f.dst = c
	}

	// --- series -------------------------------------------------------------
	// One lightness and one chroma fraction across all fifteen hues. Equal
	// lightness is what makes them read as equally prominent; solving each hue
	// separately would satisfy the same ratio and destroy the balance, which is
	// the thing that never worked with the old cube.
	//
	// A series colour is the entire encoding of a line or a wedge, so one that
	// sinks into the page takes its data with it (SC 1.4.11). If the chosen
	// lightness cannot carry every hue over that bar, the whole row moves
	// together rather than one member breaking rank.
	seriesL := sp.SeriesL
	step := 0.01
	if !up {
		step = -0.01
	}
	for range 60 {
		if seriesClears(seriesL, p, s.Paper) {
			break
		}
		seriesL += step
	}
	if !seriesClears(seriesL, p, s.Paper) {
		return s, unmet("the chart series", "3:1 on the page at a single shared lightness")
	}
	for i := range s.Series {
		h := 360 * float64(i) / float64(SeriesCount)
		s.Series[i] = color.OKLCH{L: seriesL, C: p.ChromaPct * color.MaxChroma(seriesL, h), H: h}
	}

	return s, nil
}

// seriesClears reports whether every series hue meets 3:1 on the page at one
// shared lightness.
func seriesClears(l float64, p Params, paper color.OKLCH) bool {
	if l <= 0.02 || l >= 0.99 {
		return false
	}
	for i := range SeriesCount {
		h := 360 * float64(i) / float64(SeriesCount)
		c := color.OKLCH{L: l, C: p.ChromaPct * color.MaxChroma(l, h), H: h}
		if color.Contrast(c, paper) < color.AANonText {
			return false
		}
	}
	return true
}

func unmet(what, duty string) error {
	return fmt.Errorf("no colour on this hue gives %s %s — move the hue, the surface ladder, or the chroma fraction", what, duty)
}
