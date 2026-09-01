// Package good contains the sanctioned spellings of everything bad.go violates,
// so the analyzers must report zero findings here.
package good

import (
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
)

// Datastar attributes go through the typed builders, never raw strings.
var attr = datastar.OnClick("@post('/x')")

// Colors come from design tokens; a var(--token, #fallback) shape is allowed.
const swatch = `<span style="color:var(--accent)">hi</span>`
const swatchFallback = `<rect fill="var(--chart-grid, #000000)"/>`

// The only sanctioned script is the framework's datastar bundle.
const bundle = `<script type="module" src="/static/js/datastar.js" defer></script>`

// Routes register through the typed shapes / the Raw escape hatch, not HandleFunc.
func wire(app *beach.App) {
	app.Raw("POST", "/webhooks", func(w http.ResponseWriter, r *http.Request) {})
}

// A comment mentioning data-on-load must not trip the raw-datastar rule, since
// rules only scan string literals, not comments.
var _ = attr
