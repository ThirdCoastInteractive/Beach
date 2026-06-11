// Package ui is the-beach's component toolkit and the home of the performance
// laws (no pop-in, no blocking, the 14KB rule, deferred sections). A themed
// implementation of the toolkit is a *kit*; ui/driftwood is the reference kit.
//
// This file defines the toolkit's spine: the ui.Kit interface and its
// sub-interfaces (Layout, Nav, Buttons, Forms, Data, Feedback, Overlays, Media,
// Messaging). Every method returns a templ.Component so a kit can render with
// templ or with plain Go closures (templ.ComponentFunc) interchangeably.
//
// Kits never hardcode color, type, space, motion, or depth: those come from
// Stellar (Datastar Pro) as CSS variables — a panel is background:
// var(--neutral-2), its ink var(--neutral-2-on); overlays stack on
// var(--zindex-dialog); and so on. The markup these interfaces produce
// references those tokens directly, never a literal hex/OKLCH.
//
// Props structs are immutable by convention: a handler constructs one, passes
// it to a Kit method, and never mutates it afterward. Each carries an optional
// datastar.Attrs (the Attrs field) for the single sanctioned channel of
// data-* attributes — server round-trips only, never local presentational
// state, which is HTML and CSS per the HTML-and-CSS-first law.
package ui

import (
	"html"

	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/a-h/templ"
)

// Kit is the full component toolkit. An app receives one through the request
// context (see KitFrom / the App builder's c.Kit()) and renders pages by
// calling its sub-interfaces. A kit is a value, not a singleton: an app may run
// more than one (per-tenant theming), which is why it travels in context.
//
// Every method returns templ.Component, deferring rendering to the writer the
// page eventually flushes to. No method blocks, performs I/O, or mutates its
// props.
type Kit interface {
	// Name identifies the kit (e.g. "driftwood"). It is used in diagnostics and
	// to key the kit registry.
	Name() string

	Layout() Layout
	Nav() Nav
	Buttons() Buttons
	Forms() Forms
	Data() Data
	Feedback() Feedback
	Overlays() Overlays
	Media() Media
	Messaging() Messaging
}

// Layout is the page skeleton and the surfaces content sits on: app shells,
// containers, headings, cards, dividers, dashboard grids, master-detail splits.
type Layout interface {
	AppShell(AppShellProps, ...templ.Component) templ.Component
	Container(ContainerProps, ...templ.Component) templ.Component
	Card(CardProps, ...templ.Component) templ.Component
	CardHeading(CardHeadingProps) templ.Component
	PageHeading(PageHeadingProps) templ.Component
	SectionHeading(SectionHeadingProps) templ.Component
	Divider(DividerProps) templ.Component
	Grid(GridProps, ...templ.Component) templ.Component
	Split(SplitProps) templ.Component
}

// Nav is the app-shell navigation set: navbars, sidebars, tabs, breadcrumbs,
// pagination. Local open/closed and which-tab state is CSS, never script.
type Nav interface {
	Navbar(NavbarProps) templ.Component
	SidebarNav(SidebarNavProps) templ.Component
	Breadcrumbs(BreadcrumbsProps) templ.Component
	Tabs(TabsProps) templ.Component
	Pagination(PaginationProps) templ.Component
}

// Buttons covers buttons, icon buttons, button groups, and menu buttons. The
// loading state binds to a Datastar indicator signal via props.Attrs; the rest
// is markup and tokens.
type Buttons interface {
	Button(ButtonProps, ...templ.Component) templ.Component
	IconButton(IconButtonProps) templ.Component
	ButtonGroup(ButtonGroupProps, ...templ.Component) templ.Component
	MenuButton(MenuButtonProps) templ.Component
}

// Forms is the CRUD surface. Validation is server-rendered: a handler re-renders
// the form with field errors; there is no client-side validation library.
type Forms interface {
	Form(FormProps, ...templ.Component) templ.Component
	Field(FieldProps, ...templ.Component) templ.Component
	TextInput(TextInputProps) templ.Component
	Textarea(TextareaProps) templ.Component
	Select(SelectProps) templ.Component
	Checkbox(CheckboxProps) templ.Component
	RadioGroup(RadioGroupProps) templ.Component
	Toggle(ToggleProps) templ.Component
	InputGroup(InputGroupProps) templ.Component
	FormError(FormErrorProps) templ.Component
}

// Data shows rows, records, and numbers: tables, description lists, stats,
// stacked/grid lists, badges, and the chart-widget wrapper.
type Data interface {
	Table(TableProps) templ.Component
	DescList(DescListProps) templ.Component
	Stat(StatProps) templ.Component
	StackedList(StackedListProps) templ.Component
	GridList(GridListProps, ...templ.Component) templ.Component
	Badge(BadgeProps) templ.Component
}

// Feedback is alerts, toasts, empty states, progress, skeleton, spinner, and
// error pages. Skeleton is shared with ui.Defer's reserved-space placeholder.
type Feedback interface {
	Alert(AlertProps, ...templ.Component) templ.Component
	Toast(ToastProps) templ.Component
	EmptyState(EmptyStateProps) templ.Component
	Progress(ProgressProps) templ.Component
	Spinner(SpinnerProps) templ.Component
	Skeleton(SkeletonProps) templ.Component
}

// Overlays is modals, drawers, popovers, tooltips, and confirm presets. Open and
// close are native (<dialog>, the popover attribute); Datastar enters only to
// @get a heavy body, never to toggle visibility. Stacking uses
// var(--zindex-{drawer,dialog,dropdown,toast,tooltip}).
type Overlays interface {
	Modal(ModalProps, ...templ.Component) templ.Component
	Drawer(DrawerProps, ...templ.Component) templ.Component
	Popover(PopoverProps, ...templ.Component) templ.Component
	Tooltip(TooltipProps, ...templ.Component) templ.Component
}

// Media is avatars, icons, aspect-ratio boxes, images, figures, video. Every
// image and video sits in an aspect-ratio box that reserves its exact space —
// the no-pop-in law applied to media.
type Media interface {
	Avatar(AvatarProps) templ.Component
	Image(ImageProps) templ.Component
	Figure(FigureProps) templ.Component
	AspectBox(AspectBoxProps, ...templ.Component) templ.Component
}

// Messaging is the conversation surface: message lists, composers, typing
// indicators, presence pills, session banners.
type Messaging interface {
	MessageList(MessageListProps) templ.Component
	Composer(ComposerProps) templ.Component
	PresencePill(PresencePillProps) templ.Component
}

// withAttrs splices a props' Datastar attributes into a start tag. It is the one
// place a kit reaches for datastar.Attrs; it returns the rendered, space-leading
// attribute string (empty when there are none) so callers can write it verbatim.
func withAttrs(a datastar.Attrs) string {
	if len(a) == 0 {
		return ""
	}
	return " " + a.String()
}

// --- noopKit implementations of the expanded catalog -------------------------
//
// These live here (a file the catalog owns) rather than in noop.go so the
// built-in no-op kit keeps satisfying the full ui.Kit interface as the catalog
// grows. They are intentionally minimal: semantic markup, token classes, no
// hardcoded color — the themed driftwood kit is the real implementation.

func (k noopKit) AppShell(p AppShellProps, inner ...templ.Component) templ.Component {
	body := []templ.Component{}
	if p.Topbar != nil {
		body = append(body, el("header", "ui-shell-top", "", p.Topbar))
	}
	mainBody := el("main", "ui-shell-main", "", inner...)
	if p.Sidebar != nil {
		body = append(body, el("div", "ui-shell-cols", "",
			el("aside", "ui-shell-side", "", p.Sidebar), mainBody))
	} else {
		body = append(body, mainBody)
	}
	return el("div", "ui-shell", withAttrs(p.Attrs), body...)
}

func (noopKit) CardHeading(p CardHeadingProps) templ.Component {
	left := []templ.Component{el("h3", "ui-card-title", "", txt(p.Title))}
	if p.Meta != "" {
		left = append(left, el("p", "ui-card-meta", "", txt(p.Meta)))
	}
	body := []templ.Component{el("div", "ui-card-head-text", "", left...)}
	if p.Action != nil {
		body = append(body, el("div", "ui-card-head-action", "", p.Action))
	}
	return el("header", "ui-card-head", withAttrs(p.Attrs), body...)
}

func (noopKit) Grid(p GridProps, inner ...templ.Component) templ.Component {
	return el("div", "ui-grid", withAttrs(p.Attrs), inner...)
}

func (noopKit) Split(p SplitProps) templ.Component {
	return el("div", "ui-split", withAttrs(p.Attrs),
		el("div", "ui-split-list", "", p.List),
		el("div", "ui-split-detail", "", p.Detail))
}

func (noopKit) SidebarNav(p SidebarNavProps) templ.Component {
	secs := make([]templ.Component, 0, len(p.Sections))
	for _, s := range p.Sections {
		items := make([]templ.Component, 0, len(s.Items)+1)
		if s.Label != "" {
			items = append(items, el("p", "ui-side-label", "", txt(s.Label)))
		}
		for _, it := range s.Items {
			items = append(items, navLink(it))
		}
		secs = append(secs, el("div", "ui-side-section", "", items...))
	}
	return el("nav", "ui-sidebar", ` aria-label="Sidebar"`+withAttrs(p.Attrs), secs...)
}

func (k noopKit) ButtonGroup(p ButtonGroupProps, inner ...templ.Component) templ.Component {
	a := ` role="group"`
	if p.Label != "" {
		a += ` aria-label="` + html.EscapeString(p.Label) + `"`
	}
	return el("div", "ui-btngroup", a+withAttrs(p.Attrs), inner...)
}

func (noopKit) MenuButton(p MenuButtonProps) templ.Component {
	items := make([]templ.Component, 0, len(p.Items))
	for _, mi := range p.Items {
		items = append(items, el("li", "ui-menu-item", "", txt(mi.Label)))
	}
	return el("div", "ui-menubtn", withAttrs(p.Attrs),
		el("button", "ui-btn", ` type="button"`, txt(p.Label)),
		el("ul", "ui-menu", "", items...))
}

func (k noopKit) Form(p FormProps, inner ...templ.Component) templ.Component {
	return el("form", "ui-form", withAttrs(p.Attrs), inner...)
}

func (noopKit) Checkbox(p CheckboxProps) templ.Component {
	a := ` type="checkbox"`
	if p.Name != "" {
		a += ` name="` + html.EscapeString(p.Name) + `"`
	}
	if p.Checked {
		a += " checked"
	}
	box := el("input", "ui-checkbox", a)
	return el("label", "ui-check", "", box, txt(p.Label))
}

func (noopKit) RadioGroup(p RadioGroupProps) templ.Component {
	opts := make([]templ.Component, 0, len(p.Options))
	for _, o := range p.Options {
		a := ` type="radio" name="` + html.EscapeString(p.Name) + `" value="` + html.EscapeString(o.Value) + `"`
		if o.Checked {
			a += " checked"
		}
		opts = append(opts, el("label", "ui-radio", "", el("input", "", a), txt(o.Label)))
	}
	return el("div", "ui-radiogroup", withAttrs(p.Attrs), opts...)
}

func (noopKit) Toggle(p ToggleProps) templ.Component {
	a := ` type="checkbox" role="switch"`
	if p.Name != "" {
		a += ` name="` + html.EscapeString(p.Name) + `"`
	}
	if p.Checked {
		a += " checked"
	}
	return el("label", "ui-toggle", withAttrs(p.Attrs),
		el("input", "ui-toggle-input", a), txt(p.Label))
}

func (k noopKit) InputGroup(p InputGroupProps) templ.Component {
	return el("div", "ui-inputgroup", withAttrs(p.Attrs),
		k.TextInput(p.Input), k.Button(p.Button))
}

func (noopKit) FormError(p FormErrorProps) templ.Component {
	body := []templ.Component{}
	if p.Title != "" {
		body = append(body, el("strong", "ui-formerror-title", "", txt(p.Title)))
	}
	for _, m := range p.Messages {
		body = append(body, el("li", "", "", txt(m)))
	}
	return el("div", "ui-formerror", ` role="alert"`+withAttrs(p.Attrs), body...)
}

func (noopKit) DescList(p DescListProps) templ.Component {
	body := make([]templ.Component, 0, len(p.Items)*2)
	for _, it := range p.Items {
		body = append(body, el("dt", "ui-dt", "", txt(it.Term)))
		body = append(body, el("dd", "ui-dd", "", txt(it.Value)))
	}
	return el("dl", "ui-desclist", withAttrs(p.Attrs), body...)
}

func (k noopKit) StackedList(p StackedListProps) templ.Component {
	rows := make([]templ.Component, 0, len(p.Rows))
	for _, r := range p.Rows {
		body := []templ.Component{}
		if r.Avatar != nil {
			body = append(body, r.Avatar)
		}
		body = append(body, el("div", "ui-row-text", "",
			el("p", "ui-row-title", "", txt(r.Title)),
			el("p", "ui-row-meta", "", txt(r.Meta))))
		rows = append(rows, el("li", "ui-row", "", body...))
	}
	return el("ul", "ui-stacked", withAttrs(p.Attrs), rows...)
}

func (noopKit) GridList(p GridListProps, inner ...templ.Component) templ.Component {
	return el("div", "ui-gridlist", withAttrs(p.Attrs), inner...)
}

func (noopKit) Toast(p ToastProps) templ.Component {
	cls := "ui-toast"
	if p.Role != "" {
		cls += " ui-toast-" + string(p.Role)
	}
	body := []templ.Component{}
	if p.Title != "" {
		body = append(body, el("strong", "ui-toast-title", "", txt(p.Title)))
	}
	body = append(body, el("p", "ui-toast-msg", "", txt(p.Message)))
	return el("div", cls, ` role="status"`+withAttrs(p.Attrs), body...)
}

func (noopKit) Progress(p ProgressProps) templ.Component {
	a := withAttrs(p.Attrs)
	if p.Label != "" {
		a = ` aria-label="` + html.EscapeString(p.Label) + `"` + a
	}
	cls := "ui-progress"
	if p.Max <= 0 {
		cls += " is-indeterminate"
	}
	return el("div", cls, ` role="progressbar"`+a,
		el("div", "ui-progress-bar", ""))
}

func (k noopKit) Drawer(p DrawerProps, inner ...templ.Component) templ.Component {
	cls := "ui-drawer"
	if p.Side != "" {
		cls += " ui-drawer-" + html.EscapeString(p.Side)
	}
	body := []templ.Component{}
	if p.Title != "" {
		body = append(body, el("header", "ui-drawer-head", "", txt(p.Title)))
	}
	body = append(body, inner...)
	a := ` id="` + html.EscapeString(p.ID) + `"` + withAttrs(p.Attrs)
	return el("dialog", cls, a, body...)
}

func (noopKit) Popover(p PopoverProps, inner ...templ.Component) templ.Component {
	return el("span", "ui-popover", withAttrs(p.Attrs),
		el("button", "ui-btn", ` type="button"`, txt(p.Label)),
		el("div", "ui-popover-panel", "", inner...))
}

func (k noopKit) Figure(p FigureProps) templ.Component {
	body := []templ.Component{k.Image(p.Image)}
	if p.Caption != "" {
		body = append(body, el("figcaption", "ui-figcaption", "", txt(p.Caption)))
	}
	return el("figure", "ui-figure", withAttrs(p.Attrs), body...)
}
