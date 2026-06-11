package main

import (
	"context"
	"io"

	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
	"github.com/a-h/templ"
)

// kit is the driftwood kit value. Its zero value satisfies the whole ui.Kit
// interface, so the package-level view builders can render kit components
// without threading a *Ctx through every call.
var kit driftwood.Kit

// room wraps inner components in the single live region every state transition
// morphs in place: <div id="room"> ... </div>. The framework's SSE loop morphs
// this element by its own id (outer morph), so swapping lobby/waiting/chat is one
// fragment publish.
func room(inner ...templ.Component) templ.Component {
	return wrap("div", roomTarget, "db-room", inner...)
}

// wrap renders <tag id="id" class="class"> with the given children. A small
// helper so every fragment carries a stable id for morph-by-id fan-out.
func wrap(tag, id, class string, inner ...templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<`+tag+` id="`+id+`" class="`+class+`">`); err != nil {
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
		_, err := io.WriteString(w, `</`+tag+`>`)
		return err
	})
}

// presence renders a driftwood presence pill (a status dot + label).
func presence(state, label string) templ.Component {
	return kit.Messaging().PresencePill(ui.PresencePillProps{State: state, Label: label})
}

// --- room states -------------------------------------------------------------

// lobbyFragment is the idle state: a call to action that posts /start to enter
// matchmaking. This is the initial #room on a fresh page.
func lobbyFragment() templ.Component {
	return room(
		kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: "Ready when you are"},
			presence("idle", "Not connected"),
			text("p", "db-lead", "Tap below and we'll drop you into a chat with the next available stranger. No names, no history — when you leave, it's gone."),
			kit.Buttons().Button(ui.ButtonProps{
				Label:    "Talk to a stranger",
				Role:     ui.RoleAccent,
				LeadIcon: "play",
				Attrs:    datastar.Attrs{datastar.OnClick("@post('/start')")},
			}),
		),
	)
}

// waitingFragment is the lobby-queue state: parked, waiting for a partner.
func waitingFragment() templ.Component {
	return room(
		kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: "Looking for a stranger…"},
			presence("waiting", "Waiting for a match"),
			kit.Feedback().Spinner(ui.SpinnerProps{Block: true}),
			text("p", "db-lead", "Hang tight — pairing you with the next person who shows up."),
			kit.Buttons().Button(ui.ButtonProps{
				Label:    "Cancel",
				Role:     ui.RoleGhost,
				LeadIcon: "x",
				Attrs:    datastar.Attrs{datastar.OnClick("@post('/next')")},
			}),
		),
	)
}

// chatFragment is an active, empty chat (just paired). chatFragmentWith adds an
// existing feed (used by the SSE catch-up replay on reconnect).
func chatFragment() templ.Component { return chatFragmentWith(nil) }

func chatFragmentWith(feed []chatMessage) templ.Component {
	return room(
		kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: "Connected to a stranger"},
			presence("paired", "You're chatting with a stranger"),
			// #notice is an inline slot for transient moderation/system lines.
			wrap("div", "notice", "db-notice"),
			feedFragment(feed),
			kit.Messaging().Composer(ui.ComposerProps{
				Name:        "msg",
				Placeholder: "Say something…",
				Post:        "/say",
			}),
			kit.Buttons().Button(ui.ButtonProps{
				Label:    "Next stranger",
				Role:     ui.RoleNeutral,
				LeadIcon: "skip-forward",
				Attrs:    datastar.Attrs{datastar.OnClick("@post('/next')")},
			}),
		),
	)
}

// feedFragment renders the message list inside #feed. It is published on its own
// (morphing #feed by id) on every new message, and embedded inside a chat
// fragment on first render / catch-up.
func feedFragment(feed []chatMessage) templ.Component {
	msgs := make([]ui.Message, 0, len(feed))
	for _, m := range feed {
		msgs = append(msgs, ui.Message{
			Body: m.Body,
			Own:  m.From == "me",
		})
	}
	list := kit.Messaging().MessageList(ui.MessageListProps{Messages: msgs})
	return wrap("div", feedTarget, "db-feed", list)
}

// endedFragment is the post-chat state shown to the partner left behind when the
// other side hits Next or disconnects: a notice plus a button to find someone new.
func endedFragment() templ.Component {
	return room(
		kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: "The stranger left"},
			presence("idle", "Disconnected"),
			text("p", "db-lead", "They moved on. Want to meet someone else?"),
			kit.Buttons().Button(ui.ButtonProps{
				Label:    "Find another stranger",
				Role:     ui.RoleAccent,
				LeadIcon: "play",
				Attrs:    datastar.Attrs{datastar.OnClick("@post('/start')")},
			}),
		),
	)
}

// noticeFragment renders a transient inline line into #notice (rate-limit nudges,
// system messages). It is an alert styled by the kit.
func noticeFragment(msg string) templ.Component {
	return wrap("div", "notice", "db-notice",
		kit.Feedback().Alert(ui.AlertProps{Role: ui.RoleWarn, Dismissible: true}, text("span", "", msg)),
	)
}

// text renders <tag class="class">escaped-content</tag>.
func text(tag, class, content string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		open := "<" + tag
		if class != "" {
			open += ` class="` + class + `"`
		}
		open += ">"
		if _, err := io.WriteString(w, open); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(content)); err != nil {
			return err
		}
		_, err := io.WriteString(w, "</"+tag+">")
		return err
	})
}
