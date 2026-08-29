// Package driftwood is the-beach's component kit: a package of exported,
// package-level templ components that render clean semantic HTML. Every color,
// space, radius, and motion value comes from a CSS custom property defined in
// the app stylesheet; no literal hex/OKLCH ever appears in the markup. Markup
// carries short dw-* semantic classes styled in internal/view/css/input.css,
// which compiles into the single /static/css/app.css the Page shell links.
//
// Apps and the framework call the components directly:
//
//	@driftwood.Card(driftwood.CardProps{Heading: "Holdings"}) {
//		<p>body</p>
//	}
package driftwood

import (
	"context"

	"github.com/ThirdCoastInteractive/the-beach/pkg/datastar"
	"github.com/ThirdCoastInteractive/the-beach/pkg/i18n"
	"github.com/a-h/templ"
)

// Props structs are immutable by convention: build one, pass it, never mutate.
// Each may carry an optional datastar.Attrs for server round-trips only.

// Surface names a design surface a component renders on. The kit maps each to a
// background token (e.g. SurfacePanel -> var(--neutral-2)) and its matching
// -on ink; callers never write a color literal.
type Surface string

const (
	SurfacePanel Surface = "panel"
	SurfaceWell  Surface = "well"
	SurfacePlain Surface = ""
)

// Role is a semantic accent shared by buttons, badges, and alerts. It selects a
// design palette (--primary-*, --error-*, ...), never a literal color.
type Role string

const (
	RoleAccent  Role = "accent"
	RoleDanger  Role = "danger"
	RoleGhost   Role = "ghost"
	RoleQuiet   Role = "quiet"
	RoleGood    Role = "good"
	RoleWarn    Role = "warn"
	RoleInfo    Role = "info"
	RoleNeutral Role = ""
)

// Size is a fluid-scale step (--size-*) shared across controls.
type Size string

const (
	SizeSm Size = "sm"
	SizeMd Size = ""
	SizeLg Size = "lg"
)

// Ratio names an aspect-ratio step for the media box, mapped to a dw-aspect-*
// class in app.css.
type Ratio string

const (
	RatioSquare Ratio = "square" // 1:1
	RatioWide   Ratio = "wide"   // 16:9
	RatioPhoto  Ratio = "photo"  // 4:3
	RatioPortr  Ratio = "portrait"
)

// Fit is the object-fit of media inside its aspect box.
type Fit string

const (
	FitCover   Fit = "cover"
	FitContain Fit = "contain"
)

// --- Layout ---

type ContainerProps struct {
	MaxWidth string // a size token name or empty for the default gutter
	Attrs    datastar.Attrs
}

type CardProps struct {
	Surface Surface // panel (default) or well
	Heading string  // optional title rendered in the card header
	// Level is the heading level for Heading, 2–6. It defaults to 3, which is
	// right under a SectionHeading (h2) and wrong directly under a PageHeading
	// (h1) — a skipped level is how a screen reader's heading list loses the
	// shape of the page (WCAG 1.3.1). The kit cannot know where a card sits, so
	// the caller owns the outline.
	Level int
	Attrs datastar.Attrs
}

type PageHeadingProps struct {
	Title    string
	Subtitle string
	Attrs    datastar.Attrs
}

type SectionHeadingProps struct {
	Label string
	// Level is the heading level, 2–6; defaults to 2.
	Level int
	Attrs datastar.Attrs
}

type DividerProps struct {
	Label string // optional labeled divider
	Attrs datastar.Attrs
}

// AppShellProps is the top-level page frame: a topbar across the top, optional
// fixed sidebar on the left, and the main content area. Sidebar is rendered when
// non-nil. The children of Shell are the main region.
type AppShellProps struct {
	Topbar  templ.Component // optional navbar/topbar slot
	Sidebar templ.Component // optional left sidebar nav; nil = stacked shell
	// SidebarLabel names the <aside> landmark so a screen reader's landmark
	// list distinguishes it from the topbar. Defaults to the kit's translated
	// "Sidebar"; set it when an app's rail holds something more specific.
	SidebarLabel string
	// Embedded marks this Shell as a preview inside another page rather than
	// the page root — the shape a component gallery or a docs page needs.
	//
	// A page has exactly one main landmark, one bypass link, and one live
	// region. A second Shell on the same page would duplicate all three, along
	// with their ids, which is worse than having none: a screen reader's
	// landmark list gains a phantom, and a patch aimed at the live region has
	// two targets. So an embedded Shell renders its frame and nothing else.
	Embedded bool
	Attrs    datastar.Attrs
}

// CardHeadingProps is a richer card header: title + optional meta line + an
// action slot rendered on the right (a Card's Heading string is the simple case).
type CardHeadingProps struct {
	Title string
	// Level is the heading level for Title, 2–6; defaults to 3. See CardProps.
	Level  int
	Meta   string          // optional secondary line
	Action templ.Component // optional right-aligned action slot
	Attrs  datastar.Attrs
}

// GridProps is a responsive dashboard widget grid. Cols is the column count at
// the widest breakpoint (the kit collapses it down responsively); cells are the
// children.
type GridProps struct {
	Cols  int // target columns at the widest breakpoint (default 3)
	Gap   string
	Attrs datastar.Attrs
}

// SplitProps is a two-pane master-detail layout: a list pane and a detail pane.
type SplitProps struct {
	List   templ.Component
	Detail templ.Component
	Attrs  datastar.Attrs
}

// DashGridProps is the responsive 12-column dashboard grid: a single column on a
// phone, six columns at the md breakpoint and twelve at lg (denser again past an
// ultrawide 1920px). Children are DashCol cells that declare their span, so a
// layout reads as stacked rows on mobile and side-by-side columns on a desktop.
type DashGridProps struct {
	Attrs datastar.Attrs
}

// DashColProps is one cell of a DashGrid. Span is the column span at the widest
// breakpoint (one of 3, 4, 6, 8, 12 of 12); the grid collapses it toward a
// full-width row as the viewport narrows. Rows optionally spans 1–4 grid rows for
// a taller cell.
type DashColProps struct {
	Span  int // 3, 4, 6, 8, or 12 (defaults to 12)
	Rows  int // optional row span 1–4 (0 = single row)
	Attrs datastar.Attrs
}

// --- Nav ---

// NavItem is one link in a navbar/sidebar/breadcrumb trail.
type NavItem struct {
	Label  string
	Href   string
	Active bool
	Icon   string // optional ui icon name
}

type NavbarProps struct {
	Brand string
	Items []NavItem
	Attrs datastar.Attrs
}

type BreadcrumbsProps struct {
	Items []NavItem // last item is the current page
	Attrs datastar.Attrs
}

// Tab is one tab; Get is set only when the panel is deferred (CSS otherwise).
type Tab struct {
	ID     string
	Label  string
	Active bool
	Get    string // @get target for a deferred panel; empty = inline CSS tab
	// Panel is the tab's content. A Tab with no Panel renders an empty panel
	// box — which is what a Get-deferred tab wants, since the fragment patches
	// into it — but a tab set with no panels at all is a bug, not a design.
	Panel templ.Component
}

// MaxTabs is how many tabs one Tabs set can switch between. Selecting a panel
// from a checked radio is a per-index CSS rule, so the kit sheet carries a
// ladder of exactly this many ("the tab ladder" in input.css) — keep the two
// in step. A surface wanting a ninth tab wants navigation, not tabs.
const MaxTabs = 8

type TabsProps struct {
	Name string // radio group name for the CSS :checked mechanism
	// Label names the tab set for assistive technology. The tabs are a radio
	// group, and a radio group without a name is announced as an anonymous set
	// of choices (WCAG 4.1.2).
	Label string
	Tabs  []Tab
	Attrs datastar.Attrs
}

type PaginationProps struct {
	Page    int
	Pages   int
	GetBase string // @get base; the kit appends ?page=N
	Attrs   datastar.Attrs
}

// SidebarSection groups sidebar items under an optional label.
type SidebarSection struct {
	Label string
	Items []NavItem
}

// SidebarNavProps is the fixed left-rail navigation: labeled sections of items
// with icons and active state.
type SidebarNavProps struct {
	Sections []SidebarSection
	Attrs    datastar.Attrs
}

// --- Buttons ---

// ButtonProps configures a Button.
//
// Label (or child text) is what names the button. A button carrying only
// LeadIcon or TailIcon has no accessible name and is announced as "button" —
// use IconButton for that shape instead, which takes the name as a field and so
// cannot be built without one.
type ButtonProps struct {
	Label    string
	Role     Role
	Size     Size
	Hazard   bool // diagonal warning stripes — texture for irreversible/destructive actions
	LeadIcon string
	TailIcon string
	Loading  string // signal expression that flips the loading state, e.g. "$busy"
	Disabled bool
	Type     string // "button" (default), "submit", "reset"
	Attrs    datastar.Attrs
}

type IconButtonProps struct {
	Icon  string
	Label string // accessible label / tooltip
	Role  Role
	Size  Size
	Attrs datastar.Attrs
}

// ButtonGroupProps is a segmented control: the children render joined into a
// single grouped control.
type ButtonGroupProps struct {
	Label string // accessible group label
	Attrs datastar.Attrs
}

// Segment is one toggle button in a Segmented control.
type Segment struct {
	Value  string
	Label  string
	Icon   string // optional leading icon
	Active bool
	On     string // optional @-expression run on click (e.g. "@get('/range/day')")
}

// SegmentedProps is a single-select toggle-button control: a joined row of
// buttons where exactly one is the active choice (aria-pressed). Unlike
// ButtonGroup (visual join only), the active segment carries selected styling
// and the group announces its purpose. Name is the accessible group label.
type SegmentedProps struct {
	Name     string // accessible group label
	Segments []Segment
	Size     Size
	Attrs    datastar.Attrs
}

// MenuItem is one entry in a menu-button popover.
type MenuItem struct {
	Label string
	Href  string // a link item; empty + Action makes a hypermedia item
	Icon  string
	On    string // optional @-expression run on click (e.g. "@post('/x')")
}

// MenuButtonProps is a label + caret that opens a native popover menu (no JS to
// toggle: the popover attribute drives it).
type MenuButtonProps struct {
	ID    string // anchor id wiring the trigger to its popover
	Label string
	Icon  string
	Role  Role
	Size  Size
	Items []MenuItem
	Attrs datastar.Attrs
}

// --- Forms ---

// FieldProps is one label / control / help / error row.
//
// The association is automatic. Field publishes the ids of whatever Help and
// Error text it renders onto the context it hands its children, and the kit's
// controls (Input, Textarea, Select, Checkbox) read them back to fill in their
// own aria-describedby and aria-invalid. So a plain
//
//	@Field(FieldProps{Label: "Name", For: "name", Error: err}) {
//		@Input(TextInputProps{ID: "name", Name: "name"})
//	}
//
// announces "Name, invalid entry, <the error>" with nothing else to wire up
// (WCAG 1.3.1 / 3.3.1). With For empty the label wraps the control instead of
// pointing at it, so the association holds either way.
type FieldProps struct {
	Label string
	For   string // id of the labelled control; empty makes the label wrap it
	Help  string
	Error string // server-rendered field error
	// Required marks the field's label so the requirement is visible and
	// announced. It does not set the control's required attribute — that is
	// TextInputProps.Required, on the control that owns the constraint.
	Required bool
	Attrs    datastar.Attrs
}

// FieldsetProps groups related fields under a legend (a styled
// <fieldset>/<legend>, so the group never falls back to the UA default on
// dark). Help renders a description line under the legend; the grouped controls
// are the children.
type FieldsetProps struct {
	// ID is what the group's help line hangs off, so the <fieldset> can point
	// aria-describedby at it. Without an ID the help still renders but is only
	// visually associated — pass one whenever there is Help.
	ID       string
	Legend   string
	Help     string
	Disabled bool // disable every control in the group at once
	Attrs    datastar.Attrs
}

type TextInputProps struct {
	ID   string
	Name string
	// Label is the control's accessible name for the case where there is no
	// visible label to point at it — a search box, a filter in a toolbar. Inside
	// a Field, leave it empty: the Field's own label is the name, and setting
	// both means the visible text and the announced text can drift apart.
	Label string

	Type         string // "text" (default), "email", "password", "date", ...
	Value        string
	Placeholder  string
	Prefix       string
	Suffix       string
	Min          string // numeric/date lower bound (min attribute)
	Max          string // numeric/date upper bound (max attribute)
	Step         string // numeric step granularity (step attribute; "any" for free)
	InputMode    string // virtual-keyboard hint: "numeric", "decimal", "tel", ...
	Autocomplete string // autocomplete token, e.g. "off", "email", "one-time-code"
	Spellcheck   string // "true"/"false" — disable for codes, units, identifiers
	Required     bool   // native required + aria-required
	Error        bool   // force the invalid state; an enclosing Field's Error implies it
	Disabled     bool
	Attrs        datastar.Attrs
}

type TextareaProps struct {
	ID   string
	Name string
	// Label is the control's accessible name for the case where there is no
	// visible label to point at it — a search box, a filter in a toolbar. Inside
	// a Field, leave it empty: the Field's own label is the name, and setting
	// both means the visible text and the announced text can drift apart.
	Label    string
	Value    string
	Rows     int
	AutoGrow bool
	Error    bool
	Attrs    datastar.Attrs
}

// Option is one <option> in a Select.
type Option struct {
	Value    string
	Label    string
	Selected bool
	Disabled bool
}

// OptionGroup is a labelled set of options rendered as an <optgroup>. A Select
// uses Groups for grouped dropdowns and Options for the flat case; when both are
// set the flat Options render first, then the groups.
type OptionGroup struct {
	Label   string
	Options []Option
}

type SelectProps struct {
	ID   string
	Name string
	// Label is the control's accessible name for the case where there is no
	// visible label to point at it. Inside a Field, leave it empty: the Field's
	// own label is the name.
	Label       string
	Options     []Option      // flat options (the simple case)
	Groups      []OptionGroup // grouped options, rendered as <optgroup>
	Placeholder string
	Disabled    bool
	Error       bool
	Attrs       datastar.Attrs
}

// FormProps is the form element wrapper: it sets the layout (stacked or 2-column)
// and an optional @post/@get target. Fields are the children.
type FormProps struct {
	Post   string // @post target (hypermedia submit)
	Get    string // @get target (alternative to Post)
	TwoCol bool   // 2-column label/control layout vs stacked
	Attrs  datastar.Attrs
}

// CheckboxProps is a single checkbox with an inline label.
type CheckboxProps struct {
	ID      string
	Name    string
	Value   string
	Label   string
	Checked bool
	Error   bool
	Attrs   datastar.Attrs
}

// RadioOption is one choice in a RadioGroup.
type RadioOption struct {
	Value   string
	Label   string
	Checked bool
}

// RadioGroupProps is a set of radios sharing a Name; Inline lays them out in a row.
type RadioGroupProps struct {
	Name string
	// Label names the group for assistive technology. role=radiogroup without
	// an accessible name announces an anonymous set of choices (WCAG 4.1.2), so
	// this is effectively required — unless the group sits inside a Field, whose
	// label the group borrows automatically.
	Label   string
	Options []RadioOption
	Inline  bool
	Attrs   datastar.Attrs
}

// ToggleProps is a switch with a label and optional description.
type ToggleProps struct {
	ID          string
	Name        string
	Label       string
	Description string
	Checked     bool
	Attrs       datastar.Attrs
}

// InputGroupProps is a button-attached input (search box, chat send). The input
// is described by Input; Button is the attached trailing button.
type InputGroupProps struct {
	Input  TextInputProps
	Button ButtonProps
	Attrs  datastar.Attrs
}

// FormErrorProps is a form-level error summary banner (distinct from per-field
// errors). Messages lists the individual problems.
// FormErrorProps is the form-level error summary banner.
//
// Messages is the plain-text form; Problems is the same list with an anchor, so
// each entry links to the control it is about. Prefer Problems: a summary a
// keyboard user cannot act on is only half of WCAG 3.3.1. Both render; Messages
// come first.
type FormErrorProps struct {
	Title    string
	Messages []string
	Problems []Problem
	Attrs    datastar.Attrs
}

// Problem is one entry in a FormError summary: what is wrong, and the id of the
// control it is wrong about.
type Problem struct {
	Message string
	For     string // id of the offending control; empty renders plain text
}

// --- Data ---

// Column is a table header; Get is set when the header is sortable (@get).
type Column struct {
	Header string
	// HeaderHidden keeps the header text out of sight but in the accessible
	// tree — the answer for an actions or icon column, where an empty <th>
	// would leave that column's cells with no header at all (WCAG 1.3.1).
	HeaderHidden bool
	Get          string
	// Sort is the column's current sort state — SortAsc, SortDesc, or the zero
	// value for unsorted. It drives aria-sort and picks a directional chevron,
	// so the state is announced and visible rather than implied by the data
	// order alone (WCAG 1.3.1).
	Sort SortDir
}

// SortDir is a sortable column's current state.
type SortDir string

const (
	SortNone SortDir = ""
	SortAsc  SortDir = "ascending"
	SortDesc SortDir = "descending"
)

type TableProps struct {
	Columns []Column
	Rows    [][]string
	Dense   bool
	Empty   string // empty-state message rendered when Rows is empty
	// Caption names the table. A data table with no caption and no surrounding
	// heading is an unlabelled grid to anyone navigating by table (WCAG 1.3.1).
	// It renders as a visible <caption> unless CaptionHidden is set.
	Caption       string
	CaptionHidden bool
	Attrs         datastar.Attrs
}

// Delta is a stat's change indicator; Dir is "up"/"down"/"" and is never the
// only encoding (the kit pairs it with an arrow glyph and a label).
type Delta struct {
	Value string
	Dir   string
}

type StatProps struct {
	Label string
	Value string
	Delta Delta
	Attrs datastar.Attrs
}

type BadgeProps struct {
	Label string
	Role  Role
	Dot   bool
	Attrs datastar.Attrs
}

// DescItem is one key/value pair in a description list.
type DescItem struct {
	Term  string
	Value string
}

// DescListProps is a key/value grid. TwoCol renders terms and values side by
// side; otherwise a single-column stack.
type DescListProps struct {
	Items  []DescItem
	TwoCol bool
	Attrs  datastar.Attrs
}

// ListRow is one row in a StackedList: an optional leading avatar, a title and
// meta line, and an optional trailing action.
type ListRow struct {
	Avatar templ.Component // optional leading media slot
	Title  string
	Meta   string
	Href   string          // makes the row a link
	Action templ.Component // optional trailing action slot
	Badge  *BadgeProps     // optional trailing status badge
}

// StackedListProps is a vertical list of rows (feeds, item lists).
type StackedListProps struct {
	Rows  []ListRow
	Attrs datastar.Attrs
}

// GridListProps is a responsive card grid (property tiles, item cards). Cells are
// the children; Cols is the target column count.
type GridListProps struct {
	Cols  int
	Gap   string
	Attrs datastar.Attrs
}

// --- Feedback ---

type AlertProps struct {
	Role        Role // good/warn/bad(info)
	Title       string
	Dismissible bool
	Attrs       datastar.Attrs
}

type EmptyStateProps struct {
	Icon    string
	Message string
	Attrs   datastar.Attrs
}

type SpinnerProps struct {
	Size  Size
	Block bool // block (centered) vs inline
	Attrs datastar.Attrs
}

type SkeletonProps struct {
	Width  string
	Height string
	Attrs  datastar.Attrs
}

// ToastProps is a single notification card, SSE-pushable and stacked by the
// kit's toast region. It carries a role, a message, and a manual close control.
type ToastProps struct {
	Role    Role
	Title   string
	Message string
	Attrs   datastar.Attrs
}

// ProgressProps is a progress bar. When Max is 0 the bar is indeterminate
// (loads, unknown-duration work); otherwise it shows Value/Max.
type ProgressProps struct {
	Value float64
	Max   float64 // 0 = indeterminate
	Label string  // accessible label
	Role  Role
	Attrs datastar.Attrs
}

// --- Overlays ---

type ModalProps struct {
	ID    string
	Title string
	Get   string // optional @get that lazy-loads a heavy body under budget
	Attrs datastar.Attrs
}

// AuthModalProps configures a reusable login/register modal: a Modal (native
// <dialog>) holding two tabbed Forms. Each form posts its bound signals
// (username/password, + email/agree on register) to the given endpoints; the
// handler sets the session cookie and redirects on success, or patches an error
// fragment into the ErrorTarget slot on failure. Open it from a trigger with
// datastar.OnClick("document.getElementById('<ID>').showModal()").
// The forms submit as plain POSTs (native navigation), so the handler sets the
// session cookie and 303-redirects on success — no inline script, CSP-clean. On
// failure the handler redirects back with an error and the app re-renders this
// modal with OpenOnLoad + Error set so it reopens showing the message.
type AuthModalProps struct {
	ID           string // dialog id (default "auth-modal")
	Title        string // header text (default "Welcome")
	LoginPost    string // form action for login (e.g. "/auth/login")
	RegisterPost string // form action for register (e.g. "/auth/register")
	ErrorTarget  string // id of the error slot (default <ID>-error)
	ShowEmail    bool   // include an optional email field on the register form
	TOSHref      string // when set, register shows an "I accept the terms" checkbox + link
	OpenOnLoad   bool   // reopen the dialog on page load (after a failed attempt)
	Error        string // a message to show in the error slot
	Tab          string // initial tab: "login" (default) or "register"
	Attrs        datastar.Attrs
}

// authErrorID returns the error-slot id for an AuthModal (ErrorTarget or a
// derived default).
func authErrorID(p AuthModalProps) string {
	if p.ErrorTarget != "" {
		return p.ErrorTarget
	}
	return defaultID(p.ID, "auth-modal") + "-error"
}

// authTitle returns the AuthModal header (Title or a default).
func authTitle(ctx context.Context, p AuthModalProps) string {
	return a11yName(p.Title, i18n.T(ctx, "ui.auth.welcome"))
}

// authTabPressed binds a switcher button's aria-pressed to the tab signal, so
// the announced state follows the drawn state instead of being frozen at render
// time. It is the typed-builder route to a reactive attribute; a literal
// aria-pressed here would be a value that stops being true on the first click.
func authTabPressed(tab string) templ.Attributes {
	return datastar.Attrs{
		datastar.AttrBind("aria-pressed", ariaBoolExpr("$authtab === '"+tab+"'")),
	}.Templ()
}

// The following hold the Datastar expressions for the AuthModal tab signal. They
// live in Go (not inline in the .templ) because templ's attribute-expression
// parser is not string-aware and mis-balances the braces.

// authTab returns the initial tab ("login" default; "register" if requested).
func authTab(p AuthModalProps) string {
	if p.Tab == "register" {
		return "register"
	}
	return "login"
}

// authSignalsTab is the data-signals init for the tab signal.
func authSignalsTab(tab string) string { return "{authtab: '" + tab + "'}" }

// authOpenScript is the data-init expression that reopens the dialog on load.
func authOpenScript(id string) string {
	return "document.getElementById('" + id + "').showModal()"
}

// authTabClass is the data-class expression that accents the active tab button.
func authTabClass(tab string) string {
	return "{'dw-btn-accent': $authtab === '" + tab + "'}"
}

// TooltipProps is a hover/focus tip attached to its children.
//
// The tip describes the trigger (aria-describedby), so it is announced rather
// than merely displayed. Note the one place the kit knowingly falls short of
// WCAG 2.1: SC 1.4.13 asks that hover content be dismissible without moving
// focus, which a CSS-only tooltip cannot do — see the known exceptions in
// docs/rfc/06-accessibility.md. Use it for supplementary hints, never for
// information a user must have.
type TooltipProps struct {
	ID   string // id linking trigger to tip; defaults to a stable kit id
	Text string
	// Interactive marks the children as already focusable (a button, a link),
	// which stops the tooltip adding a tabindex of its own and creating a
	// second, redundant tab stop.
	Interactive bool
	Attrs       datastar.Attrs
}

// DrawerProps is a slide-over panel. Side is "left" or "right" (default right).
// Open/close is native: it is a <dialog> opened with showModal, same as Modal,
// styled to slide in from the edge. Get lazy-loads a heavy body under budget.
type DrawerProps struct {
	ID    string
	Title string
	Side  string // "left" or "right" (default "right")
	Get   string // optional @get that lazy-loads a heavy body under budget
	Attrs datastar.Attrs
}

// PopoverProps is a native popover flyout anchored to a trigger. The children
// are the panel body. Toggling is the native popover attribute (no script).
type PopoverProps struct {
	ID    string // popover id wiring trigger -> panel
	Label string // trigger label
	Icon  string // optional trigger icon
	Attrs datastar.Attrs
}

// --- Media ---

type AvatarProps struct {
	Src      string
	Initials string // fallback when Src is empty
	// Alt names the person or thing the avatar stands for. Leave it empty when
	// the avatar sits next to that name already — the image is then decorative
	// and repeating the name only doubles the announcement.
	Alt  string
	Size Size
	// Status is the status-dot slug ("online"/"away"/...). It selects the dot's
	// color, which is why StatusLabel exists: color alone is never the encoding.
	Status string
	// StatusLabel is what the dot is announced as. Empty falls back to Status,
	// which is a slug and reads poorly — pass the app's own translated wording.
	StatusLabel string
	Attrs       datastar.Attrs
}

type ImageProps struct {
	Src   string
	Set   string // srcset
	Sizes string
	// Alt is the image's text alternative and is required (WCAG 1.1.1). A
	// genuinely decorative image — a texture, a rule, a duplicate of adjacent
	// text — sets Decorative instead; leaving both empty is a beach-vet finding
	// (rule a11y-img-alt), because that is what an accident looks like.
	Alt string
	// Decorative marks the image as conveying nothing, hiding it from assistive
	// technology (alt="" plus aria-hidden). It is mutually exclusive with Alt.
	Decorative bool
	Ratio      Ratio
	Fit        Fit
	Eager      bool // above-fold hero opts into eager + fetchpriority=high
	Attrs      datastar.Attrs
}

type AspectBoxProps struct {
	Ratio Ratio
	Attrs datastar.Attrs
}

// FigureProps is an image with a caption (<figure>/<figcaption>). The image
// props are embedded so the figure inherits the media perf laws.
type FigureProps struct {
	Image   ImageProps
	Caption string
	Attrs   datastar.Attrs
}

// --- Messaging ---

// Message is one entry in a MessageList.
type Message struct {
	Author string
	Body   string
	Own    bool
	System bool
	At     string // pre-formatted timestamp
}

type MessageListProps struct {
	Messages []Message
	Attrs    datastar.Attrs
}

type ComposerProps struct {
	Name string // bound signal / field name
	// Label names the message box. A placeholder is not a label: it disappears
	// the moment there is text, and some screen readers never announce it
	// (WCAG 3.3.2). Empty falls back to Placeholder so an existing composer
	// still gets a name, but pass a real one.
	Label       string
	Placeholder string
	Post        string // @post target for send
	Attrs       datastar.Attrs
}

type PresencePillProps struct {
	State string // "connected"/"reconnecting"/"paired"/"waiting"
	Label string
	Attrs datastar.Attrs
}

// PageProps configures the document shell Page renders.
//
// Lang and Dir are overrides, not the normal path: both default to the request's
// active i18n locale, so a page served under an es-ES locale declares
// lang="es-ES" without the app doing anything (WCAG 3.1.1). Set them only when
// a page's language genuinely differs from the request's.
type PageProps struct {
	Title       string // <title> text
	Lang        string // <html lang>; defaults to the request locale, then "en"
	Dir         string // <html dir>; defaults to the direction of the active lang
	Description string // optional meta description
	BodyClass   string // optional extra class on <body>
	Stylesheet  string // override the kit stylesheet href (default /static/css/app.css)
}
