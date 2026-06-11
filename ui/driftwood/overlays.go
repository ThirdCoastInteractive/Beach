package driftwood

import (
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// --- Overlays ----------------------------------------------------------------

// Modal is a native <dialog>: open/close is the platform's, never script.
// Datastar enters only to @get a heavy body under the 14KB budget; stacking
// sits on var(--zindex-dialog) in the kit CSS.
func (Kit) Modal(p ui.ModalProps, inner ...templ.Component) templ.Component {
	head := make([]templ.Component, 0, 2)
	if p.Title != "" {
		head = append(head, el("h2", "dw-modal-title", "", txt(p.Title)))
	}
	// The close control uses the dialog's own form method=dialog — no script.
	head = append(head, el("form", "dw-modal-x", ` method="dialog"`,
		el("button", "dw-iconbtn", ` type="submit" aria-label="Close"`, ui.I("x"))))

	bodyAttr := ""
	if p.Get != "" {
		// Lazy-load the heavy body once on open.
		bodyAttr = " " + datastar.Init("@get('"+p.Get+"')").String()
	}
	body := el("div", "dw-modal-body", bodyAttr, inner...)

	a := ""
	if p.ID != "" {
		a += ` id="` + esc(p.ID) + `"`
	}
	a += attrs(p.Attrs)
	return el("dialog", "dw-modal", a,
		el("header", "dw-modal-head", "", head...),
		body)
}

// Drawer is a slide-over panel built on the native <dialog> (same open/close
// mechanism as Modal: showModal + method="dialog" close, no script). Side picks
// the edge; the kit CSS slides it in. Get lazy-loads a heavy body under budget.
func (Kit) Drawer(p ui.DrawerProps, inner ...templ.Component) templ.Component {
	side := p.Side
	if side != "left" {
		side = "right"
	}
	cls := "dw-drawer dw-drawer-" + side
	head := make([]templ.Component, 0, 2)
	if p.Title != "" {
		head = append(head, el("h2", "dw-drawer-title", "", txt(p.Title)))
	}
	head = append(head, el("form", "dw-drawer-x", ` method="dialog"`,
		el("button", "dw-iconbtn", ` type="submit" aria-label="Close"`, ui.I("x"))))

	bodyAttr := ""
	if p.Get != "" {
		bodyAttr = " " + datastar.Init("@get('"+p.Get+"')").String()
	}
	a := ""
	if p.ID != "" {
		a += ` id="` + esc(p.ID) + `"`
	}
	a += attrs(p.Attrs)
	return el("dialog", cls, a,
		el("header", "dw-drawer-head", "", head...),
		el("div", "dw-drawer-body", bodyAttr, inner...))
}

// Popover is a native popover flyout anchored to its trigger: the trigger's
// popovertarget points at the panel, and the platform toggles it with zero
// script. The variadic content is the panel body (menu items, info, a form).
func (Kit) Popover(p ui.PopoverProps, inner ...templ.Component) templ.Component {
	id := p.ID
	if id == "" {
		id = "dw-popover"
	}
	panelID := id + "-panel"
	trigger := make([]templ.Component, 0, 2)
	if p.Icon != "" {
		trigger = append(trigger, ui.I(p.Icon))
	}
	if p.Label != "" {
		trigger = append(trigger, el("span", "dw-btn-label", "", txt(p.Label)))
	}
	btn := el("button", "dw-btn dw-popover-trigger",
		` type="button" popovertarget="`+esc(panelID)+`"`, trigger...)
	panel := el("div", "dw-popover-panel",
		` id="`+esc(panelID)+`" popover anchor="`+esc(id)+`"`, inner...)
	return el("div", "dw-popover", ` id="`+esc(id)+`"`+attrs(p.Attrs), btn, panel)
}

// Tooltip is the native popover attribute + CSS anchor positioning: zero script,
// works before any bundle loads.
func (Kit) Tooltip(p ui.TooltipProps, inner ...templ.Component) templ.Component {
	trigger := el("span", "dw-tooltip-trigger", ` tabindex="0"`, inner...)
	tip := el("span", "dw-tooltip-text", ` role="tooltip"`, txt(p.Text))
	return el("span", "dw-tooltip", attrs(p.Attrs), trigger, tip)
}
