package driftwood

// What the stylesheet owes, as tests.
//
// The palette itself is no longer checked here. It is derived by pkg/theme from
// a preset, and pkg/theme's own tests hold every pair of every preset to WCAG AA
// in both schemes — which is a stronger check than this file could make, because
// it tests the thing that produces the colours rather than a parse of the
// colours it produced.
//
// What is left is the part that is genuinely about the *sheet*: that the sheet
// carries exactly what the generator would write, and that every animation it
// starts can be stopped.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/Beach/pkg/beach/view"
	"github.com/ThirdCoastInteractive/Beach/pkg/theme"
)

// TestGeneratedPaletteMatchesInputCSS re-derives the shipped preset and asserts
// the stylesheet contains that exact block.
//
// It is what makes "do not edit the generated values" enforceable rather than
// advisory. Hand-tuning one stop in the sheet is the single most tempting thing
// to do to a generated palette and the single most destructive: the value stops
// being the solution to its contrast obligations, nothing recomputes it, and the
// next regeneration silently reverts it.
func TestGeneratedPaletteMatchesInputCSS(t *testing.T) {
	want, err := theme.BuildPreset(view.ThemePreset)
	if err != nil {
		t.Fatalf("deriving %s: %v", view.ThemePreset, err)
	}
	if !strings.Contains(view.InputCSS(), want.CSS()) {
		t.Errorf("input.css does not carry the palette that preset %q derives to. "+
			"Either a generated value was hand-edited, or the preset changed without "+
			"`make palette` being run.", view.ThemePreset)
	}
}

// TestSheetDeclaresEveryTokenTheKitPaints guards the other direction: a token the
// components reference but the palette never defines resolves to nothing, and a
// CSS custom property that resolves to nothing takes its whole declaration with
// it — so the element paints as though the rule had never been written.
func TestSheetDeclaresEveryTokenTheKitPaints(t *testing.T) {
	declared := view.Tokens()
	for _, name := range theme.TokenNames() {
		if _, ok := declared[name]; !ok {
			t.Errorf("%s is derived but never lands in the stylesheet", name)
		}
	}
}

// TestBothSchemesReachTheSheet checks the three-state block survived into the
// sheet. Losing the :not() guard is the classic failure: the toggle then works
// in one direction and silently does nothing in the other.
func TestBothSchemesReachTheSheet(t *testing.T) {
	css := view.InputCSS()
	for _, want := range []string{
		"@media (prefers-color-scheme: light)",
		`:root:not([data-theme="dark"])`,
		`:root[data-theme="light"]`,
		`:root[data-theme="dark"]`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("input.css is missing %q", want)
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
