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
	"github.com/ThirdCoastInteractive/the-beach/datastar"
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
	Attrs   datastar.Attrs
}

type PageHeadingProps struct {
	Title    string
	Subtitle string
	Attrs    datastar.Attrs
}

type SectionHeadingProps struct {
	Label string
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
	Attrs   datastar.Attrs
}

// CardHeadingProps is a richer card header: title + optional meta line + an
// action slot rendered on the right (a Card's Heading string is the simple case).
type CardHeadingProps struct {
	Title  string
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
}

type TabsProps struct {
	Name  string // radio group name for the CSS :checked mechanism
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

type FieldProps struct {
	Label string
	For   string // id of the labelled control
	Help  string
	Error string // server-rendered field error
	Attrs datastar.Attrs
}

type TextInputProps struct {
	ID          string
	Name        string
	Type        string // "text" (default), "email", "password", "date", ...
	Value       string
	Placeholder string
	Prefix      string
	Suffix      string
	Error       bool
	Disabled    bool
	Attrs       datastar.Attrs
}

type TextareaProps struct {
	ID       string
	Name     string
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
}

type SelectProps struct {
	ID          string
	Name        string
	Options     []Option
	Placeholder string
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
	Name    string
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
type FormErrorProps struct {
	Title    string
	Messages []string
	Attrs    datastar.Attrs
}

// --- Data ---

// Column is a table header; Get is set when the header is sortable (@get).
type Column struct {
	Header string
	Get    string
}

type TableProps struct {
	Columns []Column
	Rows    [][]string
	Dense   bool
	Empty   string // empty-state message rendered when Rows is empty
	Attrs   datastar.Attrs
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

type TooltipProps struct {
	Text  string
	Attrs datastar.Attrs
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
	Alt      string
	Size     Size
	Status   string // optional status-dot role: "online"/"away"/...
	Attrs    datastar.Attrs
}

type ImageProps struct {
	Src   string
	Set   string // srcset
	Sizes string
	Alt   string // required by construction (a11y); empty is a vet smell
	Ratio Ratio
	Fit   Fit
	Eager bool // above-fold hero opts into eager + fetchpriority=high
	Attrs datastar.Attrs
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
	Name        string // bound signal / field name
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
type PageProps struct {
	Title       string // <title> text
	Lang        string // <html lang>, defaults to "en"
	Description string // optional meta description
	BodyClass   string // optional extra class on <body>
	Stylesheet  string // override the kit stylesheet href (default /static/css/app.css)
}
