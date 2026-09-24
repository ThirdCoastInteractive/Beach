package lint

import (
	"regexp"
	"strings"
)

// --- Rule 10: hand-written spacing in markup ---------------------------------
//
// Spacing is the thing that goes wrong most often in generated markup — too
// much, too little, or none at all — and every one of those failures starts the
// same way: someone writes a number. A padding invented at the call site is
// invisible to the design system, drifts from every other value on the page, and
// nothing can check it, because there is nothing to check it against.
//
// The kit's answer is that spacing is a closed set: driftwood.Space has nine
// rungs and no escape hatch, and the layout primitives take one. A caller
// literally cannot write `p-7` or `2.3rem` through the props. This rule closes
// the other door — writing it into the markup directly.
//
// Like the colour rule, it reads the *generated* *_templ.go, where templ has
// already flattened the markup into string literals. That means it sees the
// same attribute text the browser will.

// classAttrRe finds a class attribute's value in markup. templ splits an
// attribute around an interpolated value, so a literal may hold only a fragment;
// matching the opening quote and reading to the next one keeps the scan honest
// on both shapes.
var classAttrRe = regexp.MustCompile(`class="([^"]*)"`)

// styleAttrRe is the same for inline styles.
var styleAttrRe = regexp.MustCompile(`style="([^"]*)"`)

// spacingUtilRe matches a Tailwind spacing utility: an optional variant prefix
// (sm:, hover:, -), then one of the spacing properties, then a numeric or
// bracketed value. The value is required, so semantic class names of the kit's
// own shape — `dw-pad-lg`, `dw-gap-md` — do not match.
var spacingUtilRe = regexp.MustCompile(`(?:^|\s|:)-?(p|px|py|pt|pr|pb|pl|ps|pe|m|mx|my|mt|mr|mb|ml|ms|me|gap|gap-x|gap-y|space-x|space-y)-(\d|\[)`)

// inlineSpacingRe matches a padding or margin declaration in a style attribute.
// It requires the colon, so a CSS custom property whose *name* contains the word
// is not mistaken for a declaration.
var inlineSpacingRe = regexp.MustCompile(`(?:^|[;\s])(padding|margin)(-(top|right|bottom|left|inline|block|inline-start|inline-end|block-start|block-end))?\s*:`)

// spacingSanctioned is where hand-written spacing is a requirement rather than a
// smell.
//
// Mail is the real exemption: HTML email clients strip stylesheets and ignore
// most of CSS, so an email's padding genuinely has to live on the element, and
// an email is not a page the design system reaches anyway.
//
// beach-palette is a development tool that serves its own diagnostic page from
// its own stylesheet — it is not an application built on the kit, so the kit's
// ladder is not available to it.
var spacingSanctioned = []string{"mailer", "cmd/beach-palette"}

// ruleRawSpacing flags spacing written into markup rather than taken from the
// kit's ladder.
func ruleRawSpacing(fc *fileCtx, out *[]Finding) {
	if fc.onPath(lintSelf) {
		return // the analyzer's own patterns are not markup
	}
	if strings.Contains(fc.pkg, "internal/mail") || fc.onPath(spacingSanctioned...) {
		return
	}
	fc.stringLits2(func(val string, line int) {
		for _, m := range classAttrRe.FindAllStringSubmatch(val, -1) {
			if loc := spacingUtilRe.FindString(m[1]); loc != "" {
				*out = append(*out, Finding{
					File: fc.path,
					Line: line,
					Rule: "raw-spacing",
					Message: "hand-written spacing utility " + strings.TrimSpace(loc) +
						" in markup; spacing comes from the kit's ladder — a Gap or Pad prop " +
						"taking a driftwood.Space, or a layout primitive (Stack, Inline, Box, Section)",
				})
				return
			}
		}
		for _, m := range styleAttrRe.FindAllStringSubmatch(val, -1) {
			if inlineSpacingRe.MatchString(m[1]) {
				*out = append(*out, Finding{
					File: fc.path,
					Line: line,
					Rule: "raw-spacing",
					Message: "inline padding/margin in a style attribute; spacing comes from the " +
						"kit's ladder — a Gap or Pad prop taking a driftwood.Space, or a layout " +
						"primitive. Inline style is for per-instance dimensions the kit cannot know " +
						"(a reserved height), not for spacing",
				})
				return
			}
		}
	})
}
