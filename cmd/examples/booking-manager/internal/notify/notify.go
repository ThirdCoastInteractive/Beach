// Package notify sends booking-manager's guest messages: an acknowledgement
// when an inquiry arrives and a confirmation (with the door code) when a
// booking is confirmed. Email goes through pkg/mailer and texts through
// pkg/sms — both behind one-method interfaces whose transports are picked by
// configuration, so a fresh checkout "delivers" to the terminal.
//
// Sends are best-effort: a guest message failing to leave never fails the
// request that triggered it. Failures are logged and the operator can resend
// by hand; an app that needs delivery guarantees adds an outbox.
package notify

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/mailer"
	"github.com/ThirdCoastInteractive/Beach/pkg/sms"
)

// Notifier fans one guest event out to the configured channels: email always
// (when the guest gave an address), SMS when the guest gave a phone number.
type Notifier struct {
	mailer mailer.Mailer
	sms    sms.Sender
	log    *slog.Logger
}

// New wires a notifier over the already-constructed transports.
func New(m mailer.Mailer, s sms.Sender, log *slog.Logger) *Notifier {
	return &Notifier{mailer: m, sms: s, log: log}
}

// InquiryReceived acknowledges a new inquiry to the guest by email.
func (n *Notifier) InquiryReceived(guestName, guestEmail, propertyName string) {
	if guestEmail == "" {
		return
	}
	text := fmt.Sprintf(
		"Hi %s,\n\nThanks for asking about %s — we got your inquiry and will get back to you with a quote shortly.\n",
		guestName, propertyName)
	n.send(&mailer.Email{To: guestEmail, Subject: "We got your inquiry", Text: text}, nil)
}

// BookingConfirmed tells the guest their stay is on, by email and (when a
// phone number is on file) by text. doorCode may be empty when the property
// has no connected lock.
func (n *Notifier) BookingConfirmed(guestName, guestEmail, guestPhone, propertyName string, checkIn, checkOut time.Time, doorCode string) {
	text := fmt.Sprintf("Hi %s,\n\nYour stay at %s is confirmed.\n\nCheck-in:  %s\nCheck-out: %s\n",
		guestName, propertyName, checkIn.Format("Mon, Jan 2 2006"), checkOut.Format("Mon, Jan 2 2006"))
	if doorCode != "" {
		text += fmt.Sprintf("Door code: %s\n", doorCode)
	}
	text += "\nSee you soon!\n"

	var msg *sms.Message
	if guestPhone != "" {
		body := fmt.Sprintf("%s: you're confirmed %s – %s.",
			propertyName, checkIn.Format("Jan 2"), checkOut.Format("Jan 2"))
		if doorCode != "" {
			body += " Door code " + doorCode + "."
		}
		msg = &sms.Message{To: guestPhone, Body: body}
	}

	var email *mailer.Email
	if guestEmail != "" {
		email = &mailer.Email{To: guestEmail, Subject: "Your stay at " + propertyName + " is confirmed", Text: text}
	}
	n.send(email, msg)
}

// send delivers each non-nil message, logging failures instead of returning
// them — guest messaging never fails the triggering request.
func (n *Notifier) send(email *mailer.Email, msg *sms.Message) {
	if email != nil {
		if err := n.mailer.Send(email); err != nil {
			n.log.Error("notify: email send failed", "to", email.To, "subject", email.Subject, "err", err)
		}
	}
	if msg != nil {
		if err := n.sms.Send(msg); err != nil {
			n.log.Error("notify: sms send failed", "to", msg.To, "err", err)
		}
	}
}
