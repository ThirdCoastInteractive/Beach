// Package mailer sends transactional email behind one small interface: build
// an [Email], hand it to a [Mailer], and let configuration pick the wire.
// [New] chooses the strongest transport the config supports — the Mailgun API,
// then plain SMTP, then a [LogMailer] that prints instead of sending — so a
// fresh checkout "delivers" to the terminal and production delivers for real
// without a code change.
//
// Ported from manifold13.tv's pkg/mailer. Only the transport layer came along;
// templating and click tracking stay app-side.
package mailer

import (
	"bytes"
	"fmt"
	"html"
	"log/slog"
	"net/smtp"
	"os"
	"strings"
)

// Config carries everything [New] needs to pick a transport. Populate it by
// hand or with [ConfigFromEnv]; the mailer itself never reads the environment,
// so tests and boot code stay in control of where values come from.
type Config struct {
	FromName string
	FromAddr string

	MailgunKey    string
	MailgunDomain string

	SMTPHost     string
	SMTPPort     string // defaulted to "587" when empty
	SMTPUsername string
	SMTPPassword string
}

// ConfigFromEnv reads the conventional MAIL_/MAILGUN_/SMTP_ variables into a
// Config. It is a convenience for boot code that doesn't declare these fields
// in its own config struct; SMTP_PORT defaults to 587, the submission port.
func ConfigFromEnv() Config {
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if port == "" {
		port = "587"
	}
	return Config{
		FromName:      os.Getenv("MAIL_FROM_NAME"),
		FromAddr:      os.Getenv("MAIL_FROM_ADDR"),
		MailgunKey:    os.Getenv("MAILGUN_KEY"),
		MailgunDomain: os.Getenv("MAILGUN_DOMAIN"),
		SMTPHost:      os.Getenv("SMTP_HOST"),
		SMTPPort:      port,
		SMTPUsername:  os.Getenv("SMTP_USERNAME"),
		SMTPPassword:  os.Getenv("SMTP_PASSWORD"),
	}
}

// Mailer sends transactional email. The package ships Mailgun, SMTP, and
// log-only implementations; anything with a Send method — SendGrid, Postmark,
// SES — can slot in without the app changing.
type Mailer interface {
	Send(msg *Email) error
}

// Email is one outbound message. Text is the canonical body; HTML is optional,
// and transports that want both parts derive one via [HTMLFromText].
type Email struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// SMTPMailer speaks plain SMTP via net/smtp. Every message goes out as
// multipart/alternative with both a text and an HTML part — when the caller
// gave no HTML, [HTMLFromText] fabricates one — so any client renders
// something sensible. Auth is PlainAuth when a username is set, anonymous
// otherwise (a relay on localhost needs none).
type SMTPMailer struct {
	Addr     string // host:port dial target
	Host     string // bare host, what PlainAuth identifies against
	Username string
	Password string
	From     string
	FromName string
}

func (m *SMTPMailer) Send(msg *Email) error {
	if m == nil || m.Host == "" || m.From == "" {
		return fmt.Errorf("smtp mailer not configured")
	}
	data := buildMIMEMessage(m, msg)
	var auth smtp.Auth
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}
	return smtp.SendMail(m.Addr, auth, m.From, []string{msg.To}, data)
}

func buildMIMEMessage(m *SMTPMailer, msg *Email) []byte {
	boundary := "beach-boundary-2026"
	from := m.From
	if m.FromName != "" {
		from = fmt.Sprintf("%s <%s>", m.FromName, m.From)
	}
	htmlBody := msg.HTML
	if htmlBody == "" {
		htmlBody = HTMLFromText(msg.Text)
	}
	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + msg.To + "\r\n")
	buf.WriteString("Subject: " + msg.Subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(msg.Text + "\r\n")
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	buf.WriteString(htmlBody + "\r\n")
	buf.WriteString("--" + boundary + "--\r\n")
	return buf.Bytes()
}

// LogMailer prints messages instead of delivering them. It is what [New]
// returns when nothing else is configured, which makes it the development
// default: signup flows work on a fresh checkout and the "email" lands in the
// terminal where you can read it.
type LogMailer struct{}

func (m *LogMailer) Send(msg *Email) error {
	slog.Info("mailer: not sending", "to", msg.To, "subject", msg.Subject)
	slog.Info("mailer: body", "text", msg.Text)
	return nil
}

// HTMLFromText wraps escaped plain text in a minimal dark container, so a
// text-only message still ships a presentable HTML part. Newlines become
// <br>; everything else is escaped verbatim.
func HTMLFromText(text string) string {
	escaped := html.EscapeString(text)
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return `<div style="font-family: sans-serif; max-width: 700px; margin: 0 auto; color: #e5e5e5; background: #0a0a0a; padding: 32px;">` + escaped + `</div>`
}

// New picks a transport from cfg, strongest first: Mailgun when an API key is
// set, SMTP when a host is, [LogMailer] otherwise. A Mailgun key without a
// domain is a misconfiguration — it warns and falls back to logging rather
// than returning a mailer that can only error. Missing from addresses default
// to noreply@domain (Mailgun) or the SMTP username, so the minimum viable
// config is just the credentials.
func New(cfg Config) Mailer {
	if key := strings.TrimSpace(cfg.MailgunKey); key != "" {
		domain := strings.TrimSpace(cfg.MailgunDomain)
		if domain == "" {
			slog.Warn("mailer: mailgun key set but domain missing, falling back to log mailer")
			return &LogMailer{}
		}
		fromAddr := cfg.FromAddr
		if fromAddr == "" {
			fromAddr = "noreply@" + domain
		}
		return &MailgunMailer{APIKey: key, Domain: domain, From: fromAddr}
	}

	host := strings.TrimSpace(cfg.SMTPHost)
	if host == "" {
		return &LogMailer{}
	}
	port := strings.TrimSpace(cfg.SMTPPort)
	if port == "" {
		port = "587"
	}
	fromAddr := cfg.FromAddr
	if fromAddr == "" {
		fromAddr = strings.TrimSpace(cfg.SMTPUsername)
	}
	return &SMTPMailer{
		Addr:     host + ":" + port,
		Host:     host,
		Username: strings.TrimSpace(cfg.SMTPUsername),
		Password: cfg.SMTPPassword,
		From:     fromAddr,
		FromName: cfg.FromName,
	}
}
