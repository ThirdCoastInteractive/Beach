// Command beach-demos renders a small static demo site for the-beach geo
// chart suite (geo map, globe, sun clock) to a directory of self-contained
// HTML files. Every page works opened directly from disk via file:// with no
// server: the compiled stylesheet, the needed browser JS modules, and (on
// animated-globe pages) the world geometry are all inlined at generate time.
//
//	go run ./cmd/beach-demos            # writes ./demos/
//	go run ./cmd/beach-demos -out site  # writes ./site/
//
// The charts are server-rendered SVG (pkg/chart), so each page is produced by
// calling the chart templ components' Render straight into the page HTML. The
// only runtime inputs are files already in the tree (the built app.css, the
// static JS modules, and the world-geo.json globe payload); nothing here is
// generated on the fly, so the same tree always yields the same site.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/a-h/templ"
)

func main() {
	out := flag.String("out", "demos", "output directory for the generated site")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		log.Fatalf("locate repo root: %v", err)
	}
	a, err := loadAssets(root)
	if err != nil {
		log.Fatalf("load assets: %v", err)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *out, err)
	}

	for _, p := range pages {
		html := renderShell(p, a)
		dst := filepath.Join(*out, p.Slug)
		if err := os.WriteFile(dst, []byte(html), 0o644); err != nil {
			log.Fatalf("write %s: %v", dst, err)
		}
		fmt.Printf("wrote %s (%d KB)\n", dst, (len(html)+512)/1024)
	}
	fmt.Printf("done — %d pages in %s\n", len(pages), *out)
}

// --- assets ------------------------------------------------------------------

// assets holds everything inlined into every page: the stylesheet, the
// concatenated JS module block, and the raw world-geo payload for animated
// globes. Loaded once, shared across all pages.
type assets struct {
	css      string // app.css + a forced-dark :root re-assertion + the shell chrome
	js       string // chart-tip + geomap-interact + globe, import lines stripped
	worldGeo string // pkg/beach/view/static/geo/world-geo.json, verbatim
}

func loadAssets(root string) (*assets, error) {
	static := filepath.Join(root, "pkg", "beach", "view", "static")

	css, err := os.ReadFile(filepath.Join(static, "css", "app.css"))
	if err != nil {
		return nil, fmt.Errorf("read app.css: %w", err)
	}
	worldGeo, err := os.ReadFile(filepath.Join(static, "geo", "world-geo.json"))
	if err != nil {
		return nil, fmt.Errorf("read world-geo.json: %w", err)
	}

	// Concatenate the side-effect JS modules into one module block, in the
	// order tip → interact → globe (tooltips first, hydration last). They are
	// side-effect modules with no exports; any `import './x.js';` line is a
	// sibling reference that a single concatenated block does not need, so it
	// is stripped here rather than hand-copied out.
	var jsb strings.Builder
	for _, name := range []string{"chart-tip.js", "geomap-interact.js", "globe.js"} {
		src, err := os.ReadFile(filepath.Join(static, "js", name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		// IIFE per module: as separate files they each own a module scope,
		// and their top-level declarations collide when merged bare (both
		// geomap-interact and globe declare `reducedMotion`).
		jsb.WriteString("// ---- ")
		jsb.WriteString(name)
		jsb.WriteString(" ----\n(() => {\n")
		jsb.WriteString(stripImports(string(src)))
		jsb.WriteString("\n})();\n")
	}

	return &assets{
		css:      string(css) + "\n" + forceDarkRoot(string(css)) + "\n" + shellCSS,
		js:       jsb.String(),
		worldGeo: strings.TrimSpace(string(worldGeo)),
	}, nil
}

// importLine matches a bare side-effect module import ("import './x.js';").
var importLine = regexp.MustCompile(`(?m)^\s*import\s+['"][^'"]+['"];?\s*$`)

// stripImports removes side-effect import statements so several modules can be
// concatenated into one inline block. It is deliberately conservative: only
// bare `import '…';` lines go, never `import { x } from …` bindings (the geo
// modules have none today, but the filter must not silently drop real code if
// one is added later).
func stripImports(js string) string {
	return importLine.ReplaceAllString(js, "")
}

// rootBlock matches the first top-level `:root { … }` declaration.
var rootBlock = regexp.MustCompile(`(?s):root\s*\{.*?\}`)

// forceDarkRoot returns a copy of the sheet's first (dark) :root block so it
// can be re-appended after the whole stylesheet. app.css is dark-first with a
// `@media (prefers-color-scheme: light)` override; re-asserting the dark
// tokens last (same specificity, later in source, unlayered) pins the demo
// site to the dark theme regardless of the viewer's OS preference, which is
// what the geo charts are designed against.
func forceDarkRoot(css string) string {
	m := rootBlock.FindString(css)
	if m == "" {
		return ""
	}
	return "/* demo site: pin the dark theme */\n" + m
}

// --- page shell --------------------------------------------------------------

// page is one generated HTML file: its filename, nav label, heading, a builder
// for the main content, and whether it embeds an animated globe (which needs
// the world geometry inlined for a file:// export).
type page struct {
	Slug     string
	Nav      string
	Title    string
	Subtitle string
	Build    func() string
	Globe    bool // inline world-geo.json for animated globes
}

// renderShell wraps a page's content in the full standalone document: inlined
// stylesheet, top nav, heading, the content, the optional inline globe
// geometry, and the inlined JS module block.
func renderShell(p page, a *assets) string {
	var nav strings.Builder
	for _, o := range pages {
		cls := "demo-nav-link"
		if o.Slug == p.Slug {
			cls += " is-active"
		}
		fmt.Fprintf(&nav, `<a class="%s" href="%s">%s</a>`, cls, o.Slug, htmlesc(o.Nav))
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString(`<meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	fmt.Fprintf(&b, "<title>%s — the-beach geo charts</title>\n", htmlesc(p.Title))
	b.WriteString("<style>\n")
	b.WriteString(a.css)
	b.WriteString("\n</style>\n</head>\n<body class=\"demo-body\">\n")

	// Top chrome: brand + nav.
	b.WriteString(`<header class="demo-topbar">`)
	b.WriteString(`<div class="demo-brand"><span class="demo-brand-mark">Third Coast Interactive</span><span class="demo-brand-sub">the-beach geo charts</span></div>`)
	fmt.Fprintf(&b, `<nav class="demo-nav">%s</nav>`, nav.String())
	b.WriteString(`</header>`)

	// Page heading + content.
	b.WriteString(`<main class="demo-main">`)
	b.WriteString(`<div class="demo-head">`)
	fmt.Fprintf(&b, `<h1 class="demo-h1">%s</h1>`, htmlesc(p.Title))
	if p.Subtitle != "" {
		fmt.Fprintf(&b, `<p class="demo-sub">%s</p>`, htmlesc(p.Subtitle))
	}
	b.WriteString(`</div>`)
	b.WriteString(p.Build())
	b.WriteString(`</main>`)

	b.WriteString(`<footer class="demo-foot">Server-rendered SVG · self-contained static export · Third Coast Interactive</footer>`)

	// Inline the world geometry before the module block so globe.js finds it.
	if p.Globe {
		b.WriteString(`<script type="application/json" id="world-geo-inline">`)
		b.WriteString(a.worldGeo)
		b.WriteString("</script>\n")
	}
	b.WriteString("<script type=\"module\">\n")
	b.WriteString(a.js)
	b.WriteString("\n</script>\n</body>\n</html>\n")

	html := b.String()
	if p.Globe {
		// Animated-globe payloads carry a fallback src of
		// /static/geo/world-geo.json for served deployments. This export inlines
		// the geometry (globe.js reads #world-geo-inline before any fetch), so
		// the src is never requested; rewrite it to the inline marker to leave
		// the file with zero /static/ URLs — genuinely self-contained on file://.
		html = strings.ReplaceAll(html, "/static/geo/world-geo.json", "world-geo-inline")
	}
	return html
}

// --- small helpers -----------------------------------------------------------

// render runs a chart templ component into an HTML string. The chart geometry
// is trusted, already-escaped markup, so it is spliced into the page verbatim.
func render(c templ.Component) string {
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		log.Fatalf("render component: %v", err)
	}
	return b.String()
}

// panel wraps content in a titled card matching the house .panel look.
func panel(title, note, span, inner string) string {
	var b strings.Builder
	cls := "demo-panel"
	if span != "" {
		cls += " " + span
	}
	fmt.Fprintf(&b, `<section class="%s">`, cls)
	if title != "" {
		fmt.Fprintf(&b, `<header class="demo-panel-head"><span class="demo-eyebrow">%s</span>`, htmlesc(title))
		if note != "" {
			fmt.Fprintf(&b, `<span class="demo-panel-note">%s</span>`, htmlesc(note))
		}
		b.WriteString(`</header>`)
	}
	b.WriteString(inner)
	b.WriteString(`</section>`)
	return b.String()
}

// chartBox gives a chart fragment an explicit height (the fragment fills its
// box via width/height 100%). heightCSS is any CSS length ("420px", "min(70vw,560px)").
func chartBox(heightCSS, inner string) string {
	return fmt.Sprintf(`<div class="demo-chart" style="height:%s">%s</div>`, heightCSS, inner)
}

// grid wraps cards in an auto-flow grid.
func grid(inner string) string { return `<div class="demo-grid">` + inner + `</div>` }

func htmlesc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// repoRoot walks up from this source file to the directory holding go.mod, so
// the generator finds the static assets no matter the working directory.
func repoRoot() (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	dir := filepath.Dir(self)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", filepath.Dir(self))
		}
		dir = parent
	}
}
