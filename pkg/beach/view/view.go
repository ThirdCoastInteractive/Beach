// Package view owns the framework's own front-end source: the Tailwind input
// stylesheet under css/ and the compiled, served tree under static/. It exists
// as a package for one reason — so the design tokens declared in the stylesheet
// can be read from Go.
//
// That matters because contrast is a property of pairs, not of colors, and every
// pair the kit paints owes a WCAG ratio (see
// docs/architecture/06-ui.md#accessible-by-construction). Checking those pairs
// means resolving var(--color-…) somewhere other than the browser, and the only
// honest source is the sheet itself: a second copy of the values in Go would be
// a copy that drifts, and it would report ratios for colors the page is no
// longer painting.
package view

import (
	"embed"
	"regexp"
	"strings"
	"sync"
)

//go:embed css/input.css
var css embed.FS

// InputCSS returns the stylesheet source: the Tailwind input the served
// app.css is built from. It is the same text Tokens() parses, exposed whole for
// checks a token lookup cannot express — chiefly that every animation the kit
// defines is also switched off under prefers-reduced-motion, which no test can
// see from the compiled output alone.
func InputCSS() string {
	b, err := css.ReadFile("css/input.css")
	if err != nil {
		// Embedded at build time; a failure here is a build bug.
		panic("view: reading embedded input.css: " + err.Error())
	}
	return string(b)
}

// tokenRe matches a `--color-foo: #rrggbb;` declaration. Tokens defined through
// color-mix or var() indirection are deliberately not matched: their painted
// value is not knowable from the source, so a caller measuring contrast has to
// composite them itself.
var tokenRe = regexp.MustCompile(`(--color-[a-z0-9-]+):\s*(#[0-9a-fA-F]{3,8})\s*;`)

var (
	tokensOnce sync.Once
	tokens     map[string]string
)

// Tokens returns the literal hex color tokens declared in the stylesheet, keyed
// by custom-property name ("--color-accent"). The first declaration of a name
// wins, so a theme override further down the sheet does not displace the base
// value. The returned map is shared and must not be mutated.
func Tokens() map[string]string {
	tokensOnce.Do(func() {
		b, err := css.ReadFile("css/input.css")
		if err != nil {
			// The file is embedded at build time; a failure here is a build bug.
			panic("view: reading embedded input.css: " + err.Error())
		}
		tokens = map[string]string{}
		for _, m := range tokenRe.FindAllStringSubmatch(string(b), -1) {
			if _, seen := tokens[m[1]]; !seen {
				tokens[m[1]] = strings.ToLower(m[2])
			}
		}
	})
	return tokens
}
