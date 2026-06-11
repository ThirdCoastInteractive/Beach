package driftwood

import (
	"strconv"

	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

func itoa(n int) string { return strconv.Itoa(n) }

// --- Layout ------------------------------------------------------------------

// AppShell is the top-level page frame: an optional topbar, an optional fixed
// left sidebar, and the main content region. With no sidebar it is the default
// stacked shell; with one it is the sidebar shell — both server-rendered, no JS.
func (Kit) AppShell(p ui.AppShellProps, inner ...templ.Component) templ.Component {
	body := make([]templ.Component, 0, 2)
	if p.Topbar != nil {
		body = append(body, el("header", "dw-shell-top", "", p.Topbar))
	}
	main := el("main", "dw-shell-main", "", inner...)
	if p.Sidebar != nil {
		body = append(body, el("div", "dw-shell-cols", "",
			el("aside", "dw-shell-side", "", p.Sidebar), main))
	} else {
		body = append(body, main)
	}
	cls := "dw-shell"
	if p.Sidebar != nil {
		cls += " has-sidebar"
	}
	return el("div", cls, attrs(p.Attrs), body...)
}

func (Kit) Container(p ui.ContainerProps, inner ...templ.Component) templ.Component {
	attr := attrs(p.Attrs)
	if p.MaxWidth != "" {
		attr = ` style="max-width:var(--size-` + esc(p.MaxWidth) + `)"` + attr
	}
	return el("div", "dw-container", attr, inner...)
}

func (Kit) Card(p ui.CardProps, inner ...templ.Component) templ.Component {
	class := "dw-card"
	switch p.Surface {
	case ui.SurfaceWell:
		class += " dw-surface-well"
	case ui.SurfacePanel:
		class += " dw-surface-panel"
	}
	body := make([]templ.Component, 0, len(inner)+1)
	if p.Heading != "" {
		body = append(body, el("header", "dw-card-head", "",
			el("h3", "dw-card-title", "", txt(p.Heading))))
	}
	body = append(body, el("div", "dw-card-body", "", inner...))
	return el("section", class, attrs(p.Attrs), body...)
}

func (Kit) PageHeading(p ui.PageHeadingProps) templ.Component {
	inner := []templ.Component{el("h1", "dw-page-title", "", txt(p.Title))}
	if p.Subtitle != "" {
		inner = append(inner, el("p", "dw-page-subtitle", "", txt(p.Subtitle)))
	}
	return el("header", "dw-page-heading", attrs(p.Attrs), inner...)
}

func (Kit) SectionHeading(p ui.SectionHeadingProps) templ.Component {
	return el("h2", "dw-section-heading", attrs(p.Attrs), txt(p.Label))
}

func (Kit) Divider(p ui.DividerProps) templ.Component {
	if p.Label != "" {
		return el("div", "dw-divider dw-divider-labeled", ` role="separator"`+attrs(p.Attrs),
			el("span", "dw-divider-label", "", txt(p.Label)))
	}
	return raw(`<hr class="dw-divider"` + attrs(p.Attrs) + `>`)
}

// CardHeading is the richer card header: a title and optional meta line on the
// left, an optional action slot on the right (Card's Heading string is the
// simple case; drop one of these into a Card's body for the full header).
func (Kit) CardHeading(p ui.CardHeadingProps) templ.Component {
	left := make([]templ.Component, 0, 2)
	left = append(left, el("h3", "dw-card-title", "", txt(p.Title)))
	if p.Meta != "" {
		left = append(left, el("p", "dw-card-meta", "", txt(p.Meta)))
	}
	body := []templ.Component{el("div", "dw-card-head-text", "", left...)}
	if p.Action != nil {
		body = append(body, el("div", "dw-card-head-action", "", p.Action))
	}
	return el("header", "dw-card-head", attrs(p.Attrs), body...)
}

// Grid is a responsive dashboard widget grid. The column count rides on a CSS
// custom property the kit sheet clamps responsively (no media queries in Go).
func (Kit) Grid(p ui.GridProps, inner ...templ.Component) templ.Component {
	cols := p.Cols
	if cols < 1 {
		cols = 3
	}
	style := ` style="--dw-grid-cols:` + itoa(cols)
	if p.Gap != "" {
		style += `;--dw-grid-gap:` + esc(p.Gap)
	}
	style += `"`
	return el("div", "dw-grid", style+attrs(p.Attrs), inner...)
}

// Split is a two-pane master-detail layout: a list pane and a detail pane.
func (Kit) Split(p ui.SplitProps) templ.Component {
	return el("div", "dw-split", attrs(p.Attrs),
		el("div", "dw-split-list", "", p.List),
		el("div", "dw-split-detail", "", p.Detail))
}

// --- Nav ---------------------------------------------------------------------

func (Kit) Navbar(p ui.NavbarProps) templ.Component {
	items := make([]templ.Component, 0, len(p.Items)+1)
	if p.Brand != "" {
		items = append(items, el("a", "dw-brand", ` href="/"`, txt(p.Brand)))
	}
	links := make([]templ.Component, 0, len(p.Items))
	for _, it := range p.Items {
		links = append(links, navLink(it))
	}
	items = append(items, el("div", "dw-navlinks", "", links...))
	return el("nav", "dw-navbar", ` aria-label="Primary"`+attrs(p.Attrs), items...)
}

// SidebarNav is the fixed left-rail navigation: labeled sections of items with
// icons and active state. Active state is a class, never script.
func (Kit) SidebarNav(p ui.SidebarNavProps) templ.Component {
	secs := make([]templ.Component, 0, len(p.Sections))
	for _, s := range p.Sections {
		body := make([]templ.Component, 0, len(s.Items)+1)
		if s.Label != "" {
			body = append(body, el("p", "dw-side-label", "", txt(s.Label)))
		}
		links := make([]templ.Component, 0, len(s.Items))
		for _, it := range s.Items {
			links = append(links, el("li", "dw-side-item", "", sideLink(it)))
		}
		body = append(body, el("ul", "dw-side-items", "", links...))
		secs = append(secs, el("div", "dw-side-section", "", body...))
	}
	return el("nav", "dw-sidebar", ` aria-label="Sidebar"`+attrs(p.Attrs), secs...)
}

func sideLink(it ui.NavItem) templ.Component {
	cls := "dw-side-link"
	if it.Active {
		cls += " is-active"
	}
	a := ` href="` + esc(it.Href) + `"`
	if it.Active {
		a += ` aria-current="page"`
	}
	inner := make([]templ.Component, 0, 2)
	if it.Icon != "" {
		inner = append(inner, ui.I(it.Icon))
	}
	inner = append(inner, el("span", "dw-side-text", "", txt(it.Label)))
	return el("a", cls, a, inner...)
}

func (Kit) Breadcrumbs(p ui.BreadcrumbsProps) templ.Component {
	crumbs := make([]templ.Component, 0, len(p.Items))
	for i, it := range p.Items {
		last := i == len(p.Items)-1
		if last {
			crumbs = append(crumbs, el("li", "dw-crumb is-current", ` aria-current="page"`, txt(it.Label)))
			continue
		}
		link := el("a", "dw-crumb-link", ` href="`+esc(it.Href)+`"`, txt(it.Label))
		crumbs = append(crumbs, el("li", "dw-crumb", "", link))
	}
	return el("nav", "dw-breadcrumbs", ` aria-label="Breadcrumb"`+attrs(p.Attrs),
		el("ol", "dw-crumbs", "", crumbs...))
}

// Tabs is pure CSS: a radio per tab drives :checked panel visibility. Datastar
// enters only when a tab's panel is deferred (Tab.Get set), wiring an @get.
func (Kit) Tabs(p ui.TabsProps) templ.Component {
	group := p.Name
	if group == "" {
		group = "dw-tabs"
	}
	radios := make([]templ.Component, 0, len(p.Tabs))
	labels := make([]templ.Component, 0, len(p.Tabs))
	for _, t := range p.Tabs {
		id := group + "-" + esc(t.ID)
		ra := ` type="radio" name="` + esc(group) + `" id="` + id + `" class="dw-tab-radio"`
		if t.Active {
			ra += " checked"
		}
		if t.Get != "" {
			ra += " " + datastar.On("change", "@get('"+t.Get+"')").String()
		}
		radios = append(radios, raw(`<input`+ra+`>`))
		labels = append(labels, el("label", "dw-tab", ` for="`+id+`"`, txt(t.Label)))
	}
	body := append(radios, el("div", "dw-tablist", ` role="tablist"`, labels...))
	return el("div", "dw-tabs", attrs(p.Attrs), body...)
}

func (Kit) Pagination(p ui.PaginationProps) templ.Component {
	if p.Pages <= 1 {
		return el("nav", "dw-pagination", ` aria-label="Pagination"`+attrs(p.Attrs))
	}
	links := make([]templ.Component, 0, p.Pages+2)
	links = append(links, pageLink("Prev", p.GetBase, p.Page-1, p.Page <= 1, false))
	for n := 1; n <= p.Pages; n++ {
		links = append(links, pageLink(itoa(n), p.GetBase, n, false, n == p.Page))
	}
	links = append(links, pageLink("Next", p.GetBase, p.Page+1, p.Page >= p.Pages, false))
	return el("nav", "dw-pagination", ` aria-label="Pagination"`+attrs(p.Attrs), links...)
}

func pageLink(label, base string, n int, disabled, current bool) templ.Component {
	cls := "dw-page-link"
	if current {
		cls += " is-current"
	}
	if disabled {
		return el("span", cls+" is-disabled", ` aria-disabled="true"`, txt(label))
	}
	get := datastar.On("click", "@get('"+base+"?page="+itoa(n)+"')").String()
	a := ` href="` + esc(base) + `?page=` + itoa(n) + `" ` + get
	if current {
		a += ` aria-current="page"`
	}
	return el("a", cls, a, txt(label))
}

func navLink(it ui.NavItem) templ.Component {
	cls := "dw-navitem"
	if it.Active {
		cls += " is-active"
	}
	a := ` href="` + esc(it.Href) + `"`
	if it.Active {
		a += ` aria-current="page"`
	}
	inner := []templ.Component{}
	if it.Icon != "" {
		inner = append(inner, ui.I(it.Icon))
	}
	inner = append(inner, txt(it.Label))
	return el("a", cls, a, inner...)
}
