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
	"io/fs"
	"regexp"
	"strings"
	"sync"

	"github.com/ThirdCoastInteractive/Beach/pkg/chart"
)

//go:embed css/input.css
var css embed.FS

// ThemePreset is the pkg/theme preset key the served palette is derived from.
// It lives here, beside the sheet it describes, so three things cannot drift
// apart: the generator that writes the tokens (cmd/beach-palette), the
// stylesheet that carries them, and the test that re-derives the palette and
// diffs it against what the sheet actually says.
//
// Re-theming the whole framework — both schemes, every token — is changing this
// string and running `make palette`. `beach-palette -serve` previews the
// alternatives.
const ThemePreset = "driftwood"

// chartImport matches an input.css line that pulls in one of pkg/chart's
// stylesheet partials. Tailwind inlines those at build time.
var chartImport = regexp.MustCompile(`(?m)^@import "\.\./\.\./\.\./chart/static/(css/[a-z-]+\.css)";\r?$`)

// InputCSS returns the stylesheet source: the Tailwind input the served
// app.css is built from, with pkg/chart's partials inlined where input.css
// imports them, as Tailwind does. It is the same text Tokens() parses, exposed
// whole for checks a token lookup cannot express — chiefly that every animation
// the kit defines is also switched off under prefers-reduced-motion, which no
// test can see from the compiled output alone.
func InputCSS() string {
	b, err := css.ReadFile("css/input.css")
	if err != nil {
		// Embedded at build time; a failure here is a build bug.
		panic("view: reading embedded input.css: " + err.Error())
	}
	return chartImport.ReplaceAllStringFunc(string(b), func(line string) string {
		name := chartImport.FindStringSubmatch(line)[1]
		part, err := fs.ReadFile(chart.Static(), name)
		if err != nil {
			// input.css names a partial pkg/chart does not embed.
			panic("view: reading chart partial " + name + ": " + err.Error())
		}
		return string(part)
	})
}

// tokenRe matches a `--color-foo: <value>;` declaration. The value is captured
// whole rather than as a hex literal: since the palette became generated it is
// written as oklch(), which is the form the theme was specified in and the form
// a reader can reason about.
var tokenRe = regexp.MustCompile(`(--color-[a-z0-9-]+):\s*([^;]+);`)

var (
	tokensOnce sync.Once
	tokens     map[string]string
)

// Tokens returns the color tokens declared in the stylesheet, keyed by
// custom-property name ("--color-accent") with the raw CSS value.
//
// The sheet declares every token four times — once on bare :root, once under the
// prefers-color-scheme query, and once under each explicit [data-theme] — so the
// first declaration wins and what comes back is the dark scheme, the one a
// visitor with no stated preference sees. A caller wanting both schemes should
// derive them from pkg/theme rather than parse them back out of here: the
// stylesheet is an output of that package, and re-reading an output to recover
// its input is how a second source of truth gets born.
//
// The returned map is shared and must not be mutated.
func Tokens() map[string]string {
	tokensOnce.Do(func() {
		tokens = map[string]string{}
		for _, m := range tokenRe.FindAllStringSubmatch(InputCSS(), -1) {
			if _, seen := tokens[m[1]]; !seen {
				tokens[m[1]] = strings.TrimSpace(m[2])
			}
		}
	})
	return tokens
}
