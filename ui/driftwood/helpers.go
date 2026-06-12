package driftwood

import (
	"strconv"

	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/a-h/templ"
)

// itoa/ftoa keep numeric attribute expressions terse in the .templ files.
func itoa(n int) string     { return strconv.Itoa(n) }
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// maybeClass wraps a possibly-empty class name so templ's class list drops it
// cleanly instead of rendering a stray space.
func maybeClass(cls string) templ.KeyValue[string, bool] {
	return templ.KV(cls, cls != "")
}

// roleClass maps a semantic Role to a driftwood modifier class. The class —
// not the Go code — binds to the design palette in app.css.
func roleClass(prefix string, r Role) templ.KeyValue[string, bool] {
	if r == RoleNeutral {
		return maybeClass("")
	}
	return maybeClass(prefix + "-" + string(r))
}

// sizeClass maps a Size step to a modifier class; the default step is the
// bare component class.
func sizeClass(prefix string, s Size) templ.KeyValue[string, bool] {
	if s == SizeMd {
		return maybeClass("")
	}
	return maybeClass(prefix + "-" + string(s))
}

// surfaceClass maps a Surface to its background-token class.
func surfaceClass(s Surface) templ.KeyValue[string, bool] {
	switch s {
	case SurfaceWell:
		return maybeClass("dw-surface-well")
	case SurfacePanel:
		return maybeClass("dw-surface-panel")
	}
	return maybeClass("")
}

// alertIcon picks a glyph that matches the alert role, so the meaning survives
// without color (color is never the only encoding).
func alertIcon(r Role) string {
	switch r {
	case RoleGood:
		return "check-circle"
	case RoleWarn:
		return "alert-triangle"
	case RoleDanger:
		return "alert-octagon"
	default:
		return "info-circle"
	}
}

// bind renders the data-bind:<signal> attribute for a named control so a form
// @post submits the value (Datastar posts signals, not raw form fields). The
// dynamic attribute key forces the spread form in templ.
func bind(name string) templ.Attributes {
	return datastar.Attrs{datastar.Bind(name)}.Templ()
}

// defaultID falls back to a stable id when the caller did not provide one, so
// popover/anchor wiring always has a target.
func defaultID(id, fallback string) string {
	if id == "" {
		return fallback
	}
	return id
}

// buttonType defaults a button's type to "button" so a bare kit button never
// accidentally submits an enclosing form.
func buttonType(t string) string {
	if t == "" {
		return "button"
	}
	return t
}

// inputType defaults a text input's type to "text".
func inputType(t string) string {
	if t == "" {
		return "text"
	}
	return t
}

// formSubmitExpr picks the form's hypermedia submit action; empty means the
// form carries no Datastar submit wiring.
func formSubmitExpr(p FormProps) string {
	switch {
	case p.Post != "":
		return "@post('" + p.Post + "')"
	case p.Get != "":
		return "@get('" + p.Get + "')"
	}
	return ""
}

// deltaIcon/deltaLabel pair a stat delta direction with an arrow glyph and a
// screen-reader label so color is never the only encoding.
func deltaIcon(dir string) string {
	switch dir {
	case "up":
		return "arrow-up"
	case "down":
		return "arrow-down"
	}
	return "arrow-right"
}

func deltaLabel(dir string) string {
	switch dir {
	case "up":
		return "up"
	case "down":
		return "down"
	}
	return "no change"
}

// deltaClass is the direction modifier on a stat delta; empty Dir gets none.
func deltaClass(dir string) templ.KeyValue[string, bool] {
	if dir == "" {
		return maybeClass("")
	}
	return maybeClass("dw-delta-" + dir)
}

// gridStyle pins the grid's column count and gap on CSS custom properties the
// kit sheet clamps responsively (no media queries in Go). The values are
// developer-supplied design tokens, not user input, hence SafeCSS.
func gridStyle(cols int, gap string) templ.SafeCSS {
	if cols < 1 {
		cols = 3
	}
	s := "--dw-grid-cols:" + strconv.Itoa(cols)
	if gap != "" {
		s += ";--dw-grid-gap:" + gap
	}
	return templ.SafeCSS(s)
}

// containerStyle pins an optional max-width to a size token; never a literal length.
func containerStyle(maxWidth string) templ.SafeCSS {
	return templ.SafeCSS("max-width:var(--size-" + maxWidth + ")")
}

// skeletonStyle reserves the placeholder's exact box up front so late content
// fills reserved space and never shifts pixels (the no-pop-in law).
func skeletonStyle(p SkeletonProps) templ.SafeCSS {
	s := ""
	if p.Width != "" {
		s += "width:" + p.Width + ";"
	}
	if p.Height != "" {
		s += "height:" + p.Height + ";"
	}
	return templ.SafeCSS(s)
}

// progressPct clamps Value/Max into a 0–100 width percentage for the bar fill.
func progressPct(value, max float64) string {
	pct := value / max * 100
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return strconv.FormatFloat(pct, 'f', 2, 64)
}

// drawerSide normalizes Side: anything but "left" slides from the right.
func drawerSide(s string) string {
	if s == "left" {
		return "left"
	}
	return "right"
}

// fitClass maps a Fit to its object-fit class; cover is the default.
func fitClass(f Fit) string {
	if f == "" {
		f = FitCover
	}
	return "dw-fit-" + string(f)
}

// aspectClass maps a Ratio to its aspect-token class; empty reserves nothing.
func aspectClass(r Ratio) templ.KeyValue[string, bool] {
	if r == "" {
		return maybeClass("")
	}
	return maybeClass("dw-aspect-" + string(r))
}

// imgLoading picks the loading strategy: above-fold heroes opt into eager,
// everything else lazy-loads below the fold (the no-blocking law).
func imgLoading(eager bool) string {
	if eager {
		return "eager"
	}
	return "lazy"
}

// msgClass places a chat message: system notices, own messages, or others'.
func msgClass(m Message) string {
	switch {
	case m.System:
		return "is-system"
	case m.Own:
		return "is-own"
	default:
		return "is-other"
	}
}

// presenceClass is the connection-state modifier on a presence pill.
func presenceClass(state string) templ.KeyValue[string, bool] {
	if state == "" {
		return maybeClass("")
	}
	return maybeClass("is-" + state)
}

// pageLang defaults the document language to English.
func pageLang(lang string) string {
	if lang == "" {
		return "en"
	}
	return lang
}

// appHref is the kit's single render-blocking stylesheet: the compiled CSS
// (reset, tokens, dw-* and chart classes), served from the framework static tree.
const appHref = "/static/css/app.css"

// pageStylesheet resolves the document stylesheet, honoring an override.
func pageStylesheet(href string) string {
	if href == "" {
		return appHref
	}
	return href
}

// errorAlertClass styles an error toast by severity: danger for failures the
// user must act on, warn for recoverable ones.
func errorAlertClass(danger bool) string {
	if danger {
		return "dw-toast-danger"
	}
	return "dw-toast-warn"
}

// errorAlertIcon pairs the severity with a glyph (color is never the only encoding).
func errorAlertIcon(danger bool) string {
	if danger {
		return "alert-octagon"
	}
	return "alert-triangle"
}
