package driftwood

import (
	"strconv"

	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// alertIcon picks a glyph that matches the alert role, so the meaning survives
// without color (color is never the only encoding).
func alertIcon(r ui.Role) string {
	switch r {
	case ui.RoleGood:
		return "check-circle"
	case ui.RoleWarn:
		return "alert-triangle"
	case ui.RoleDanger:
		return "alert-octagon"
	default:
		return "info-circle"
	}
}

// --- Feedback ----------------------------------------------------------------

func (Kit) Alert(p ui.AlertProps, inner ...templ.Component) templ.Component {
	cls := "dw-alert" + roleClass("dw-alert", p.Role)
	body := make([]templ.Component, 0, len(inner)+3)
	body = append(body, el("span", "dw-alert-icon", ` aria-hidden="true"`, ui.I(alertIcon(p.Role))))
	content := make([]templ.Component, 0, len(inner)+1)
	if p.Title != "" {
		content = append(content, el("strong", "dw-alert-title", "", txt(p.Title)))
	}
	content = append(content, inner...)
	body = append(body, el("div", "dw-alert-body", "", content...))
	if p.Dismissible {
		// Native close with zero script: the alert is hidden via :checked on a
		// sibling checkbox toggled by the close button's label.
		body = append(body, el("button", "dw-alert-close",
			` type="button" aria-label="Dismiss" data-on:click="el.closest('.dw-alert').remove()"`,
			ui.I("x")))
	}
	return el("div", cls, ` role="alert"`+attrs(p.Attrs), body...)
}

// Toast is a single notification card. It is SSE-pushable: a handler patches one
// into the kit's toast region; manual close removes it with no page script of
// its own (the same self-remove handler the Alert uses). Auto-dismiss is CSS.
func (Kit) Toast(p ui.ToastProps) templ.Component {
	cls := "dw-toast" + roleClass("dw-toast", p.Role)
	body := make([]templ.Component, 0, 3)
	body = append(body, el("span", "dw-toast-icon", ` aria-hidden="true"`, ui.I(alertIcon(p.Role))))
	content := make([]templ.Component, 0, 2)
	if p.Title != "" {
		content = append(content, el("strong", "dw-toast-title", "", txt(p.Title)))
	}
	if p.Message != "" {
		content = append(content, el("p", "dw-toast-msg", "", txt(p.Message)))
	}
	body = append(body, el("div", "dw-toast-body", "", content...))
	body = append(body, el("button", "dw-toast-close",
		` type="button" aria-label="Dismiss" data-on:click="el.closest('.dw-toast').remove()"`,
		ui.I("x")))
	return el("div", cls, ` role="status" aria-live="polite"`+attrs(p.Attrs), body...)
}

// Progress is a progress bar. Max == 0 makes it indeterminate (unknown-duration
// loads); otherwise it reflects Value/Max with the proper ARIA range.
func (Kit) Progress(p ui.ProgressProps) templ.Component {
	cls := "dw-progress" + roleClass("dw-progress", p.Role)
	a := ` role="progressbar"`
	if p.Label != "" {
		a += ` aria-label="` + esc(p.Label) + `"`
	}
	barStyle := ""
	if p.Max > 0 {
		pct := p.Value / p.Max * 100
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		a += ` aria-valuenow="` + strconv.FormatFloat(p.Value, 'f', -1, 64) + `"` +
			` aria-valuemin="0" aria-valuemax="` + strconv.FormatFloat(p.Max, 'f', -1, 64) + `"`
		barStyle = ` style="width:` + strconv.FormatFloat(pct, 'f', 2, 64) + `%"`
	} else {
		cls += " is-indeterminate"
	}
	a += attrs(p.Attrs)
	return el("div", cls, a, el("div", "dw-progress-bar", barStyle))
}

func (Kit) EmptyState(p ui.EmptyStateProps) templ.Component {
	body := make([]templ.Component, 0, 2)
	if p.Icon != "" {
		body = append(body, el("span", "dw-empty-icon", ` aria-hidden="true"`, ui.I(p.Icon)))
	}
	body = append(body, el("p", "dw-empty-msg", "", txt(p.Message)))
	return el("div", "dw-empty", attrs(p.Attrs), body...)
}

func (Kit) Spinner(p ui.SpinnerProps) templ.Component {
	cls := "dw-spinner" + sizeClass("dw-spinner", p.Size)
	if p.Block {
		cls += " is-block"
	}
	return el("span", cls, ` role="status" aria-label="Loading"`+attrs(p.Attrs),
		ui.I("spinner", ui.Spin))
}

func (Kit) Skeleton(p ui.SkeletonProps) templ.Component {
	style := ""
	if p.Width != "" {
		style += "width:" + esc(p.Width) + ";"
	}
	if p.Height != "" {
		style += "height:" + esc(p.Height) + ";"
	}
	a := ` aria-hidden="true"`
	if style != "" {
		a += ` style="` + style + `"`
	}
	return el("div", "dw-skeleton", a+attrs(p.Attrs))
}
