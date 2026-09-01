package rybitten

import (
	"fmt"
	"math"
	"strings"
)

// Theme derivation — the framework's whole token vocabulary as a function of one
// gamut.
//
// The alternative, and what Beach did first, is to hand-pick the stops and
// hold them with a contrast test. That works, but it inverts the effort: every
// token is chosen by guessing, pasting, running the test, and nudging. Worse, it
// makes the constraints invisible in the result. The accent sat at a muted
// #3d7c82 for months not because anyone wanted a muted teal, but because white
// label text needs 4.5:1 and a focus ring needs 3:1, and by the time both were
// satisfied there was nothing left. The dullness was a side effect nobody chose.
//
// So this file does the opposite. Each token names the pairs it owes, and the
// derivation searches the gamut's lightness axis for the stop that satisfies them
// with the most color left over. Contrast stops being something the palette is
// checked against afterwards and becomes the thing it is built from — and
// swapping the entire look becomes one gamut key.

// ThemeOptions steers a derivation: where on the RYB hue wheel each family is
// sampled, and how dark the surface ladder sits.
//
// Hues are RYB degrees, not RGB ones. The wheel runs 0 red, 60 orange, 120
// yellow, 180 green, 240 blue, 300 violet — a painter's wheel, because that is
// what the cube interpolates (see [RYBHSL2RGB]).
type ThemeOptions struct {
	// NeutralHue tints the surface and ink ladder. A neutral at exactly zero
	// saturation is the cold digital grey the kit is trying to get away from; a
	// few percent of a warm hue is what makes a dark surface read as a material
	// rather than as an absence of light.
	NeutralHue float64
	NeutralSat float64

	// Surface lightnesses, darkest first. These are the one genuinely aesthetic
	// choice in the file — they owe no ratio of their own, they only set what
	// the ink and edge searches below have to clear.
	PaperL, PanelL, PanelHoverL, LineSoftL float64

	// Hues for the accent family and the four semantic roles. The roles keep
	// their conventional meaning — green still means good — and take only the
	// gamut's character, never its hue assignments.
	AccentHue, Accent2Hue, Accent3Hue float64
	GoodHue, WarnHue, BadHue, InfoHue float64

	// Sat is the saturation the chromatic families are sampled at. Full
	// saturation is the point of using a gamut at all; the cube, not a
	// desaturation, is what keeps the colors from screaming.
	Sat float64

	// WashAlpha is how much accent goes into the ghost-button hover tint. It is
	// an input rather than a searched value because the tint is a background:
	// fix it first, then derive the link color that has to sit on it. Deriving
	// them the other way round — strongest tint the existing link color
	// survives — makes the wash hostage to a stop chosen for a different job,
	// and on most gamuts the answer comes back "no tint at all".
	WashAlpha float64
}

// DefaultThemeOptions is Beach's own derivation: a warm sand-tinted neutral
// ladder, a tidepool accent where green turns to blue, and a coral/violet
// category pair chosen for distance from the accent and from each other.
//
// The four role hues are the only values here that are not a taste decision.
// They are conventional, and convention is the whole reason they work: a
// visitor reads green as good before they read anything else on the page, so
// the gamut gets to decide what *kind* of green, never whether it is green.
func DefaultThemeOptions() ThemeOptions {
	return ThemeOptions{
		NeutralHue: 45, // between red and yellow — sand, driftwood, wet rope
		// Enough warmth that a surface reads as a material rather than as an
		// absence of light, and not so much that secondary text turns tan.
		NeutralSat: 0.14,

		PaperL:      0.055,
		PanelL:      0.105,
		PanelHoverL: 0.170,
		LineSoftL:   0.195,

		AccentHue: 205, // green toward blue: sea glass, shallow water
		// The category pair is picked for separation, not for warmth: coral sits
		// opposite the accent, and violet is the furthest hue from both that is
		// also clear of all four role hues — a gold third would sit 20° from
		// warn, which is exactly the confusion category coding must not have.
		Accent2Hue: 25,
		Accent3Hue: 300,

		GoodHue: 180,
		WarnHue: 95,
		BadHue:  5,
		InfoHue: 235,

		Sat:       1.0,
		WashAlpha: 0.18,
	}
}

// Theme is a fully derived token set: every color the kit paints, in the order
// the stylesheet declares them.
type Theme struct {
	Gamut Gamut
	Opts  ThemeOptions

	Paper, Panel, PanelHover RGB
	FgStrong, FgDefault      RGB
	FgMuted, FgDisabled      RGB
	LineSoft, LineStrong     RGB

	Accent, AccentHover, AccentText, AccentSoft, OnAccent RGB
	Accent2, Accent3                                      RGB
	Accent2Light, Accent3Light                            RGB

	Good, Warn, Bad, Info RGB

	Series [SeriesCount]RGB

	// OnAccentIsInk records which way the accent pairing resolved: true when the
	// label on an accent fill is the dark paper ink, false when it is white. It
	// is the single decision that decides whether the accent can be bright.
	OnAccentIsInk bool
}

// WCAG 2.1 AA, the two thresholds the kit holds itself to. Text owes 4.5:1
// (SC 1.4.3); a component boundary or focus indicator owes 3:1 (SC 1.4.11).
const (
	aaText    = 4.5
	aaNonText = 3.0
	// aaMargin keeps a derived stop off the exact threshold. The result is
	// written at 8 bits per channel, so a stop measuring 4.500 in float can
	// measure 4.497 once rounded — and ship a failing build.
	aaMargin = 0.08
)

// BuildTheme derives a complete [Theme] from a gamut.
//
// Every search here walks the lightness axis, but the two families optimize for
// opposite things once they get there. A neutral — muted ink, an input edge — is
// meant to recede, so it takes the dimmest stop that still meets its ratio;
// taking the brightest passing stop would satisfy the same tests and produce a
// palette of near-whites. A chromatic — the accent, the four roles — is meant to
// be present, so it takes the most colorful stop instead. Held to recession
// alongside the neutrals, "good" comes out olive and "warn" comes out mud: a
// status color that only just clears its ratio has spent all its color on the
// ratio.
//
// An error names the pair that could not be satisfied, so an unusable gamut
// fails at generation rather than shipping a theme that fails the build.
func BuildTheme(g Gamut, o ThemeOptions) (Theme, error) {
	t := Theme{Gamut: g, Opts: o}

	neutral := func(l float64) RGB {
		return RYBHSL2RGB([3]float64{o.NeutralHue, o.NeutralSat, l}, g.Cube, true)
	}
	chromatic := func(h, l float64) RGB {
		return RYBHSL2RGB([3]float64{h, o.Sat, l}, g.Cube, true)
	}

	// --- surfaces -----------------------------------------------------------
	t.Paper = neutral(o.PaperL)
	t.Panel = neutral(o.PanelL)
	t.PanelHover = neutral(o.PanelHoverL)
	t.LineSoft = neutral(o.LineSoftL)
	surfaces := []RGB{t.Paper, t.Panel, t.PanelHover}

	// --- ink ----------------------------------------------------------------
	// Primary ink is pinned bright: it is the page's speaking voice, and there
	// is nothing to gain by dimming it toward its own floor.
	t.FgStrong = neutral(0.98)
	t.FgDefault = neutral(0.93)
	if err := requireAll("body text", t.FgDefault, surfaces, aaText); err != nil {
		return t, err
	}

	// Muted ink, disabled ink and the strong edge are the stops worth searching:
	// all three are meant to recede, and the dimmest legal value recedes most.
	var ok bool
	t.FgMuted, ok = dimmest(0.30, 0.92, neutral, clearsAll(surfaces, aaText+aaMargin))
	if !ok {
		return t, unsatisfiable("muted text", "4.5:1 on paper, panel and panel-hover")
	}
	t.FgDisabled, ok = dimmest(0.20, 0.90, neutral, clearsAll(surfaces, aaNonText+aaMargin))
	if !ok {
		return t, unsatisfiable("disabled text", "3:1 on paper, panel and panel-hover")
	}
	// An input's edge is what identifies the control, so it is a component
	// boundary under SC 1.4.11 and owes 3:1 on every surface it is drawn on.
	t.LineStrong, ok = dimmest(0.20, 0.90, neutral, clearsAll(surfaces, aaNonText+aaMargin))
	if !ok {
		return t, unsatisfiable("input edge", "3:1 on paper, panel and panel-hover")
	}

	// --- accent -------------------------------------------------------------
	// The pairing decision, made by measurement rather than by fiat.
	//
	// A solid accent fill carries label text, so the accent's lightness is
	// bounded by whatever ink goes on it; the same color draws the focus ring,
	// so it is bounded from the other side by the surfaces. With white ink the
	// two bounds nearly meet, which is what flattened the old accent. With the
	// page's own dark ink the window opens in the other direction — the accent
	// gets to be bright, and bright is where the chroma is. Try both, keep
	// whichever leaves more color in the result.
	accentAt := func(l float64) RGB { return chromatic(o.AccentHue, l) }
	bestChroma := -1.0
	for _, cand := range []struct {
		ink   RGB
		isInk bool
	}{{t.Paper, true}, {RGB{1, 1, 1}, false}} {
		c, l, found := mostColorful(0.10, 0.95, accentAt, func(c RGB) bool {
			return Contrast(cand.ink, c) >= aaText+aaMargin && clearsAll(surfaces, aaNonText+aaMargin)(c)
		})
		if !found {
			continue
		}
		// Hover moves the fill away from its ink rather than toward it, so the
		// label's ratio only ever improves on hover — SC 1.4.3 applies to text
		// as displayed, hover state included, and this makes that free.
		hoverL := l + 0.06
		if !cand.isInk {
			hoverL = l - 0.06
		}
		hover := accentAt(hoverL)
		if Contrast(cand.ink, hover) < aaText+aaMargin {
			continue
		}
		if ch := chroma(c); ch > bestChroma {
			bestChroma = ch
			t.Accent, t.AccentHover, t.OnAccent, t.OnAccentIsInk = c, hover, cand.ink, cand.isInk
		}
	}
	if bestChroma < 0 {
		return t, unsatisfiable("accent fill", "4.5:1 for its label and 3:1 as a focus ring")
	}

	// The ghost button's hover wash: the accent composited over paper. It is
	// emitted as an opaque stop rather than an alpha because a ratio can only be
	// measured against a composited color, and it is settled *before* the link
	// color because it is one of the backgrounds that color has to clear.
	t.AccentSoft = t.Accent.Over(t.Paper, o.WashAlpha)

	// The accent as a link or as text on a dark surface — a different job from
	// the fill, and so a different stop. It has three backdrops, not two: the
	// page, a card, and the ghost button's own hover wash.
	t.AccentText, _, ok = mostColorful(0.30, 0.95, accentAt,
		clearsAll([]RGB{t.Paper, t.Panel, t.AccentSoft}, aaText+aaMargin))
	if !ok {
		return t, unsatisfiable("link text", "4.5:1 on paper, on a card and on the ghost-button wash")
	}

	// --- category accents and roles -----------------------------------------
	// All six are painted as text somewhere in the kit — a badge label, an alert
	// body, a legend entry — so all six are held to the text bar. Unlike the
	// neutrals they are searched for the *most colorful* legal stop rather than
	// the dimmest one: a status color that only just clears its ratio reads as
	// muddy, and "good" rendered in olive is a worse answer than a green one
	// with contrast to spare.
	for _, f := range []struct {
		name string
		hue  float64
		dst  *RGB
	}{
		{"secondary accent", o.Accent2Hue, &t.Accent2},
		{"tertiary accent", o.Accent3Hue, &t.Accent3},
		{"success", o.GoodHue, &t.Good},
		{"warning", o.WarnHue, &t.Warn},
		{"error", o.BadHue, &t.Bad},
		{"info", o.InfoHue, &t.Info},
	} {
		at := func(l float64) RGB { return chromatic(f.hue, l) }
		v, _, found := mostColorful(0.25, 0.95, at, clearsAll(surfaces, aaText+aaMargin))
		if !found {
			return t, unsatisfiable(f.name, "4.5:1 on paper, panel and panel-hover")
		}
		*f.dst = v
	}
	// The role solid buttons put paper ink on the role fill. That pair had no
	// token check before this file existed, because neither color was chosen
	// with it in mind.
	for _, r := range []struct {
		name string
		c    RGB
	}{{"success", t.Good}, {"warning", t.Warn}, {"error", t.Bad}, {"info", t.Info}} {
		if got := Contrast(t.Paper, r.c); got < aaText {
			return t, fmt.Errorf("rybitten: the %s button label is %.2f:1 on its own fill in gamut %q, WCAG 1.4.3 needs %.1f:1",
				r.name, got, g.Key, aaText)
		}
	}

	// The two category accents also appear against a light surface, where the
	// same stop would be invisible. Their light-scheme partners are the same
	// hues held to the same bar against white.
	white := RGB{1, 1, 1}
	for _, f := range []struct {
		name string
		hue  float64
		dst  *RGB
	}{
		{"secondary accent (light)", o.Accent2Hue, &t.Accent2Light},
		{"tertiary accent (light)", o.Accent3Hue, &t.Accent3Light},
	} {
		at := func(l float64) RGB { return chromatic(f.hue, l) }
		v, found := brightest(0.10, 0.80, at, clearsAll([]RGB{white}, aaText+aaMargin))
		if !found {
			return t, unsatisfiable(f.name, "4.5:1 on white")
		}
		*f.dst = v
	}

	// --- series -------------------------------------------------------------
	// A series color is the entire encoding of a line or a wedge, so one that
	// sinks into the page takes its data with it (SC 1.4.11). The hue wheel is
	// sampled as usual, then any stop landing under the bar is walked up its own
	// hue until it clears — which is what lets every gamut in [Cubes] be a
	// usable theme, not only the ones that happen to pass.
	for i, c := range Series(g, SeriesCount) {
		h := 360 * float64(i) / float64(SeriesCount)
		for l := 0.5; Contrast(c, t.Paper) < aaNonText+aaMargin && l <= 0.95; l += 0.01 {
			c = chromatic(h, l)
		}
		if Contrast(c, t.Paper) < aaNonText {
			return t, unsatisfiable(fmt.Sprintf("series %c", 'a'+i), "3:1 on paper")
		}
		t.Series[i] = c
	}

	return t, nil
}

// --- the search ----------------------------------------------------------------

// lightnessStep is the resolution of every search in this file. Finer than the
// 8-bit hex the result is written at, so the step is never what decides a stop.
const lightnessStep = 0.002

// dimmest returns the lowest lightness in [lo, hi] whose color satisfies ok —
// the deepest stop that still meets its contrast duty on a dark page. It is the
// objective for neutrals, which are meant to recede.
func dimmest(lo, hi float64, at func(float64) RGB, ok func(RGB) bool) (RGB, bool) {
	for l := lo; l <= hi; l += lightnessStep {
		if c := at(l); ok(c) {
			return c, true
		}
	}
	return RGB{}, false
}

// brightest is dimmest' mirror, for a stop that has to hold up against a light
// surface instead of a dark one.
func brightest(lo, hi float64, at func(float64) RGB, ok func(RGB) bool) (RGB, bool) {
	for l := hi; l >= lo; l -= lightnessStep {
		if c := at(l); ok(c) {
			return c, true
		}
	}
	return RGB{}, false
}

// mostColorful returns the feasible stop with the widest channel spread, and the
// lightness it was found at. Where dimmest optimizes for recession, this
// optimizes for presence — the objective for every chromatic token.
func mostColorful(lo, hi float64, at func(float64) RGB, ok func(RGB) bool) (RGB, float64, bool) {
	var best RGB
	bestL, bestC, found := 0.0, -1.0, false
	for l := lo; l <= hi; l += lightnessStep {
		c := at(l)
		if !ok(c) {
			continue
		}
		if ch := chroma(c); ch > bestC {
			best, bestL, bestC, found = c, l, ch, true
		}
	}
	return best, bestL, found
}

// chroma is the channel spread — a crude but honest stand-in for colorfulness,
// and one that needs no color-appearance model to compute.
func chroma(c RGB) float64 {
	c = c.Clamp()
	return math.Max(c[0], math.Max(c[1], c[2])) - math.Min(c[0], math.Min(c[1], c[2]))
}

// clearsAll builds a predicate: the color must reach min against every backdrop.
func clearsAll(bgs []RGB, min float64) func(RGB) bool {
	return func(c RGB) bool {
		for _, bg := range bgs {
			if Contrast(c, bg) < min {
				return false
			}
		}
		return true
	}
}

// requireAll asserts a pinned (unsearched) stop against its backdrops.
func requireAll(what string, c RGB, bgs []RGB, min float64) error {
	for _, bg := range bgs {
		if got := Contrast(c, bg); got < min {
			return fmt.Errorf("rybitten: %s is %.2f:1 against %s, WCAG needs %.1f:1", what, got, bg.Hex(), min)
		}
	}
	return nil
}

func unsatisfiable(what, duty string) error {
	return fmt.Errorf("rybitten: no stop on this gamut's axis gives %s %s — try another gamut, or shift its hue", what, duty)
}

// --- emission ------------------------------------------------------------------

// Tokens returns the theme as CSS custom-property name → hex, the same shape
// view.Tokens() parses back out of the stylesheet. Comparing the two is how a
// hand-edit of a generated stop gets caught.
func (t Theme) Tokens() map[string]string {
	m := map[string]string{
		"--color-paper":        t.Paper.Hex(),
		"--color-panel":        t.Panel.Hex(),
		"--color-panel-hover":  t.PanelHover.Hex(),
		"--color-fg-strong":    t.FgStrong.Hex(),
		"--color-fg-default":   t.FgDefault.Hex(),
		"--color-fg-muted":     t.FgMuted.Hex(),
		"--color-fg-disabled":  t.FgDisabled.Hex(),
		"--color-line-soft":    t.LineSoft.Hex(),
		"--color-line-strong":  t.LineStrong.Hex(),
		"--color-accent":       t.Accent.Hex(),
		"--color-accent-hover": t.AccentHover.Hex(),
		"--color-accent-text":  t.AccentText.Hex(),
		"--color-accent-soft":  t.AccentSoft.Hex(),
		"--color-on-accent":    t.OnAccent.Hex(),
		"--color-accent-2":     t.Accent2.Hex(),
		"--color-accent-3":     t.Accent3.Hex(),
		"--color-good":         t.Good.Hex(),
		"--color-warn":         t.Warn.Hex(),
		"--color-bad":          t.Bad.Hex(),
		"--color-info":         t.Info.Hex(),
	}
	for i, c := range t.Series {
		m[fmt.Sprintf("--color-series-%c", 'a'+i)] = c.Hex()
	}
	return m
}

// ThemeVars renders the theme as the stylesheet block the framework serves: the
// :root declarations plus the light-scheme category overrides, with the
// provenance and the pairing decision written into the comments. This is what
// cmd/beach-palette writes between the sentinels in input.css.
func ThemeVars(t Theme) string {
	var b strings.Builder

	ink := "white"
	if t.OnAccentIsInk {
		ink = "the page's own dark ink"
	}
	fmt.Fprintf(&b, "/* %s — %s (%d). Derived from rybitten gamut %q by cmd/beach-palette;\n", t.Gamut.Title, t.Gamut.Author, t.Gamut.Year, t.Gamut.Key)
	b.WriteString("   change the generator or its gamut, never these values — TestGeneratedPaletteMatchesInputCSS\n")
	b.WriteString("   re-derives the whole block and diffs it against this one.\n\n")
	b.WriteString("   Every stop is the dimmest one on the gamut's lightness axis that still meets its\n")
	b.WriteString("   WCAG 2.1 AA duty — 4.5:1 for text, 3:1 for a component edge or the focus ring — so\n")
	b.WriteString("   the palette carries as much of the gamut's color as those obligations leave room\n")
	fmt.Fprintf(&b, "   for. The accent's label ink resolved to %s. */\n", ink)

	b.WriteString(":root {\n")
	group := func(title string, rows [][3]string) {
		fmt.Fprintf(&b, "\n  /* %s */\n", title)
		for _, r := range rows {
			decl := fmt.Sprintf("  %s: %s;", r[0], r[1])
			if r[2] == "" {
				fmt.Fprintf(&b, "%s\n", decl)
				continue
			}
			fmt.Fprintf(&b, "%-38s/* %s */\n", decl, r[2])
		}
	}

	group("Surface ladder", [][3]string{
		{"--color-paper", t.Paper.Hex(), "page background"},
		{"--color-panel", t.Panel.Hex(), "card / chrome background"},
		{"--color-panel-hover", t.PanelHover.Hex(), "raised / hovered panel"},
	})
	group("Foreground ladder", [][3]string{
		{"--color-fg-strong", t.FgStrong.Hex(), "headings and figures"},
		{"--color-fg-default", t.FgDefault.Hex(), "primary text"},
		{"--color-fg-muted", t.FgMuted.Hex(), "secondary text — 4.5:1 on every surface"},
		{"--color-fg-disabled", t.FgDisabled.Hex(), "inactive controls — 3:1; SC 1.4.3 exempts them"},
	})
	group("Line ladder", [][3]string{
		{"--color-line-soft", t.LineSoft.Hex(), "hairline dividers — decorative, owes no ratio"},
		{"--color-line-strong", t.LineStrong.Hex(), "input edges — a component boundary, 3:1 (SC 1.4.11)"},
	})
	group("Accent", [][3]string{
		{"--color-accent", t.Accent.Hex(), "solid fill, and the focus ring"},
		{"--color-accent-hover", t.AccentHover.Hex(), "fill hover — moves away from its ink, never toward it"},
		{"--color-accent-text", t.AccentText.Hex(), "the accent as a link on a dark surface"},
		{"--color-accent-soft", t.AccentSoft.Hex(), "ghost-button wash: the accent composited over paper"},
		{"--color-on-accent", t.OnAccent.Hex(), "the ink an accent fill carries"},
	})
	group("Category accents — A/B/C and phase coding", [][3]string{
		{"--color-accent-2", t.Accent2.Hex(), "secondary"},
		{"--color-accent-3", t.Accent3.Hex(), "tertiary"},
	})
	group("Status semantics — the gamut's character, never its hue assignments", [][3]string{
		{"--color-good", t.Good.Hex(), ""},
		{"--color-warn", t.Warn.Hex(), ""},
		{"--color-bad", t.Bad.Hex(), ""},
		{"--color-info", t.Info.Hex(), ""},
	})

	b.WriteString("\n  /* Chart series a–o, sampled around the gamut's hue wheel. */\n")
	for i, c := range t.Series {
		fmt.Fprintf(&b, "  --color-series-%c: %s;\n", 'a'+i, c.Hex())
	}
	b.WriteString("}\n")

	b.WriteString("\n/* The category accents against a light surface. Only these two flip: the rest of\n")
	b.WriteString("   the theme is dark by design, but A/B and feed-side coding has to resolve on a\n")
	b.WriteString("   light surface too, so each carries a partner held to 4.5:1 on white. */\n")
	b.WriteString("@media (prefers-color-scheme: light) {\n  :root {\n")
	fmt.Fprintf(&b, "    --color-accent-2: %s;\n", t.Accent2Light.Hex())
	fmt.Fprintf(&b, "    --color-accent-3: %s;\n", t.Accent3Light.Hex())
	b.WriteString("  }\n}\n")

	return b.String()
}
