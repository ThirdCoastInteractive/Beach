package ui

import (
	"context"
	"html"
	"io"

	"github.com/a-h/templ"
)

// noopKit is a tiny reference Kit: it satisfies the full interface with the
// minimum markup needed to prove the toolkit compiles and renders. It is NOT a
// real theme — ui/driftwood is the reference kit, and apps bring their own. The
// no-op exists so ui builds and tests standalone (and so a missing kit in a test
// has something to fall back to). Its markup still references Stellar tokens and
// never a hardcoded color.
type noopKit struct{}

// NoopKit returns the built-in no-op kit. Useful in tests and as a last-resort
// fallback; real apps register a themed kit.
func NoopKit() Kit { return noopKit{} }

func (noopKit) Name() string           { return "noop" }
func (k noopKit) Layout() Layout       { return k }
func (k noopKit) Nav() Nav             { return k }
func (k noopKit) Buttons() Buttons     { return k }
func (k noopKit) Forms() Forms         { return k }
func (k noopKit) Data() Data           { return k }
func (k noopKit) Feedback() Feedback   { return k }
func (k noopKit) Overlays() Overlays   { return k }
func (k noopKit) Media() Media         { return k }
func (k noopKit) Messaging() Messaging { return k }

// el is a small helper: render an element with class, optional attrs string,
// and inner components.
func el(tag, class, attrs string, inner ...templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		open := "<" + tag + ` class="` + class + `"` + attrs + ">"
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

// txt renders escaped text content.
func txt(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, html.EscapeString(s))
		return err
	})
}

// --- Layout ---

func (noopKit) Container(p ContainerProps, inner ...templ.Component) templ.Component {
	return el("div", "ui-container", withAttrs(p.Attrs), inner...)
}
func (noopKit) Card(p CardProps, inner ...templ.Component) templ.Component {
	class := "ui-card"
	if p.Surface != "" {
		class += " ui-" + string(p.Surface)
	}
	body := inner
	if p.Heading != "" {
		body = append([]templ.Component{el("header", "ui-card-head", "", txt(p.Heading))}, inner...)
	}
	return el("section", class, withAttrs(p.Attrs), body...)
}
func (noopKit) PageHeading(p PageHeadingProps) templ.Component {
	inner := []templ.Component{el("h1", "ui-title", "", txt(p.Title))}
	if p.Subtitle != "" {
		inner = append(inner, el("p", "ui-subtitle", "", txt(p.Subtitle)))
	}
	return el("header", "ui-page-heading", withAttrs(p.Attrs), inner...)
}
func (noopKit) SectionHeading(p SectionHeadingProps) templ.Component {
	return el("h2", "ui-section-heading", withAttrs(p.Attrs), txt(p.Label))
}
func (noopKit) Divider(p DividerProps) templ.Component {
	if p.Label != "" {
		return el("div", "ui-divider", withAttrs(p.Attrs), txt(p.Label))
	}
	return el("hr", "ui-divider", withAttrs(p.Attrs))
}

// --- Nav ---

func (noopKit) Navbar(p NavbarProps) templ.Component {
	items := make([]templ.Component, 0, len(p.Items)+1)
	if p.Brand != "" {
		items = append(items, el("span", "ui-brand", "", txt(p.Brand)))
	}
	for _, it := range p.Items {
		items = append(items, navLink(it))
	}
	return el("nav", "ui-navbar", withAttrs(p.Attrs), items...)
}
func (noopKit) Breadcrumbs(p BreadcrumbsProps) templ.Component {
	items := make([]templ.Component, 0, len(p.Items))
	for _, it := range p.Items {
		items = append(items, navLink(it))
	}
	return el("nav", "ui-breadcrumbs", withAttrs(p.Attrs), items...)
}
func (noopKit) Tabs(p TabsProps) templ.Component {
	tabs := make([]templ.Component, 0, len(p.Tabs))
	for _, t := range p.Tabs {
		cls := "ui-tab"
		if t.Active {
			cls += " is-active"
		}
		tabs = append(tabs, el("button", cls, "", txt(t.Label)))
	}
	return el("div", "ui-tabs", withAttrs(p.Attrs), tabs...)
}
func (noopKit) Pagination(p PaginationProps) templ.Component {
	return el("nav", "ui-pagination", withAttrs(p.Attrs))
}

func navLink(it NavItem) templ.Component {
	cls := "ui-navitem"
	if it.Active {
		cls += " is-active"
	}
	attrs := ` href="` + html.EscapeString(it.Href) + `"`
	return el("a", cls, attrs, txt(it.Label))
}

// --- Buttons ---

func (noopKit) Button(p ButtonProps, inner ...templ.Component) templ.Component {
	cls := "ui-btn"
	if p.Role != "" {
		cls += " ui-btn-" + string(p.Role)
	}
	if p.Size != "" {
		cls += " ui-btn-" + string(p.Size)
	}
	attrs := withAttrs(p.Attrs)
	typ := p.Type
	if typ == "" {
		typ = "button"
	}
	attrs = ` type="` + typ + `"` + attrs
	if p.Disabled {
		attrs += " disabled"
	}
	body := []templ.Component{}
	if p.LeadIcon != "" {
		body = append(body, I(p.LeadIcon))
	}
	if p.Label != "" {
		body = append(body, txt(p.Label))
	}
	body = append(body, inner...)
	if p.TailIcon != "" {
		body = append(body, I(p.TailIcon))
	}
	return el("button", cls, attrs, body...)
}
func (noopKit) IconButton(p IconButtonProps) templ.Component {
	cls := "ui-iconbtn"
	if p.Role != "" {
		cls += " ui-btn-" + string(p.Role)
	}
	attrs := ` type="button" aria-label="` + html.EscapeString(p.Label) + `"` + withAttrs(p.Attrs)
	return el("button", cls, attrs, I(p.Icon))
}

// --- Forms ---

func (noopKit) Field(p FieldProps, inner ...templ.Component) templ.Component {
	body := []templ.Component{}
	if p.Label != "" {
		la := ""
		if p.For != "" {
			la = ` for="` + html.EscapeString(p.For) + `"`
		}
		body = append(body, el("label", "ui-label", la, txt(p.Label)))
	}
	body = append(body, inner...)
	if p.Help != "" {
		body = append(body, el("small", "ui-help", "", txt(p.Help)))
	}
	if p.Error != "" {
		body = append(body, el("small", "ui-error", "", txt(p.Error)))
	}
	return el("div", "ui-field", withAttrs(p.Attrs), body...)
}
func (noopKit) TextInput(p TextInputProps) templ.Component {
	typ := p.Type
	if typ == "" {
		typ = "text"
	}
	cls := "ui-input"
	if p.Error {
		cls += " is-error"
	}
	attrs := ` type="` + typ + `"` +
		` id="` + html.EscapeString(p.ID) + `"` +
		` name="` + html.EscapeString(p.Name) + `"` +
		` value="` + html.EscapeString(p.Value) + `"`
	if p.Placeholder != "" {
		attrs += ` placeholder="` + html.EscapeString(p.Placeholder) + `"`
	}
	if p.Disabled {
		attrs += " disabled"
	}
	attrs += withAttrs(p.Attrs)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<input class="`+cls+`"`+attrs+`>`)
		return err
	})
}
func (noopKit) Textarea(p TextareaProps) templ.Component {
	cls := "ui-textarea"
	if p.Error {
		cls += " is-error"
	}
	attrs := ` id="` + html.EscapeString(p.ID) + `" name="` + html.EscapeString(p.Name) + `"` + withAttrs(p.Attrs)
	return el("textarea", cls, attrs, txt(p.Value))
}
func (noopKit) Select(p SelectProps) templ.Component {
	opts := make([]templ.Component, 0, len(p.Options)+1)
	if p.Placeholder != "" {
		opts = append(opts, el("option", "", ` value=""`, txt(p.Placeholder)))
	}
	for _, o := range p.Options {
		a := ` value="` + html.EscapeString(o.Value) + `"`
		if o.Selected {
			a += " selected"
		}
		opts = append(opts, el("option", "", a, txt(o.Label)))
	}
	cls := "ui-select"
	if p.Error {
		cls += " is-error"
	}
	attrs := ` id="` + html.EscapeString(p.ID) + `" name="` + html.EscapeString(p.Name) + `"` + withAttrs(p.Attrs)
	return el("select", cls, attrs, opts...)
}

// --- Data ---

func (noopKit) Table(p TableProps) templ.Component {
	if len(p.Rows) == 0 && p.Empty != "" {
		return el("div", "ui-table-empty", withAttrs(p.Attrs), txt(p.Empty))
	}
	head := make([]templ.Component, 0, len(p.Columns))
	for _, c := range p.Columns {
		head = append(head, el("th", "", "", txt(c.Header)))
	}
	rows := make([]templ.Component, 0, len(p.Rows)+1)
	rows = append(rows, el("tr", "", "", head...))
	for _, r := range p.Rows {
		cells := make([]templ.Component, 0, len(r))
		for _, cell := range r {
			cells = append(cells, el("td", "", "", txt(cell)))
		}
		rows = append(rows, el("tr", "", "", cells...))
	}
	cls := "ui-table"
	if p.Dense {
		cls += " is-dense"
	}
	return el("table", cls, withAttrs(p.Attrs), rows...)
}
func (noopKit) Stat(p StatProps) templ.Component {
	body := []templ.Component{
		el("div", "ui-stat-value", "", txt(p.Value)),
		el("div", "ui-stat-label", "", txt(p.Label)),
	}
	if p.Delta.Value != "" {
		// Color is never the only encoding: pair the direction with a glyph.
		glyph := "minus"
		switch p.Delta.Dir {
		case "up":
			glyph = "arrow-up"
		case "down":
			glyph = "arrow-down"
		}
		body = append(body, el("div", "ui-stat-delta ui-delta-"+html.EscapeString(p.Delta.Dir), "", I(glyph), txt(p.Delta.Value)))
	}
	return el("div", "ui-stat", withAttrs(p.Attrs), body...)
}
func (noopKit) Badge(p BadgeProps) templ.Component {
	cls := "ui-badge"
	if p.Role != "" {
		cls += " ui-badge-" + string(p.Role)
	}
	body := []templ.Component{}
	if p.Dot {
		body = append(body, el("span", "ui-badge-dot", ""))
	}
	body = append(body, txt(p.Label))
	return el("span", cls, withAttrs(p.Attrs), body...)
}

// --- Feedback ---

func (noopKit) Alert(p AlertProps, inner ...templ.Component) templ.Component {
	cls := "ui-alert"
	if p.Role != "" {
		cls += " ui-alert-" + string(p.Role)
	}
	body := []templ.Component{}
	if p.Title != "" {
		body = append(body, el("strong", "ui-alert-title", "", txt(p.Title)))
	}
	body = append(body, inner...)
	return el("div", cls, ` role="alert"`+withAttrs(p.Attrs), body...)
}
func (noopKit) EmptyState(p EmptyStateProps) templ.Component {
	body := []templ.Component{}
	if p.Icon != "" {
		body = append(body, I(p.Icon))
	}
	body = append(body, el("p", "ui-empty-msg", "", txt(p.Message)))
	return el("div", "ui-empty", withAttrs(p.Attrs), body...)
}
func (noopKit) Spinner(p SpinnerProps) templ.Component {
	cls := "ui-spinner"
	if p.Block {
		cls += " is-block"
	}
	if p.Size != "" {
		cls += " ui-" + string(p.Size)
	}
	return el("span", cls, ` role="status" aria-label="Loading"`+withAttrs(p.Attrs))
}
func (noopKit) Skeleton(p SkeletonProps) templ.Component {
	style := ""
	if p.Width != "" {
		style += "width:" + html.EscapeString(p.Width) + ";"
	}
	if p.Height != "" {
		style += "height:" + html.EscapeString(p.Height) + ";"
	}
	attrs := ` aria-hidden="true"`
	if style != "" {
		attrs += ` style="` + style + `"`
	}
	return el("div", "ui-skeleton", attrs+withAttrs(p.Attrs))
}

// --- Overlays ---

func (noopKit) Modal(p ModalProps, inner ...templ.Component) templ.Component {
	body := []templ.Component{}
	if p.Title != "" {
		body = append(body, el("header", "ui-modal-head", "", txt(p.Title)))
	}
	body = append(body, inner...)
	attrs := ` id="` + html.EscapeString(p.ID) + `"` + withAttrs(p.Attrs)
	return el("dialog", "ui-modal", attrs, body...)
}
func (noopKit) Tooltip(p TooltipProps, inner ...templ.Component) templ.Component {
	body := append([]templ.Component{}, inner...)
	body = append(body, el("span", "ui-tooltip-text", ` role="tooltip"`, txt(p.Text)))
	return el("span", "ui-tooltip", withAttrs(p.Attrs), body...)
}

// --- Media ---

func (noopKit) Avatar(p AvatarProps) templ.Component {
	if p.Src != "" {
		attrs := ` src="` + html.EscapeString(p.Src) + `" alt="` + html.EscapeString(p.Alt) + `"`
		return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			_, err := io.WriteString(w, `<img class="ui-avatar"`+attrs+withAttrs(p.Attrs)+`>`)
			return err
		})
	}
	return el("span", "ui-avatar ui-avatar-initials", withAttrs(p.Attrs), txt(p.Initials))
}
func (noopKit) Image(p ImageProps) templ.Component {
	// Sits in an aspect box so late bytes fill reserved pixels (no pop-in).
	loading := "lazy"
	if p.Eager {
		loading = "eager"
	}
	attrs := ` src="` + html.EscapeString(p.Src) + `" alt="` + html.EscapeString(p.Alt) + `"` +
		` loading="` + loading + `" decoding="async"`
	if p.Set != "" {
		attrs += ` srcset="` + html.EscapeString(p.Set) + `"`
	}
	if p.Sizes != "" {
		attrs += ` sizes="` + html.EscapeString(p.Sizes) + `"`
	}
	if p.Eager {
		attrs += ` fetchpriority="high"`
	}
	img := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<img class="ui-img ui-fit-`+string(p.Fit)+`"`+attrs+`>`)
		return err
	})
	return noopKit{}.AspectBox(AspectBoxProps{Ratio: p.Ratio, Attrs: p.Attrs}, img)
}
func (noopKit) AspectBox(p AspectBoxProps, inner ...templ.Component) templ.Component {
	cls := "ui-aspect"
	if p.Ratio != "" {
		cls += " ui-aspect-" + string(p.Ratio)
	}
	return el("div", cls, withAttrs(p.Attrs), inner...)
}

// --- Messaging ---

func (noopKit) MessageList(p MessageListProps) templ.Component {
	rows := make([]templ.Component, 0, len(p.Messages))
	for _, m := range p.Messages {
		cls := "ui-msg"
		switch {
		case m.System:
			cls += " is-system"
		case m.Own:
			cls += " is-own"
		}
		rows = append(rows, el("li", cls, "", txt(m.Body)))
	}
	return el("ul", "ui-msglist", withAttrs(p.Attrs), rows...)
}
func (noopKit) Composer(p ComposerProps) templ.Component {
	ta := el("textarea", "ui-composer-input", ` name="`+html.EscapeString(p.Name)+`" placeholder="`+html.EscapeString(p.Placeholder)+`"`)
	send := el("button", "ui-composer-send", ` type="submit"`, txt("Send"))
	return el("div", "ui-composer", withAttrs(p.Attrs), ta, send)
}
func (noopKit) PresencePill(p PresencePillProps) templ.Component {
	cls := "ui-presence"
	if p.State != "" {
		cls += " is-" + html.EscapeString(p.State)
	}
	return el("span", cls, withAttrs(p.Attrs), txt(p.Label))
}
