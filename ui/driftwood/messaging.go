package driftwood

import (
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// --- Messaging ---------------------------------------------------------------

func (Kit) MessageList(p ui.MessageListProps) templ.Component {
	rows := make([]templ.Component, 0, len(p.Messages))
	for _, m := range p.Messages {
		cls := "dw-msg"
		switch {
		case m.System:
			cls += " is-system"
		case m.Own:
			cls += " is-own"
		default:
			cls += " is-other"
		}
		body := make([]templ.Component, 0, 3)
		if !m.System && m.Author != "" {
			body = append(body, el("span", "dw-msg-author", "", txt(m.Author)))
		}
		body = append(body, el("span", "dw-msg-body", "", txt(m.Body)))
		if m.At != "" {
			body = append(body, el("time", "dw-msg-time", "", txt(m.At)))
		}
		rows = append(rows, el("li", cls, "", body...))
	}
	return el("ul", "dw-msglist", attrs(p.Attrs), rows...)
}

func (Kit) Composer(p ui.ComposerProps) templ.Component {
	a := ""
	if p.Post != "" {
		a = " " + datastar.OnSubmit("@post('"+p.Post+"')").String()
	}
	ta := el("textarea", "dw-composer-input",
		` name="`+esc(p.Name)+`" rows="1" placeholder="`+esc(p.Placeholder)+`"`+
			" "+datastar.Bind(p.Name).String())
	send := el("button", "dw-composer-send", ` type="submit" aria-label="Send"`, ui.I("send"))
	return el("form", "dw-composer", a, ta, send)
}

func (Kit) PresencePill(p ui.PresencePillProps) templ.Component {
	cls := "dw-presence"
	if p.State != "" {
		cls += " is-" + esc(p.State)
	}
	return el("span", cls, ` role="status"`+attrs(p.Attrs),
		el("span", "dw-presence-dot", ` aria-hidden="true"`),
		txt(p.Label))
}
