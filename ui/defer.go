package ui

import (
	"context"
	"html"
	"io"

	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/a-h/templ"
)

// Defer renders a server-side placeholder for a deferred section with exact
// reserved dimensions (a kit-styled skeleton, zero layout shift) and a Datastar
// @get that fires on viewport intersection — or on load for behind-a-tab
// content (DeferProps.OnLoad). The @get lands on a normal beach.PageFunc; the
// framework's dual-purpose branch returns the fragment and the patch morphs into
// this placeholder.
//
// This is the deferred-sections law made concrete: the placeholder reserves its
// height up front so late content fills reserved space and never moves rendered
// pixels. Anything bigger than the 14KB budget, or an expensive query that
// shouldn't block first paint, becomes a Defer.
//
// The skeleton background comes from the kit's `.ui-defer` class (a surface
// color in the kit stylesheet), never a hardcoded color in the markup.
func Defer(p DeferProps) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		width := p.Width
		if width == "" {
			width = "100%"
		}
		// The reserved box: inline style carries only dimensions (instance data),
		// while color/radius/animation come from the .ui-defer class the kit's
		// CSS styles against design tokens.
		style := "height:" + html.EscapeString(p.Height) +
			";width:" + html.EscapeString(width) + ";"

		// @get fires on intersection by default, or on load for behind-a-tab
		// content that has no viewport trigger.
		event := "intersect"
		if p.OnLoad {
			event = "load"
		}
		attrs := datastar.Attrs{datastar.On(event, "@get('"+p.Get+"')")}

		s := `<div id="` + html.EscapeString(p.ID) + `"` +
			` class="ui-defer ui-skeleton"` +
			` style="` + style + `"` +
			` role="status" aria-busy="true"` +
			withAttrs(attrs) +
			`></div>`
		_, err := io.WriteString(w, s)
		return err
	})
}
