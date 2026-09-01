package driftwood

import (
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/Beach/pkg/beach/view"
)

// The spacing ladder's contract.
//
// Every rung of Space and Measure is a class name assembled in Go by
// concatenation, and Tailwind emits a utility only when its source scanner has
// seen the literal name — which it cannot, for a name that does not exist until
// runtime. So the rules are written out by hand in input.css, and "written out by
// hand" is exactly the kind of list that goes one short.
//
// A rung with no rule behind it is the worst possible failure here, because it
// is completely silent: the class lands in the markup, matches nothing, and the
// element keeps whatever spacing it already had. Nothing errors, nothing looks
// obviously broken, and the prop simply does not work. This suite is what makes
// that loud. It is not hypothetical — the Go side of this ladder shipped before
// the CSS did, and every rung was a no-op until it caught up.

// ladders pairs each class prefix with the values that must exist under it.
func TestSpaceScaleIsClosed(t *testing.T) {
	css := view.InputCSS()

	for _, prefix := range []string{"dw-gap", "dw-pad", "dw-padx", "dw-pady"} {
		for _, s := range Spaces {
			want := "." + prefix + "-" + string(s)
			if !hasRule(css, want) {
				t.Errorf("%s is a Space rung with no rule in input.css — the class would "+
					"render and match nothing", want)
			}
		}
	}

	// SpaceAuto deliberately has no class: it means "the component's own
	// default", and emitting a rule for it would override the very thing it
	// exists to leave alone.
	for _, prefix := range []string{"dw-gap", "dw-pad", "dw-padx", "dw-pady"} {
		if hasRule(css, "."+prefix+"-") {
			t.Errorf(".%s- with an empty suffix has a rule; SpaceAuto must emit no class at all", prefix)
		}
	}
}

func TestMeasureScaleIsClosed(t *testing.T) {
	css := view.InputCSS()
	for _, m := range Measures {
		if m == MeasureDefault {
			continue // the zero value is the bare component, and emits no class
		}
		for _, prefix := range []string{"dw-w", "dw-cell", "dw-railw"} {
			want := "." + prefix + "-" + string(m)
			if !hasRule(css, want) {
				t.Errorf("%s is a Measure step with no rule in input.css", want)
			}
		}
	}
}

// TestAlignmentScalesAreClosed covers the two smaller ladders on the same terms.
func TestAlignmentScalesAreClosed(t *testing.T) {
	css := view.InputCSS()
	for _, a := range []Align{AlignStart, AlignCenter, AlignEnd, AlignBaseline} {
		if want := ".dw-align-" + string(a); !hasRule(css, want) {
			t.Errorf("%s has no rule in input.css", want)
		}
	}
	for _, j := range []Justify{JustifyCenter, JustifyEnd, JustifyBetween} {
		if want := ".dw-justify-" + string(j); !hasRule(css, want) {
			t.Errorf("%s has no rule in input.css", want)
		}
	}
}

// TestSpacingTokensExist checks the ladder the classes resolve *through*. A class
// that sets padding to an undefined custom property is invalid at computed-value
// time, which removes the declaration entirely — the same silent failure the
// container's max-width had before this change.
func TestSpacingTokensExist(t *testing.T) {
	css := view.InputCSS()
	for _, s := range Spaces {
		if want := "--space-" + string(s) + ":"; !strings.Contains(css, want) {
			t.Errorf("%s is referenced by the ladder but never defined", want)
		}
	}
	for _, m := range []Measure{MeasureText, MeasureNarrow, MeasureWide} {
		if want := "--measure-" + string(m) + ":"; !strings.Contains(css, want) {
			t.Errorf("%s is referenced by the ladder but never defined", want)
		}
	}
}

// TestPrimitivesHaveRules covers the seven layout components. Each emits one
// structural class that has to exist, or the component renders an unstyled div
// that looks almost right and stacks nothing.
func TestPrimitivesHaveRules(t *testing.T) {
	css := view.InputCSS()
	for _, class := range []string{
		".dw-stack", ".dw-inline", ".dw-box", ".dw-section", ".dw-center",
		".dw-prose", ".dw-rail", ".dw-rail-content", ".dw-rail-side",
		".dw-rail-start", ".dw-rail-end",
	} {
		if !hasRule(css, class) {
			t.Errorf("%s is emitted by a layout primitive but has no rule in input.css", class)
		}
	}
}

// hasRule reports whether the sheet carries a rule whose selector is exactly
// this class — not merely a string match, which would let ".dw-gap-2xs" satisfy
// a search for ".dw-gap-2x".
func hasRule(css, class string) bool {
	for i := 0; ; {
		j := strings.Index(css[i:], class)
		if j < 0 {
			return false
		}
		end := i + j + len(class)
		if end < len(css) {
			// A selector ends at whitespace, a comma, a brace, or a combinator.
			// Anything else means we matched a prefix of a longer class name.
			switch css[end] {
			case ' ', '\t', '\n', ',', '{', ':', '>', '+', '~':
				return true
			}
		}
		i = end
	}
}
