package driftwood

import (
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// roleClass maps a semantic Role to a driftwood modifier class. The class —
// not the Go code — binds to the design palette in app.css.
func roleClass(prefix string, r ui.Role) string {
	if r == ui.RoleNeutral {
		return ""
	}
	return " " + prefix + "-" + string(r)
}

func sizeClass(prefix string, s ui.Size) string {
	if s == ui.SizeMd {
		return ""
	}
	return " " + prefix + "-" + string(s)
}

// --- Buttons -----------------------------------------------------------------

func (Kit) Button(p ui.ButtonProps, inner ...templ.Component) templ.Component {
	cls := "dw-btn" + roleClass("dw-btn", p.Role) + sizeClass("dw-btn", p.Size)
	if p.Hazard {
		cls += " dw-btn-hazard"
	}
	typ := p.Type
	if typ == "" {
		typ = "button"
	}
	attr := ` type="` + esc(typ) + `"`
	if p.Disabled {
		attr += " disabled"
	}
	// Loading binds an indicator signal: the button shows a spinner and is
	// aria-busy while the expression is truthy. Visibility is CSS off the signal.
	if p.Loading != "" {
		attr += ` data-attr-aria-busy="` + esc(p.Loading) + `"` +
			` data-attr-disabled="` + esc(p.Loading) + `"`
	}
	attr += attrs(p.Attrs)

	body := make([]templ.Component, 0, 4)
	if p.Loading != "" {
		body = append(body, el("span", "dw-btn-spinner", ` data-show="`+esc(p.Loading)+`" aria-hidden="true"`,
			ui.I("spinner", ui.Spin)))
	}
	if p.LeadIcon != "" {
		body = append(body, ui.I(p.LeadIcon))
	}
	if p.Label != "" {
		body = append(body, el("span", "dw-btn-label", "", txt(p.Label)))
	}
	body = append(body, inner...)
	if p.TailIcon != "" {
		body = append(body, ui.I(p.TailIcon))
	}
	return el("button", cls, attr, body...)
}

func (Kit) IconButton(p ui.IconButtonProps) templ.Component {
	cls := "dw-iconbtn" + roleClass("dw-btn", p.Role) + sizeClass("dw-btn", p.Size)
	attr := ` type="button" aria-label="` + esc(p.Label) + `" title="` + esc(p.Label) + `"` + attrs(p.Attrs)
	return el("button", cls, attr, ui.I(p.Icon))
}

// ButtonGroup joins its buttons into a single segmented control. The grouping is
// visual (CSS): pass already-built Buttons as the variadic children.
func (Kit) ButtonGroup(p ui.ButtonGroupProps, inner ...templ.Component) templ.Component {
	a := ` role="group"`
	if p.Label != "" {
		a += ` aria-label="` + esc(p.Label) + `"`
	}
	return el("div", "dw-btngroup", a+attrs(p.Attrs), inner...)
}

// MenuButton is a label + caret that opens a native popover menu. Toggling is
// the platform popover attribute (popovertarget on the trigger), so it works
// with zero JavaScript; hypermedia items wire an @-expression on click.
func (Kit) MenuButton(p ui.MenuButtonProps) templ.Component {
	id := p.ID
	if id == "" {
		id = "dw-menu"
	}
	panelID := id + "-panel"
	cls := "dw-btn dw-menubtn-trigger" + roleClass("dw-btn", p.Role) + sizeClass("dw-btn", p.Size)
	ta := ` type="button" popovertarget="` + esc(panelID) + `"`
	trigger := make([]templ.Component, 0, 3)
	if p.Icon != "" {
		trigger = append(trigger, ui.I(p.Icon))
	}
	if p.Label != "" {
		trigger = append(trigger, el("span", "dw-btn-label", "", txt(p.Label)))
	}
	trigger = append(trigger, ui.I("chevron-down"))

	items := make([]templ.Component, 0, len(p.Items))
	for _, mi := range p.Items {
		inner := make([]templ.Component, 0, 2)
		if mi.Icon != "" {
			inner = append(inner, ui.I(mi.Icon))
		}
		inner = append(inner, el("span", "dw-menu-text", "", txt(mi.Label)))
		switch {
		case mi.Href != "":
			items = append(items, el("li", "dw-menu-item", "",
				el("a", "dw-menu-link", ` href="`+esc(mi.Href)+`"`, inner...)))
		case mi.On != "":
			ia := ` type="button" ` + datastar.On("click", mi.On).String()
			items = append(items, el("li", "dw-menu-item", "",
				el("button", "dw-menu-link", ia, inner...)))
		default:
			items = append(items, el("li", "dw-menu-item", "",
				el("button", "dw-menu-link", ` type="button"`, inner...)))
		}
	}
	menu := el("ul", "dw-menu", ` id="`+esc(panelID)+`" popover anchor="`+esc(id)+`"`, items...)
	return el("div", "dw-menubtn", ` id="`+esc(id)+`"`+attrs(p.Attrs),
		el("button", cls, ta, trigger...), menu)
}

// --- Forms -------------------------------------------------------------------

func (Kit) Field(p ui.FieldProps, inner ...templ.Component) templ.Component {
	cls := "dw-field"
	if p.Error != "" {
		cls += " is-error"
	}
	body := make([]templ.Component, 0, len(inner)+3)
	if p.Label != "" {
		la := ""
		if p.For != "" {
			la = ` for="` + esc(p.For) + `"`
		}
		body = append(body, el("label", "dw-label", la, txt(p.Label)))
	}
	body = append(body, inner...)
	if p.Help != "" && p.Error == "" {
		body = append(body, el("small", "dw-help", "", txt(p.Help)))
	}
	if p.Error != "" {
		body = append(body, el("small", "dw-field-error", ` role="alert"`, txt(p.Error)))
	}
	return el("div", cls, attrs(p.Attrs), body...)
}

func (Kit) TextInput(p ui.TextInputProps) templ.Component {
	typ := p.Type
	if typ == "" {
		typ = "text"
	}
	cls := "dw-input"
	if p.Error {
		cls += " is-error"
	}
	a := ` type="` + esc(typ) + `"`
	if p.ID != "" {
		a += ` id="` + esc(p.ID) + `"`
	}
	if p.Name != "" {
		a += ` name="` + esc(p.Name) + `"`
		// Bind to a same-named Datastar signal so a form @post submits the value
		// (Datastar posts signals, not raw form fields).
		a += " " + datastar.Bind(p.Name).String()
	}
	if p.Value != "" {
		a += ` value="` + esc(p.Value) + `"`
	}
	if p.Placeholder != "" {
		a += ` placeholder="` + esc(p.Placeholder) + `"`
	}
	if p.Disabled {
		a += " disabled"
	}
	if p.Error {
		a += ` aria-invalid="true"`
	}
	a += attrs(p.Attrs)
	input := raw(`<input class="` + cls + `"` + a + `>`)

	// Prefix/suffix wrap the bare input in an addon group.
	if p.Prefix == "" && p.Suffix == "" {
		return input
	}
	parts := make([]templ.Component, 0, 3)
	if p.Prefix != "" {
		parts = append(parts, el("span", "dw-input-addon dw-input-prefix", "", txt(p.Prefix)))
	}
	parts = append(parts, input)
	if p.Suffix != "" {
		parts = append(parts, el("span", "dw-input-addon dw-input-suffix", "", txt(p.Suffix)))
	}
	return el("div", "dw-input-group", "", parts...)
}

func (Kit) Textarea(p ui.TextareaProps) templ.Component {
	cls := "dw-textarea"
	if p.Error {
		cls += " is-error"
	}
	if p.AutoGrow {
		cls += " dw-autogrow"
	}
	a := ""
	if p.ID != "" {
		a += ` id="` + esc(p.ID) + `"`
	}
	if p.Name != "" {
		a += ` name="` + esc(p.Name) + `"`
		// Bind to a same-named Datastar signal so a form @post submits the value
		// (Datastar posts signals, not raw form fields).
		a += " " + datastar.Bind(p.Name).String()
	}
	if p.Rows > 0 {
		a += ` rows="` + itoa(p.Rows) + `"`
	}
	if p.Error {
		a += ` aria-invalid="true"`
	}
	a += attrs(p.Attrs)
	return el("textarea", cls, a, txt(p.Value))
}

func (Kit) Select(p ui.SelectProps) templ.Component {
	opts := make([]templ.Component, 0, len(p.Options)+1)
	if p.Placeholder != "" {
		opts = append(opts, el("option", "", ` value="" disabled selected`, txt(p.Placeholder)))
	}
	for _, o := range p.Options {
		oa := ` value="` + esc(o.Value) + `"`
		if o.Selected {
			oa += " selected"
		}
		opts = append(opts, el("option", "", oa, txt(o.Label)))
	}
	cls := "dw-select"
	if p.Error {
		cls += " is-error"
	}
	a := ""
	if p.ID != "" {
		a += ` id="` + esc(p.ID) + `"`
	}
	if p.Name != "" {
		a += ` name="` + esc(p.Name) + `"`
		// Bind to a same-named Datastar signal so a form @post submits the value
		// (Datastar posts signals, not raw form fields).
		a += " " + datastar.Bind(p.Name).String()
	}
	if p.Error {
		a += ` aria-invalid="true"`
	}
	a += attrs(p.Attrs)
	return el("div", "dw-select-wrap", "", el("select", cls, a, opts...))
}

// Form wraps fields with a layout and an optional hypermedia target. A @post or
// @get on the form is the sanctioned submit channel; default submission is
// prevented by Datastar.
func (Kit) Form(p ui.FormProps, inner ...templ.Component) templ.Component {
	cls := "dw-form"
	if p.TwoCol {
		cls += " is-twocol"
	}
	a := ""
	switch {
	case p.Post != "":
		a = " " + datastar.OnSubmit("@post('"+p.Post+"')").String()
	case p.Get != "":
		a = " " + datastar.OnSubmit("@get('"+p.Get+"')").String()
	}
	a += attrs(p.Attrs)
	return el("form", cls, a, inner...)
}

func (Kit) Checkbox(p ui.CheckboxProps) templ.Component {
	a := ` type="checkbox" class="dw-checkbox"`
	if p.ID != "" {
		a += ` id="` + esc(p.ID) + `"`
	}
	if p.Name != "" {
		a += ` name="` + esc(p.Name) + `"`
		// Bind to a same-named Datastar signal so a form @post submits the value
		// (Datastar posts signals, not raw form fields).
		a += " " + datastar.Bind(p.Name).String()
	}
	if p.Value != "" {
		a += ` value="` + esc(p.Value) + `"`
	}
	if p.Checked {
		a += " checked"
	}
	if p.Error {
		a += ` aria-invalid="true"`
	}
	a += attrs(p.Attrs)
	box := raw(`<input` + a + `>`)
	return el("label", "dw-check", "", box, el("span", "dw-check-label", "", txt(p.Label)))
}

func (Kit) RadioGroup(p ui.RadioGroupProps) templ.Component {
	cls := "dw-radiogroup"
	if p.Inline {
		cls += " is-inline"
	}
	opts := make([]templ.Component, 0, len(p.Options))
	for i, o := range p.Options {
		ra := ` type="radio" class="dw-radio" name="` + esc(p.Name) + `" value="` + esc(o.Value) + `"`
		ra += ` id="` + esc(p.Name) + "-" + itoa(i) + `"`
		if o.Checked {
			ra += " checked"
		}
		opts = append(opts, el("label", "dw-radio-row", "",
			raw(`<input`+ra+`>`),
			el("span", "dw-radio-label", "", txt(o.Label))))
	}
	return el("div", cls, ` role="radiogroup"`+attrs(p.Attrs), opts...)
}

func (Kit) Toggle(p ui.ToggleProps) templ.Component {
	a := ` type="checkbox" role="switch" class="dw-toggle-input"`
	if p.ID != "" {
		a += ` id="` + esc(p.ID) + `"`
	}
	if p.Name != "" {
		a += ` name="` + esc(p.Name) + `"`
		// Bind to a same-named Datastar signal so a form @post submits the value
		// (Datastar posts signals, not raw form fields).
		a += " " + datastar.Bind(p.Name).String()
	}
	if p.Checked {
		a += " checked"
	}
	a += attrs(p.Attrs)
	text := make([]templ.Component, 0, 2)
	text = append(text, el("span", "dw-toggle-label", "", txt(p.Label)))
	if p.Description != "" {
		text = append(text, el("span", "dw-toggle-desc", "", txt(p.Description)))
	}
	return el("label", "dw-toggle", "",
		raw(`<input`+a+`>`),
		el("span", "dw-toggle-track", ` aria-hidden="true"`, el("span", "dw-toggle-thumb", "")),
		el("span", "dw-toggle-text", "", text...))
}

// InputGroup is a button-attached input (search, chat send): the input and its
// trailing button render joined into one control.
func (k Kit) InputGroup(p ui.InputGroupProps) templ.Component {
	return el("div", "dw-inputgroup", attrs(p.Attrs),
		k.TextInput(p.Input),
		k.Button(p.Button))
}

// FormError is the form-level error summary banner (distinct from per-field
// errors). It lists the individual problems for a server-rendered failed submit.
func (Kit) FormError(p ui.FormErrorProps) templ.Component {
	body := make([]templ.Component, 0, 2)
	body = append(body, el("span", "dw-formerror-icon", ` aria-hidden="true"`, ui.I("alert-octagon")))
	content := make([]templ.Component, 0, 2)
	if p.Title != "" {
		content = append(content, el("strong", "dw-formerror-title", "", txt(p.Title)))
	}
	if len(p.Messages) > 0 {
		lis := make([]templ.Component, 0, len(p.Messages))
		for _, m := range p.Messages {
			lis = append(lis, el("li", "dw-formerror-item", "", txt(m)))
		}
		content = append(content, el("ul", "dw-formerror-list", "", lis...))
	}
	body = append(body, el("div", "dw-formerror-body", "", content...))
	return el("div", "dw-formerror", ` role="alert"`+attrs(p.Attrs), body...)
}
