package chart

import (
	"embed"
	"io/fs"
)

// The chart stylesheet and interaction scripts live beside the fragments that
// emit their class names, so every consumer serves the same version of both.
//
//go:embed static
var staticFS embed.FS

// Static is the chart asset tree: css/ holds the stylesheet partials and
// js/ the interaction modules, with js/chart.js as the entry point. pkg/beach
// serves it under /static/ with its own assets and imports the partials into
// its stylesheet. An app without pkg/beach serves Static itself, links
// css/standalone.css, and loads js/chart.js as a module; standalone.css lists
// the theme tokens the host page must define.
func Static() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// The embed directive guarantees the path exists at build time.
		panic("chart: embedded static tree missing: " + err.Error())
	}
	return sub
}
