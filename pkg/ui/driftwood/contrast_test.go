package driftwood

// The palette contract, as a test.
//
// Contrast is a property of pairs, not of colors, so a token sheet cannot be
// checked by looking at it: --color-accent is fine as a focus ring and was, for
// a long time, not fine behind white button text. This reads the real tokens out
// of input.css — the source the stylesheet is built from, so a token edit is
// caught before `make css` ever runs — and asserts every pair the kit actually
// paints, at the WCAG 2.1 AA thresholds.
//
// Adding a component that puts one token on another means adding its pair here.
// That is the point: the sheet is where the pairing decision is made, and this
// is where it is held to.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/the-beach/pkg/beach/view"
	"github.com/ThirdCoastInteractive/the-beach/pkg/rybitten"
)

// tokens returns the painted color tokens, read from the stylesheet itself via
// view.Tokens rather than from a copy — a copy is a copy that drifts, and this
// test exists precisely to catch a token that moved.
func tokens(t *testing.T) map[string]rybitten.RGB {
	t.Helper()
	out := map[string]rybitten.RGB{}
	for name, hex := range view.Tokens() {
		c, err := rybitten.ParseHex(hex)
		if err != nil {
			continue // an 8-digit alpha value is a wash, not a painted stop
		}
		out[name] = c
	}
	if len(out) < 10 {
		t.Fatalf("parsed only %d color tokens from input.css — did the token block move?", len(out))
	}
	return out
}

// pair is one contrast obligation: a foreground painted on a background, and
// the ratio WCAG requires for that kind of pairing.
type pair struct {
	what      string
	fg, bg    string // token names, or a literal hex for white/black
	min       float64
	criterion string
}

const (
	// aaText is SC 1.4.3 for normal-size text.
	aaText = 4.5
	// aaNonText is SC 1.4.11 for UI component boundaries and focus indicators,
	// and SC 1.4.3 for large text.
	aaNonText = 3.0
)

func TestPaletteMeetsAA(t *testing.T) {
	tk := tokens(t)
	resolve := func(name string) (rybitten.RGB, bool) {
		if len(name) > 0 && name[0] == '#' {
			c, err := rybitten.ParseHex(name)
			return c, err == nil
		}
		c, ok := tk[name]
		return c, ok
	}

	pairs := []pair{
		// Body text on every surface it lands on.
		{"body text on the page", "--color-fg-default", "--color-paper", aaText, "1.4.3"},
		{"body text on a card", "--color-fg-default", "--color-panel", aaText, "1.4.3"},
		{"muted text on the page", "--color-fg-muted", "--color-paper", aaText, "1.4.3"},
		{"muted text on a card", "--color-fg-muted", "--color-panel", aaText, "1.4.3"},
		{"muted text on a raised panel", "--color-fg-muted", "--color-panel-hover", aaText, "1.4.3"},

		// The primary button, in both of its states. This is the pair that was
		// failing at 3.74:1 and drove the accent down the munsell axis.
		{"accent button label", "--color-on-accent", "--color-accent", aaText, "1.4.3"},
		{"accent button label, hovered", "--color-on-accent", "--color-accent-hover", aaText, "1.4.3"},

		// Links and the ghost button, including its hover wash.
		{"link text on the page", "--color-accent-text", "--color-paper", aaText, "1.4.3"},
		{"link text on a card", "--color-accent-text", "--color-panel", aaText, "1.4.3"},
		{"ghost button label, hovered", "--color-accent-text", "--color-accent-soft", aaText, "1.4.3"},

		// Role colors as text — badges and alerts tint from these.
		{"success text", "--color-good", "--color-panel", aaText, "1.4.3"},
		{"warning text", "--color-warn", "--color-panel", aaText, "1.4.3"},
		{"error text", "--color-bad", "--color-panel", aaText, "1.4.3"},
		{"info text", "--color-info", "--color-panel", aaText, "1.4.3"},
		{"field error text", "--color-bad", "--color-paper", aaText, "1.4.3"},

		// Chat bubbles: an own message is an accent fill, and its metadata lines
		// have to clear the same bar as its body text.
		{"message text on a received bubble", "--color-fg-default", "--color-panel-hover", aaText, "1.4.3"},
		{"message metadata on a received bubble", "--color-fg-muted", "--color-panel-hover", aaText, "1.4.3"},
		{"message metadata on an own bubble", "--color-on-accent", "--color-accent", aaText, "1.4.3"},

		// Non-text: the focus ring has to be visible on every surface it can be
		// drawn against, and an input's edge is what identifies the control.
		{"focus ring on the page", "--color-accent", "--color-paper", aaNonText, "1.4.11"},
		{"focus ring on a card", "--color-accent", "--color-panel", aaNonText, "1.4.11"},
		{"focus ring on a raised panel", "--color-accent", "--color-panel-hover", aaNonText, "1.4.11"},
		{"input edge on the page", "--color-line-strong", "--color-paper", aaNonText, "1.4.11"},
		{"input edge on a card", "--color-line-strong", "--color-panel", aaNonText, "1.4.11"},
		{"input edge on a raised panel", "--color-line-strong", "--color-panel-hover", aaNonText, "1.4.11"},
	}

	for _, p := range pairs {
		fg, okf := resolve(p.fg)
		bg, okb := resolve(p.bg)
		if !okf || !okb {
			t.Errorf("%s: token %s or %s is missing from input.css", p.what, p.fg, p.bg)
			continue
		}
		got := rybitten.Contrast(fg, bg)
		if got < p.min {
			t.Errorf("%s: %s on %s is %.2f:1, WCAG %s needs %.1f:1",
				p.what, p.fg, p.bg, got, p.criterion, p.min)
		}
	}
}

// TestChartSeriesAreDistinguishable holds the series palette to the non-text
// threshold against the page. A series color is the entire encoding of a line
// or a wedge, so one that fades into the background takes its data with it.
//
// Note what this does *not* claim: 3:1 against the paper makes each series
// visible, not distinguishable from its neighbours. Telling two series apart by
// color alone is beyond what a ratio can check, which is why the kit pairs color
// with a second encoding — labels, arrows, direct annotation — everywhere it can.
func TestChartSeriesAreDistinguishable(t *testing.T) {
	tk := tokens(t)
	paper, ok := tk["--color-paper"]
	if !ok {
		t.Fatal("--color-paper missing from input.css")
	}
	for c := 'a'; c <= 'o'; c++ {
		name := "--color-series-" + string(c)
		col, ok := tk[name]
		if !ok {
			t.Errorf("%s missing from input.css", name)
			continue
		}
		if got := rybitten.Contrast(col, paper); got < aaNonText {
			t.Errorf("%s on the page is %.2f:1, WCAG 1.4.11 needs %.1f:1", name, got, aaNonText)
		}
	}
}

// --- motion ---------------------------------------------------------------------

// TestEveryAnimationRespectsReducedMotion is the law that an animation the kit
// starts is an animation the kit can stop.
//
// WCAG 2.3.3 asks that motion from interactions be disableable, and the browser
// already tells us who wants that — prefers-reduced-motion. The failure mode is
// not a wrong rule but a forgotten one: someone adds a pulse, it looks right, and
// the person it makes ill never files a bug. So rather than checking that the
// media query exists, this checks that *every* animating selector in the sheet is
// named inside one, and fails on the one that was left out.
func TestEveryAnimationRespectsReducedMotion(t *testing.T) {
	css := view.InputCSS()

	// The reduced-motion blocks, and everything they name.
	var covered strings.Builder
	for _, block := range reducedMotionBlocks(css) {
		covered.WriteString(block)
	}
	stops := covered.String()
	if stops == "" {
		t.Fatal("no @media (prefers-reduced-motion: reduce) block in input.css")
	}

	// Every selector that starts an animation, from either spelling: a raw
	// `animation:` declaration, or Tailwind's animate-* utility.
	for _, sel := range animatingSelectors(css) {
		if !strings.Contains(stops, sel) {
			t.Errorf("%s animates but is not named in any prefers-reduced-motion block — "+
				"an animation the kit starts is one it has to be able to stop (WCAG 2.3.3)", sel)
		}
	}
}

// reducedMotionBlocks returns the body of each @media (prefers-reduced-motion)
// block, found by brace matching so a nested rule cannot end the block early.
func reducedMotionBlocks(css string) []string {
	var out []string
	const marker = "prefers-reduced-motion"
	for i := 0; ; {
		j := strings.Index(css[i:], marker)
		if j < 0 {
			return out
		}
		start := strings.Index(css[i+j:], "{")
		if start < 0 {
			return out
		}
		start += i + j
		depth, end := 0, -1
		for k := start; k < len(css); k++ {
			switch css[k] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = k
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return out
		}
		out = append(out, css[start:end])
		i = end
	}
}

// animatingSelectors lists every class selector on a rule that starts an
// animation. Only class selectors are collected: the kit's animations all hang
// off one, and a bare element rule would be a different discussion.
func animatingSelectors(css string) []string {
	var out []string
	seen := map[string]bool{}
	lines := strings.Split(css, "\n")
	for i, line := range lines {
		// The declaration may sit on the selector's line (the @apply style) or
		// on the line after it (the plain-rule style).
		if !animates(line) {
			continue
		}
		sel := line
		if !strings.Contains(line, "{") && i > 0 {
			sel = lines[i-1]
		}
		for _, s := range classSelectors(sel) {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// animates reports whether a line starts an animation. `animation: none` is how
// a rule stops one, so it does not count.
func animates(line string) bool {
	if strings.Contains(line, "animation: none") || strings.Contains(line, "animation:none") {
		return false
	}
	return strings.Contains(line, "animation:") || strings.Contains(line, "animate-")
}

// classRe pulls class names out of a selector.
var classRe = regexp.MustCompile(`\.([a-zA-Z][\w-]*)`)

func classSelectors(sel string) []string {
	// Trim anything after the opening brace: the declarations, not the selector.
	if i := strings.Index(sel, "{"); i >= 0 {
		sel = sel[:i]
	}
	var out []string
	for _, m := range classRe.FindAllStringSubmatch(sel, -1) {
		out = append(out, "."+m[1])
	}
	return out
}
