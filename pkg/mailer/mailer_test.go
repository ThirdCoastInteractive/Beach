package mailer

import (
	"bufio"
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

func TestNewSelectsTransport(t *testing.T) {
	// mailgun wins when both key and domain are present; the from address
	// defaults to noreply@domain.
	m := New(Config{MailgunKey: "key-x", MailgunDomain: "mg.example.com"})
	mg, ok := m.(*MailgunMailer)
	if !ok {
		t.Fatalf("mailgun config: got %T", m)
	}
	if mg.APIKey != "key-x" || mg.Domain != "mg.example.com" || mg.From != "noreply@mg.example.com" {
		t.Errorf("mailgun fields: %+v", mg)
	}

	// an explicit from address survives.
	m = New(Config{MailgunKey: "key-x", MailgunDomain: "mg.example.com", FromAddr: "hi@example.com"})
	if mg := m.(*MailgunMailer); mg.From != "hi@example.com" {
		t.Errorf("explicit from = %q", mg.From)
	}

	// a key without a domain is misconfiguration: fall back to logging.
	if m = New(Config{MailgunKey: "key-x"}); !isLogMailer(m) {
		t.Errorf("key without domain: got %T", m)
	}

	// smtp is next in line; the port defaults to 587 and the from address
	// falls back to the username.
	m = New(Config{SMTPHost: "smtp.example.com", SMTPUsername: " user@example.com "})
	sm, ok := m.(*SMTPMailer)
	if !ok {
		t.Fatalf("smtp config: got %T", m)
	}
	if sm.Addr != "smtp.example.com:587" || sm.Host != "smtp.example.com" {
		t.Errorf("smtp addr/host: %+v", sm)
	}
	if sm.Username != "user@example.com" || sm.From != "user@example.com" {
		t.Errorf("smtp username/from: %+v", sm)
	}

	// an explicit port is respected.
	m = New(Config{SMTPHost: "smtp.example.com", SMTPPort: "2525"})
	if sm := m.(*SMTPMailer); sm.Addr != "smtp.example.com:2525" {
		t.Errorf("smtp explicit port: addr = %q", sm.Addr)
	}

	// nothing configured: the dev default.
	if m = New(Config{}); !isLogMailer(m) {
		t.Errorf("empty config: got %T", m)
	}
}

func isLogMailer(m Mailer) bool {
	_, ok := m.(*LogMailer)
	return ok
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("MAIL_FROM_NAME", "the beach")
	t.Setenv("MAIL_FROM_ADDR", "hi@example.com")
	t.Setenv("MAILGUN_KEY", "key-x")
	t.Setenv("MAILGUN_DOMAIN", "mg.example.com")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_USERNAME", "user")
	t.Setenv("SMTP_PASSWORD", "hunter2")

	got := ConfigFromEnv()
	want := Config{
		FromName: "the beach", FromAddr: "hi@example.com",
		MailgunKey: "key-x", MailgunDomain: "mg.example.com",
		SMTPHost: "smtp.example.com", SMTPPort: "2525",
		SMTPUsername: "user", SMTPPassword: "hunter2",
	}
	if got != want {
		t.Errorf("ConfigFromEnv = %+v, want %+v", got, want)
	}

	// an unset port defaults to the submission port.
	t.Setenv("SMTP_PORT", "")
	if got := ConfigFromEnv(); got.SMTPPort != "587" {
		t.Errorf("default SMTP_PORT = %q", got.SMTPPort)
	}
}

// parseMIME splits buildMIMEMessage output into headers and decoded
// multipart parts so assertions read like statements about the wire format.
func parseMIME(t *testing.T, data []byte) (textproto.MIMEHeader, map[string]string) {
	t.Helper()
	tp := textproto.NewReader(bufio.NewReader(bytes.NewReader(data)))
	hdr, err := tp.ReadMIMEHeader()
	if err != nil {
		t.Fatalf("read headers: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(hdr.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse content type: %v", err)
	}
	if mediaType != "multipart/alternative" {
		t.Fatalf("media type = %q, want multipart/alternative", mediaType)
	}
	parts := map[string]string{}
	mr := multipart.NewReader(tp.R, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		body, _ := io.ReadAll(p)
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		parts[ct] = string(body)
	}
	return hdr, parts
}

func TestBuildMIMEMessage(t *testing.T) {
	m := &SMTPMailer{From: "hi@example.com", FromName: "the beach"}
	msg := &Email{
		To:      "you@example.com",
		Subject: "hello",
		Text:    "plain words",
		HTML:    "<p>fancy words</p>",
	}
	hdr, parts := parseMIME(t, buildMIMEMessage(m, msg))

	if got := hdr.Get("From"); got != "the beach <hi@example.com>" {
		t.Errorf("From = %q", got)
	}
	if got := hdr.Get("To"); got != "you@example.com" {
		t.Errorf("To = %q", got)
	}
	if got := hdr.Get("Subject"); got != "hello" {
		t.Errorf("Subject = %q", got)
	}

	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2: %v", len(parts), parts)
	}
	if !strings.Contains(parts["text/plain"], "plain words") {
		t.Errorf("text part = %q", parts["text/plain"])
	}
	if !strings.Contains(parts["text/html"], "<p>fancy words</p>") {
		t.Errorf("html part = %q", parts["text/html"])
	}
}

func TestBuildMIMEMessageHTMLFallback(t *testing.T) {
	// no HTML given: the html part is fabricated from the text.
	m := &SMTPMailer{From: "hi@example.com"}
	msg := &Email{To: "you@example.com", Subject: "s", Text: "line one\nline <two>"}
	hdr, parts := parseMIME(t, buildMIMEMessage(m, msg))

	// no FromName means a bare address.
	if got := hdr.Get("From"); got != "hi@example.com" {
		t.Errorf("From = %q", got)
	}
	htmlPart := parts["text/html"]
	if !strings.Contains(htmlPart, "line one<br>line &lt;two&gt;") {
		t.Errorf("fabricated html part = %q", htmlPart)
	}
}

func TestHTMLFromText(t *testing.T) {
	got := HTMLFromText("a & b\n<c>")
	if !strings.Contains(got, "a &amp; b<br>&lt;c&gt;") {
		t.Errorf("escaping/breaks: %q", got)
	}
	if !strings.HasPrefix(got, "<div") || !strings.HasSuffix(got, "</div>") {
		t.Errorf("container: %q", got)
	}
}

func TestUnconfiguredSendErrors(t *testing.T) {
	if err := (&SMTPMailer{}).Send(&Email{To: "x@example.com"}); err == nil {
		t.Error("empty SMTPMailer.Send: want error")
	}
	if err := (&MailgunMailer{}).Send(&Email{To: "x@example.com"}); err == nil {
		t.Error("empty MailgunMailer.Send: want error")
	}
}
