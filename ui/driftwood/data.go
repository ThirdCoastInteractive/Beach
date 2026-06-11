package driftwood

import (
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// --- Data --------------------------------------------------------------------

func (Kit) Table(p ui.TableProps) templ.Component {
	if len(p.Rows) == 0 && p.Empty != "" {
		return el("div", "dw-table-empty", attrs(p.Attrs),
			el("p", "dw-empty-msg", "", txt(p.Empty)))
	}
	head := make([]templ.Component, 0, len(p.Columns))
	for _, c := range p.Columns {
		if c.Get != "" {
			// Sortable header: a hypermedia @get, button-inside-th for a11y.
			sort := el("button", "dw-th-sort",
				` type="button" `+datastar.On("click", "@get('"+c.Get+"')").String(),
				txt(c.Header), ui.I("chevron-up-down"))
			head = append(head, el("th", "dw-th is-sortable", ` scope="col"`, sort))
			continue
		}
		head = append(head, el("th", "dw-th", ` scope="col"`, txt(c.Header)))
	}
	body := make([]templ.Component, 0, len(p.Rows))
	for _, r := range p.Rows {
		cells := make([]templ.Component, 0, len(r))
		for _, cell := range r {
			cells = append(cells, el("td", "dw-td", "", txt(cell)))
		}
		body = append(body, el("tr", "dw-tr", "", cells...))
	}
	cls := "dw-table"
	if p.Dense {
		cls += " is-dense"
	}
	return el("table", cls, attrs(p.Attrs),
		el("thead", "dw-thead", "", el("tr", "dw-tr", "", head...)),
		el("tbody", "dw-tbody", "", body...))
}

func (Kit) Stat(p ui.StatProps) templ.Component {
	body := []templ.Component{
		el("dt", "dw-stat-label", "", txt(p.Label)),
		el("dd", "dw-stat-value", "", txt(p.Value)),
	}
	if p.Delta.Value != "" {
		// Color is never the only encoding: pair direction with an arrow glyph
		// and a screen-reader label.
		glyph, label := "arrow-right", "no change"
		switch p.Delta.Dir {
		case "up":
			glyph, label = "arrow-up", "up"
		case "down":
			glyph, label = "arrow-down", "down"
		}
		body = append(body, el("dd", "dw-stat-delta dw-delta-"+esc(p.Delta.Dir), "",
			ui.I(glyph, ui.IconLabel(label)),
			el("span", "dw-delta-value", "", txt(p.Delta.Value))))
	}
	return el("dl", "dw-stat", attrs(p.Attrs), body...)
}

func (Kit) Badge(p ui.BadgeProps) templ.Component {
	cls := "dw-badge" + roleClass("dw-badge", p.Role)
	body := make([]templ.Component, 0, 2)
	if p.Dot {
		body = append(body, el("span", "dw-badge-dot", ` aria-hidden="true"`))
	}
	body = append(body, txt(p.Label))
	return el("span", cls, attrs(p.Attrs), body...)
}

// DescList is a key/value grid. TwoCol pairs term and value on one row;
// otherwise term-over-value stacks.
func (Kit) DescList(p ui.DescListProps) templ.Component {
	cls := "dw-desclist"
	if p.TwoCol {
		cls += " is-twocol"
	}
	body := make([]templ.Component, 0, len(p.Items)*2)
	for _, it := range p.Items {
		body = append(body, el("dt", "dw-dt", "", txt(it.Term)))
		body = append(body, el("dd", "dw-dd", "", txt(it.Value)))
	}
	return el("dl", cls, attrs(p.Attrs), body...)
}

// StackedList is a vertical list of rows (feeds, item lists): an optional leading
// avatar, a title and meta line, and an optional trailing action or badge.
func (k Kit) StackedList(p ui.StackedListProps) templ.Component {
	rows := make([]templ.Component, 0, len(p.Rows))
	for _, r := range p.Rows {
		main := make([]templ.Component, 0, 3)
		if r.Avatar != nil {
			main = append(main, el("div", "dw-row-media", "", r.Avatar))
		}
		text := make([]templ.Component, 0, 2)
		if r.Href != "" {
			text = append(text, el("a", "dw-row-title", ` href="`+esc(r.Href)+`"`, txt(r.Title)))
		} else {
			text = append(text, el("p", "dw-row-title", "", txt(r.Title)))
		}
		if r.Meta != "" {
			text = append(text, el("p", "dw-row-meta", "", txt(r.Meta)))
		}
		main = append(main, el("div", "dw-row-text", "", text...))
		trailing := make([]templ.Component, 0, 2)
		if r.Badge != nil {
			trailing = append(trailing, k.Badge(*r.Badge))
		}
		if r.Action != nil {
			trailing = append(trailing, r.Action)
		}
		if len(trailing) > 0 {
			main = append(main, el("div", "dw-row-action", "", trailing...))
		}
		rows = append(rows, el("li", "dw-row", "", main...))
	}
	return el("ul", "dw-stacked", attrs(p.Attrs), rows...)
}

// GridList is a responsive card grid (property tiles, item cards). Column count
// rides on a CSS custom property the kit sheet clamps responsively.
func (Kit) GridList(p ui.GridListProps, inner ...templ.Component) templ.Component {
	cols := p.Cols
	if cols < 1 {
		cols = 3
	}
	style := ` style="--dw-grid-cols:` + itoa(cols)
	if p.Gap != "" {
		style += `;--dw-grid-gap:` + esc(p.Gap)
	}
	style += `"`
	return el("ul", "dw-gridlist", style+attrs(p.Attrs), inner...)
}
