// Package sms sends transactional text messages behind one small interface:
// build a [Message], hand it to a [Sender], and let configuration pick the
// wire. [New] chooses the strongest transport the config supports — the
// Twilio API, then a [LogSender] that prints instead of sending — so a fresh
// checkout "delivers" to the terminal and production delivers for real
// without a code change.
//
// The shape deliberately mirrors pkg/mailer: apps that already wire a Mailer
// wire a Sender the same way, and new transports (Vonage, Telnyx, SNS)
// slot in behind the same interface.
package sms

import (
	"log/slog"
	"os"
	"strings"
)

// Config carries everything [New] needs to pick a transport. Populate it by
// hand or with [ConfigFromEnv]; the sender itself never reads the
// environment, so tests and boot code stay in control of where values come
// from.
type Config struct {
	// FromNumber is the E.164 number messages are sent from. Twilio also
	// accepts a MessagingServiceSID instead; at least one must be set.
	FromNumber string

	TwilioAccountSID          string
	TwilioAuthToken           string
	TwilioMessagingServiceSID string
}

// ConfigFromEnv reads the conventional SMS_/TWILIO_ variables into a Config.
// It is a convenience for boot code that doesn't declare these fields in its
// own config struct.
func ConfigFromEnv() Config {
	return Config{
		FromNumber:                os.Getenv("SMS_FROM_NUMBER"),
		TwilioAccountSID:          os.Getenv("TWILIO_ACCOUNT_SID"),
		TwilioAuthToken:           os.Getenv("TWILIO_AUTH_TOKEN"),
		TwilioMessagingServiceSID: os.Getenv("TWILIO_MESSAGING_SERVICE_SID"),
	}
}

// Sender sends transactional SMS. The package ships Twilio and log-only
// implementations; anything with a Send method — Vonage, Telnyx, SNS — can
// slot in without the app changing.
type Sender interface {
	Send(msg *Message) error
}

// Message is one outbound text. To is an E.164 phone number; Body is plain
// text (carriers split long bodies into segments, the transport doesn't).
type Message struct {
	To   string
	Body string
}

// LogSender prints messages instead of delivering them. It is what [New]
// returns when nothing else is configured, which makes it the development
// default: booking flows work on a fresh checkout and the "text" lands in
// the terminal where you can read it.
type LogSender struct{}

func (s *LogSender) Send(msg *Message) error {
	slog.Info("sms: not sending", "to", msg.To)
	slog.Info("sms: body", "text", msg.Body)
	return nil
}

// New picks a transport from cfg, strongest first: Twilio when an account
// SID is set, [LogSender] otherwise. An account SID without an auth token,
// or without either a from number or a messaging service, is a
// misconfiguration — it warns and falls back to logging rather than
// returning a sender that can only error.
func New(cfg Config) Sender {
	sid := strings.TrimSpace(cfg.TwilioAccountSID)
	if sid == "" {
		return &LogSender{}
	}
	token := strings.TrimSpace(cfg.TwilioAuthToken)
	if token == "" {
		slog.Warn("sms: twilio account sid set but auth token missing, falling back to log sender")
		return &LogSender{}
	}
	from := strings.TrimSpace(cfg.FromNumber)
	service := strings.TrimSpace(cfg.TwilioMessagingServiceSID)
	if from == "" && service == "" {
		slog.Warn("sms: twilio configured but no from number or messaging service, falling back to log sender")
		return &LogSender{}
	}
	return &TwilioSender{
		AccountSID:          sid,
		AuthToken:           token,
		From:                from,
		MessagingServiceSID: service,
	}
}
