package driftwood

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// PageProps configures the document shell Page renders.
type PageProps struct {
	Title       string // <title> text
	Lang        string // <html lang>, defaults to "en"
	Description string // optional meta description
	BodyClass   string // optional extra class on <body>
	Stylesheet  string // override the kit stylesheet href (default below)
}

// appHref is the kit's single render-blocking stylesheet: the Tailwind build
// (reset + the dw-* component classes), served from the framework static tree.
const appHref = "/static/css/app.css"

// Page renders a complete, server-rendered HTML document around body: the
// <html> shell with theme-correct surfaces, the Datastar module (deferred), and
// the render-blocking stylesheet link. Theme classes sit on <html> before paint
// (no pop-in); the page targets the 14KB budget — heavier content becomes a
// ui.Defer section.
func Page(p PageProps, body ...templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		lang := p.Lang
		if lang == "" {
			lang = "en"
		}
		sheet := p.Stylesheet
		if sheet == "" {
			sheet = appHref
		}

		head := `<!DOCTYPE html><html lang="` + esc(lang) + `" class="dw-root">` +
			`<head>` +
			`<meta charset="utf-8">` +
			`<meta name="viewport" content="width=device-width, initial-scale=1">` +
			`<title>` + esc(p.Title) + `</title>`
		if p.Description != "" {
			head += `<meta name="description" content="` + esc(p.Description) + `">`
		}
		// The stylesheet is the single render-blocking resource. Datastar ships
		// deferred (no-blocking law).
		head += `<link rel="stylesheet" href="` + esc(sheet) + `">` +
			`<script type="module" src="/static/js/datastar.js" defer></script>` +
			`</head>`

		bodyClass := "dw-body"
		if p.BodyClass != "" {
			bodyClass += " " + esc(p.BodyClass)
		}
		if _, err := io.WriteString(w, head+`<body class="`+bodyClass+`">`); err != nil {
			return err
		}
		for _, c := range body {
			if c == nil {
				continue
			}
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</body></html>`)
		return err
	})
}
