package driftwood

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/Beach/pkg/prefs"
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

// --- spacing ------------------------------------------------------------------
//
// These build class names by concatenation, which is why the .dw-gap-* /
// .dw-pad-* / .dw-w-* rules in input.css are written out as plain CSS instead of
// generated on demand. Tailwind emits a utility only when its source scanner has
// seen the literal class name, and a name assembled in Go is a name it never
// sees — the same trap the chart classes are written out for.

// spaceClass maps a Space rung to a modifier class under the given prefix.
// SpaceAuto yields nothing at all, so the component's own rule stands: the zero
// value has to mean "you already know", not "zero".
func spaceClass(prefix string, s Space) templ.KeyValue[string, bool] {
	if s == SpaceAuto {
		return maybeClass("")
	}
	return maybeClass(prefix + "-" + string(s))
}

// measureClass maps a Measure to its width class, under a prefix so a container,
// a centred column and a rail can each carry their own ladder.
func measureClass(prefix string, m Measure) templ.KeyValue[string, bool] {
	if m == MeasureDefault {
		return maybeClass("")
	}
	return maybeClass(prefix + "-" + string(m))
}

// alignClass and justifyClass are the same shape for the flex axes.
func alignClass(prefix string, a Align) templ.KeyValue[string, bool] {
	if a == AlignStretch {
		return maybeClass("")
	}
	return maybeClass(prefix + "-" + string(a))
}

func justifyClass(prefix string, j Justify) templ.KeyValue[string, bool] {
	if j == JustifyStart {
		return maybeClass("")
	}
	return maybeClass(prefix + "-" + string(j))
}

// centerWidth and proseWidth give Center and Prose a different default from
// Container's. Both exist to hold text, so an unset width means the readable
// measure rather than the full page column — the zero value has to be the good
// answer, or the prop gets set to the same thing at every call site.
func centerWidth(m Measure) Measure {
	if m == MeasureDefault {
		return MeasureText
	}
	return m
}

func proseWidth(m Measure) Measure {
	if m == MeasureDefault {
		return MeasureText
	}
	return m
}

// railSideClass picks which edge a Rail's secondary column takes. Anything other
// than "start" is the default end placement — an unrecognized value must not
// silently drop the rail.
func railSideClass(side string) templ.KeyValue[string, bool] {
	if side == "start" {
		return maybeClass("dw-rail-start")
	}
	return maybeClass("dw-rail-end")
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

// alertRole picks a live-region role that matches the severity: role=alert
// interrupts, role=status waits for a pause. Only warn and danger have earned
// an interruption.
func alertRole(r Role) string {
	switch r {
	case RoleWarn, RoleDanger:
		return "alert"
	}
	return "status"
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

// ariaBool renders a Go bool as the "true"/"false" string ARIA state attributes
// expect (aria-pressed, aria-expanded, ...).
func ariaBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ariaBoolExpr is ariaBool for a *reactive* state: it wraps a Datastar
// expression so the attribute is written as the string "true" or "false".
//
// This is not decoration. Datastar treats a boolean-valued data-attr as an HTML
// boolean attribute — it writes `aria-pressed=""` when true and removes the
// attribute when false. For `disabled` that is exactly right; for an ARIA state
// it is wrong twice over: an empty value is not a valid state, and a *removed*
// aria-pressed does not mean "not pressed", it means "not a toggle button at
// all", so the control silently stops being one the moment it is switched off.
func ariaBoolExpr(expr string) string {
	return "(" + expr + ") ? 'true' : 'false'"
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

func deltaLabel(ctx context.Context, dir string) string {
	switch dir {
	case "up":
		return i18n.T(ctx, "ui.a11y.delta.up")
	case "down":
		return i18n.T(ctx, "ui.a11y.delta.down")
	}
	return i18n.T(ctx, "ui.a11y.delta.none")
}

// deltaClass is the direction modifier on a stat delta; empty Dir gets none.
func deltaClass(dir string) templ.KeyValue[string, bool] {
	if dir == "" {
		return maybeClass("")
	}
	return maybeClass("dw-delta-" + dir)
}

// gridStyle pins the grid's column count on a CSS custom property the kit sheet
// clamps responsively (no media queries in Go). The count is a developer-supplied
// number, not user input, hence SafeCSS. Gap and cell width are classes off the
// Space and Measure ladders, so they are not here.
func gridStyle(cols int) templ.SafeCSS {
	if cols < 1 {
		cols = 3
	}
	return templ.SafeCSS("--dw-grid-cols:" + strconv.Itoa(cols))
}

// dashColClass builds a DashCol's span classes: a column span clamped to the
// sheet's supported set (3/4/6/8/12, default 12) plus an optional row span (1–4).
func dashColClass(span, rows int) string {
	switch span {
	case 3, 4, 6, 8, 12:
	default:
		span = 12
	}
	c := "dash-col-" + strconv.Itoa(span)
	if rows >= 1 && rows <= 4 {
		c += " dash-row-" + strconv.Itoa(rows)
	}
	return c
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

// pageLang resolves the document language: an explicit PageProps.Lang wins,
// then the request's active i18n locale, then English. Declaring the right
// language is what lets a screen reader pick the right voice and pronunciation
// rules (WCAG 3.1.1), so the default has to follow the content — which, for a
// localized app, means following the locale the response was rendered in.
func pageLang(ctx context.Context, lang string) string {
	if lang != "" {
		return lang
	}
	if loc := i18n.Locale(ctx); loc != "" {
		return loc
	}
	return "en"
}

// pageDir resolves the document's base writing direction from the resolved
// language, unless PageProps.Dir overrides it.
func pageDir(ctx context.Context, p PageProps) string {
	if p.Dir != "" {
		return p.Dir
	}
	return string(i18n.Dir(pageLang(ctx, p.Lang)))
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

// ThemeHref is where the page links the derived design tokens. It is spelled out
// here rather than imported from pkg/beach because the kit cannot import the HTTP
// layer — the same reason pkg/prefs exists. beach.ThemePath is the other half,
// and TestThemeHrefMatchesTheRoute holds the two together.
const ThemeHref = "/_beach/theme.css"

// pageScheme returns the data-theme value for this request, or "" when the
// visitor has expressed no preference.
//
// Empty is not a fallback here, it is the third state: no attribute means the
// prefers-color-scheme query in the stylesheet decides, which is the only way
// the operating system's setting can be honoured. Stamping a guessed default
// would silently override it.
func pageScheme(ctx context.Context) string {
	switch prefs.ColorScheme(ctx) {
	case prefs.SchemeLight:
		return "light"
	case prefs.SchemeDark:
		return "dark"
	}
	return ""
}

// schemeOption is one radio in a [SchemeToggle].
type schemeOption struct{ Value, Label string }

// schemeOptions lists the three choices, named from the catalog. Auto comes
// first because it is the state a visitor starts in, and the one they should be
// able to get back to.
func schemeOptions(ctx context.Context) []schemeOption {
	return []schemeOption{
		{"auto", i18n.T(ctx, "ui.a11y.scheme.auto")},
		{"light", i18n.T(ctx, "ui.a11y.scheme.light")},
		{"dark", i18n.T(ctx, "ui.a11y.scheme.dark")},
	}
}

// schemeFormValue maps the stamped attribute back to the radio value. The empty
// attribute — no explicit choice — is the "auto" radio.
func schemeFormValue(stamped string) string {
	if stamped == "" {
		return "auto"
	}
	return stamped
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

// --- accessible names ---------------------------------------------------------
//
// Every name the kit puts in front of assistive technology is a catalog key, not
// a literal. A name a screen reader reads out is content: on a page rendered in
// Spanish, "Close" is as wrong as an untranslated heading would be. The keys and
// their English defaults ship embedded in pkg/i18n, so an app that configures no
// locales gets exactly this wording and pays nothing, while one that ships an
// es-ES.json gets the whole kit announced in Spanish for free.
//
// beach-vet's a11y-literal-name rule holds the line: a literal aria-label
// inside pkg/ui is a finding.

// a11yName picks a caller-supplied name over the kit's default.
//
// The default arrives already translated, rather than as a catalog key this
// function would look up. That is deliberate: `beach i18n` finds keys by
// matching literal i18n.T("…") calls, so a key passed through a helper is a key
// the extractor cannot see, and it would be reported as stale and eventually
// deleted from the catalog while still being used. Keeping the lookup at the
// call site keeps the key set mechanically verifiable, which is the whole point
// of the extractor.
func a11yName(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

// overlayTitleID names the heading a dialog is labelled by. A dialog with an
// ID hangs the heading id off it so two dialogs on one page stay distinct;
// without one it falls back to the kit default, which is fine because a page
// with two unnamed dialogs has a bigger problem than duplicate ids.
func overlayTitleID(id, fallback string) string {
	return defaultID(id, fallback) + "-title"
}

// --- field descriptions -------------------------------------------------------

// fieldDescKey is the context key under which a Field publishes the ids of the
// help and error text it rendered.
type fieldDescKey struct{}

// fieldDesc is what a Field tells the control it wraps: which elements describe
// it, and whether it is currently invalid. Carrying this on the context is what
// lets `@Field(...) { @Input(...) }` wire up aria-describedby with nothing at
// the call site — Field cannot reach into an opaque templ.Component child, and
// making callers repeat matching ids by hand is precisely the manual sync that
// leaves real forms unlabelled.
type fieldDesc struct {
	describedBy string // space-separated ids, "" when the field has neither help nor error
	invalid     bool
}

// withFieldDesc returns a context carrying d, for a Field to hand its children.
func withFieldDesc(ctx context.Context, d fieldDesc) context.Context {
	return context.WithValue(ctx, fieldDescKey{}, d)
}

// fieldDescFrom reads the enclosing Field's description, if there is one. A
// control rendered outside a Field gets the zero value and behaves as before.
func fieldDescFrom(ctx context.Context) fieldDesc {
	if ctx == nil {
		return fieldDesc{}
	}
	if d, ok := ctx.Value(fieldDescKey{}).(fieldDesc); ok {
		return d
	}
	return fieldDesc{}
}

// newFieldDesc derives the ids Field will stamp on its help and error text. The
// ids hang off the control id so they are stable across a re-render (a patched
// fragment must not renumber them under a screen reader's cursor).
func newFieldDesc(p FieldProps) fieldDesc {
	base := p.For
	if base == "" {
		return fieldDesc{invalid: p.Error != ""}
	}
	var ids []string
	if p.Help != "" && p.Error == "" {
		ids = append(ids, base+"-help")
	}
	if p.Error != "" {
		ids = append(ids, base+"-error")
	}
	return fieldDesc{describedBy: strings.Join(ids, " "), invalid: p.Error != ""}
}

// fieldHelpID / fieldErrorID name the two description elements Field renders.
// Both return "" when there is no control id to hang them off, in which case
// Field wraps the control in its label and the description rides along inside.
func fieldHelpID(p FieldProps) string  { return suffixID(p.For, "-help") }
func fieldErrorID(p FieldProps) string { return suffixID(p.For, "-error") }

// fieldsetHelpID names a Fieldset's group-level help line, or "" when there is
// nothing to name (no help, or no id to hang it off).
func fieldsetHelpID(p FieldsetProps) string {
	if p.Help == "" {
		return ""
	}
	return suffixID(p.ID, "-help")
}

func suffixID(base, suffix string) string {
	if base == "" {
		return ""
	}
	return base + suffix
}

// controlInvalid reports whether a control should carry aria-invalid: either it
// was told so directly, or the Field around it is showing an error.
func controlInvalid(ctx context.Context, own bool) bool {
	return own || fieldDescFrom(ctx).invalid
}

// --- sortable table headers ---------------------------------------------------

// sortAria maps a column's sort state onto the aria-sort token. Unsorted
// columns get "none", which is what tells a screen reader the header is
// sortable at all rather than merely present.
func sortAria(d SortDir) string {
	switch d {
	case SortAsc, SortDesc:
		return string(d)
	}
	return "none"
}

// sortIcon pairs the sort state with a directional glyph, so the state is
// visible and not only announced.
func sortIcon(d SortDir) string {
	switch d {
	case SortAsc:
		return "arrow-up"
	case SortDesc:
		return "arrow-down"
	}
	return "chevron-up-down"
}

// sortLabel is the sortable header button's accessible name: the column heading
// plus its current state, so activating it is an informed choice.
func sortLabel(ctx context.Context, c Column) string {
	switch c.Sort {
	case SortAsc:
		return i18n.T(ctx, "ui.a11y.sort.asc", c.Header)
	case SortDesc:
		return i18n.T(ctx, "ui.a11y.sort.desc", c.Header)
	}
	return i18n.T(ctx, "ui.a11y.sort.none", c.Header)
}

// --- tabs ---------------------------------------------------------------------

// tabRadioID is the id shared by a tab's radio and its label's `for`.
func tabRadioID(group string, t Tab) string { return group + "-" + t.ID }

// tabsInLadder caps a tab set at MaxTabs: the kit sheet's per-index rules stop
// there, so a ninth tab would render a label that switches nothing. Truncating
// is the visible failure, which is the one worth having.
func tabsInLadder(tabs []Tab) []Tab {
	if len(tabs) > MaxTabs {
		return tabs[:MaxTabs]
	}
	return tabs
}

// --- media --------------------------------------------------------------------

// avatarStatusLabel is what an avatar's status dot is announced as. The Status
// slug is a styling hook and reads badly out loud ("away-long"), so a caller-
// supplied label wins; the slug is the last resort, which is still better than
// a dot that says nothing.
func avatarStatusLabel(p AvatarProps) string {
	if p.StatusLabel != "" {
		return p.StatusLabel
	}
	return p.Status
}

// composerLabel names the message box: the caller's Label, then the placeholder
// (which at least says what the box is for), then the catalog. A composer is
// never nameless, because a nameless multi-line text box is announced as
// nothing at all.
func composerLabel(ctx context.Context, p ComposerProps) string {
	if p.Label != "" {
		return p.Label
	}
	return a11yName(p.Placeholder, i18n.T(ctx, "ui.a11y.composer"))
}

// inputGroupInput fills in the group's input name from the button when the
// caller gave the input none. See InputGroup.
func inputGroupInput(p InputGroupProps) TextInputProps {
	in := p.Input
	if in.Label == "" {
		in.Label = p.Button.Label
	}
	return in
}

// imgAlt resolves an image's text alternative. A decorative image gets the empty
// alt that hides it from assistive technology; everything else gets its Alt.
func imgAlt(p ImageProps) string {
	if p.Decorative {
		return ""
	}
	return p.Alt
}

// --- headings -----------------------------------------------------------------

// headingLevel clamps a caller-supplied heading level into 2–6, falling back to
// def when unset. h1 is not offered: a document has one, and it belongs to
// PageHeading.
func headingLevel(level, def int) int {
	if level < 2 || level > 6 {
		return def
	}
	return level
}

// --- live updates ---------------------------------------------------------------

// isLive reports whether this request's visitor still wants server-pushed
// updates. Reading it from the context rather than a prop is what stops a page
// showing a "Pause" button while nothing is streaming: one source of truth, and
// it is the same one the framework's stream adapter obeys.
func isLive(ctx context.Context) bool { return prefs.LiveUpdates(ctx) }

// liveTargetState is the value the toggle's form posts: the state the visitor is
// asking to move *to*, not the one they are in.
func liveTargetState(live bool) string {
	if live {
		return "off"
	}
	return "on"
}

// liveIcon pairs the toggle's state with a glyph, so the control is not
// distinguishable by its wording alone.
func liveIcon(live bool) string {
	if live {
		return "pause"
	}
	return "play"
}

// liveButtonLabel is the control's visible text and its accessible name — the
// same string, because WCAG 2.5.3 wants the name to contain what is on screen.
// It names what is updating when the caller says, so a page with two streams has
// two distinguishable controls.
// Every key is written out rather than assembled, because `beach i18n` finds
// keys by matching literal T calls: a key built by concatenation is a key the
// extractor reports as stale and someone eventually deletes while it is in use.
func liveButtonLabel(ctx context.Context, p LiveToggleProps, live bool) string {
	switch {
	case live && p.Label != "":
		return i18n.T(ctx, "ui.a11y.live.pause_named", p.Label)
	case live:
		return i18n.T(ctx, "ui.a11y.live.pause")
	case p.Label != "":
		return i18n.T(ctx, "ui.a11y.live.resume_named", p.Label)
	default:
		return i18n.T(ctx, "ui.a11y.live.resume")
	}
}

// --- toast timing ---------------------------------------------------------------

// toastLife decides how long a toast lives, or zero for "until closed".
//
// The order matters and each step is a WCAG 2.2.1 remedy rather than a taste
// call: a visitor who switched auto-dismiss off gets no timer at all; an
// explicit Dismiss is the caller's informed choice and wins next; a toast
// carrying something to act on is kept, because an error that fades is an error
// nobody fixed; everything else takes the default.
func toastLife(ctx context.Context, p ToastProps) time.Duration {
	if !prefs.AutoDismiss(ctx) {
		return 0
	}
	if p.Dismiss == ToastPersist {
		return 0
	}
	if p.Dismiss > 0 {
		return p.Dismiss
	}
	if p.Role == RoleDanger || p.Role == RoleWarn {
		return 0
	}
	return ToastDefaultLife
}

// toastLifeStyle pins the fade duration on a custom property the sheet reads.
// The value is a duration the app chose, not user input, hence SafeCSS.
func toastLifeStyle(d time.Duration) templ.SafeCSS {
	return templ.SafeCSS("--dw-toast-life:" + strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + "s")
}

// --- video ----------------------------------------------------------------------

// videoPreload defaults to "none": the poster paints and nothing else is fetched
// until someone asks. A background video has to load to autoplay at all, so it
// takes metadata instead.
func videoPreload(p VideoProps) string {
	if p.Preload != "" {
		return p.Preload
	}
	if p.Background {
		return "metadata"
	}
	return "none"
}

// trackKind defaults a timed text track to captions.
//
// Captions rather than subtitles is the right default because captions include
// the non-speech sound that carries meaning, and a caller who has thought about
// the difference will say which they mean. A caller who has not is better served
// by the more complete of the two.
func trackKind(t Track) string {
	if t.Kind != "" {
		return t.Kind
	}
	return "captions"
}

// --- confirm --------------------------------------------------------------------

// confirmDescribe returns the id of a Confirm's message, or "" when there is no
// message to point at. An aria-describedby naming an element that does not exist
// is worse than none: a screen reader announces nothing and no validator
// notices, which is exactly how a description silently stops working.
func confirmDescribe(p ConfirmProps, msgID string) string {
	if p.Message == "" {
		return ""
	}
	return msgID
}

// confirmTitle / confirmCancel / confirmConfirm fall back to the catalog so the
// dialog is never half-English on a translated page. The confirm fallback is
// deliberately weak wording — a caller who has not named the verb should notice.
func confirmTitle(ctx context.Context, p ConfirmProps) string {
	return a11yName(p.Title, i18n.T(ctx, "ui.confirm.title"))
}

func confirmCancel(ctx context.Context, p ConfirmProps) string {
	return a11yName(p.CancelLabel, i18n.T(ctx, "ui.confirm.cancel"))
}

func confirmConfirm(ctx context.Context, p ConfirmProps) string {
	return a11yName(p.ConfirmLabel, i18n.T(ctx, "ui.confirm.confirm"))
}

// --- markdown editor --------------------------------------------------------

// mdCmd is one toolbar action. Name is the data-md-cmd token md-editor.js
// dispatches on; Icon is the ui.Icon glyph.
type mdCmd struct {
	Name string
	Icon string
}

func mdCommands() []mdCmd {
	return []mdCmd{
		{Name: "bold", Icon: "bold"},
		{Name: "italic", Icon: "italic"},
		{Name: "h2", Icon: "heading"},
		{Name: "link", Icon: "link"},
		{Name: "ul", Icon: "list"},
		{Name: "ol", Icon: "list-ol"},
		{Name: "code", Icon: "code"},
		{Name: "quote", Icon: "quote"},
		{Name: "image", Icon: "image"},
		{Name: "video", Icon: "video"},
	}
}

func mdEditorName(p MarkdownEditorProps) string {
	if p.Name != "" {
		return p.Name
	}
	return "body"
}

func mdEditorID(p MarkdownEditorProps) string {
	return "dw-md-" + mdEditorName(p)
}

func mdSourceLabel(ctx context.Context, p MarkdownEditorProps) string {
	return a11yName(p.Label, i18n.T(ctx, "ui.a11y.md.source"))
}

// mdCmdLabel names a toolbar button from the catalog. Every key is written
// out: beach i18n only sees literal T calls.
func mdCmdLabel(ctx context.Context, cmd string) string {
	switch cmd {
	case "bold":
		return i18n.T(ctx, "ui.a11y.md.bold")
	case "italic":
		return i18n.T(ctx, "ui.a11y.md.italic")
	case "h2":
		return i18n.T(ctx, "ui.a11y.md.h2")
	case "link":
		return i18n.T(ctx, "ui.a11y.md.link")
	case "ul":
		return i18n.T(ctx, "ui.a11y.md.ul")
	case "ol":
		return i18n.T(ctx, "ui.a11y.md.ol")
	case "code":
		return i18n.T(ctx, "ui.a11y.md.code")
	case "quote":
		return i18n.T(ctx, "ui.a11y.md.quote")
	case "image":
		return i18n.T(ctx, "ui.a11y.md.image")
	case "video":
		return i18n.T(ctx, "ui.a11y.md.video")
	}
	return cmd
}

// --- consent ----------------------------------------------------------------

const consentTitleID = "dw-consent-title"
const consentManagerTitleID = "dw-consent-mgr-title"

func consentCategories(ctx context.Context, p ConsentManagerProps) []ConsentCategory {
	if len(p.Categories) > 0 {
		return p.Categories
	}
	return []ConsentCategory{{
		Label:       i18n.T(ctx, "ui.consent.necessary"),
		Description: i18n.T(ctx, "ui.consent.necessary_help"),
		Necessary:   true,
		Allowed:     true,
	}}
}

func consentCatLabel(ctx context.Context, cat ConsentCategory) string {
	if cat.Label != "" {
		return cat.Label
	}
	if cat.Necessary {
		return i18n.T(ctx, "ui.consent.necessary")
	}
	if cat.Name != "" {
		return cat.Name
	}
	return i18n.T(ctx, "ui.consent.category")
}
