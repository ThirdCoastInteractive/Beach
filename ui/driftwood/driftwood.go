// Package driftwood is the-beach's reference ui.Kit — a real, themed
// implementation of the toolkit (not the bundled NoopKit). It renders clean
// semantic HTML whose every color, space, radius, and motion value comes from a
// CSS custom property (var(--neutral-2), var(--primary-6), …); no literal
// hex/OKLCH ever appears in the markup. The kit's styles compile into the single
// generated css/app.css (the Tailwind build: reset, tokens, and the dw-* classes).
//
// A kit travels in the request context; driftwood registers itself in init()
// and an app pulls it in with a side-effect import:
//
//	import _ "github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
//
// Page renders the <html> shell (stylesheet link to /static/css/app.css,
// theme-correct surfaces) so an app can serve a complete document with the kit.
package driftwood

import (
	"context"
	"html"
	"io"

	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// Kit is the driftwood kit value. It is a single struct that satisfies every
// ui sub-interface (the sub-interface getters return the same value), matching
// the pattern the toolkit expects: a kit is a value, not a singleton.
type Kit struct{}

// New returns the driftwood kit.
func New() ui.Kit { return Kit{} }

func init() { ui.Register(New()) }

// Name keys the kit in the registry and shows up in diagnostics.
func (Kit) Name() string { return "driftwood" }

func (k Kit) Layout() ui.Layout       { return k }
func (k Kit) Nav() ui.Nav             { return k }
func (k Kit) Buttons() ui.Buttons     { return k }
func (k Kit) Forms() ui.Forms         { return k }
func (k Kit) Data() ui.Data           { return k }
func (k Kit) Feedback() ui.Feedback   { return k }
func (k Kit) Overlays() ui.Overlays   { return k }
func (k Kit) Media() ui.Media         { return k }
func (k Kit) Messaging() ui.Messaging { return k }

// --- tiny render helpers -----------------------------------------------------

// esc is the HTML-escape used for all interpolated instance data.
func esc(s string) string { return html.EscapeString(s) }

// attrs renders a props' datastar attributes as a space-leading string (empty
// when there are none), so callers can splice it verbatim into a start tag.
func attrs(a datastar.Attrs) string {
	if len(a) == 0 {
		return ""
	}
	return " " + a.String()
}

// el renders an element with a class, an optional pre-rendered attribute string
// (already space-leading), and inner components.
func el(tag, class, attr string, inner ...templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		open := "<" + tag
		if class != "" {
			open += ` class="` + class + `"`
		}
		open += attr + ">"
		if _, err := io.WriteString(w, open); err != nil {
			return err
		}
		for _, c := range inner {
			if c == nil {
				continue
			}
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "</"+tag+">")
		return err
	})
}

// raw writes a literal HTML string (used for void elements like <input>/<img>).
func raw(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}

// txt renders escaped text content.
func txt(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, esc(s))
		return err
	})
}
