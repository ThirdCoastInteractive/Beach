package notify

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/mailer"
	"github.com/ThirdCoastInteractive/Beach/pkg/sms"
)

type capturedMail struct{ sent []*mailer.Email }

func (m *capturedMail) Send(msg *mailer.Email) error {
	m.sent = append(m.sent, msg)
	return nil
}

type capturedSMS struct{ sent []*sms.Message }

func (s *capturedSMS) Send(msg *sms.Message) error {
	s.sent = append(s.sent, msg)
	return nil
}

func newTest() (*Notifier, *capturedMail, *capturedSMS) {
	m, s := &capturedMail{}, &capturedSMS{}
	return New(m, s, slog.New(slog.NewTextHandler(io.Discard, nil))), m, s
}

func TestBookingConfirmedFansOut(t *testing.T) {
	n, m, s := newTest()
	in := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.Local)
	out := in.AddDate(0, 0, 3)
	n.BookingConfirmed("Ana Whitfield", "ana@example.com", "+15550003333", "Loon Lake Cottage", in, out, "4821")

	if len(m.sent) != 1 {
		t.Fatalf("emails sent = %d, want 1", len(m.sent))
	}
	email := m.sent[0]
	if email.To != "ana@example.com" {
		t.Errorf("email to = %q", email.To)
	}
	for _, want := range []string{"Loon Lake Cottage", "Fri, Jul 10 2026", "Mon, Jul 13 2026", "Door code: 4821"} {
		if !strings.Contains(email.Text, want) {
			t.Errorf("email text missing %q:\n%s", want, email.Text)
		}
	}

	if len(s.sent) != 1 {
		t.Fatalf("texts sent = %d, want 1", len(s.sent))
	}
	text := s.sent[0]
	if text.To != "+15550003333" {
		t.Errorf("sms to = %q", text.To)
	}
	for _, want := range []string{"Loon Lake Cottage", "Jul 10", "Jul 13", "Door code 4821"} {
		if !strings.Contains(text.Body, want) {
			t.Errorf("sms body missing %q: %s", want, text.Body)
		}
	}
}

func TestBookingConfirmedSkipsMissingChannels(t *testing.T) {
	n, m, s := newTest()
	in := time.Now()
	// No phone: email only. No door code: the code line stays out.
	n.BookingConfirmed("Tom Reyes", "tom@example.com", "", "Birch Hollow Cabin", in, in.AddDate(0, 0, 2), "")
	if len(s.sent) != 0 {
		t.Errorf("texts sent = %d, want 0 without a phone", len(s.sent))
	}
	if len(m.sent) != 1 {
		t.Fatalf("emails sent = %d, want 1", len(m.sent))
	}
	if strings.Contains(m.sent[0].Text, "Door code") {
		t.Errorf("email mentions a door code that doesn't exist:\n%s", m.sent[0].Text)
	}
}

func TestInquiryReceived(t *testing.T) {
	n, m, s := newTest()
	n.InquiryReceived("Priya Raman", "priya@example.com", "The Boathouse")
	if len(m.sent) != 1 || !strings.Contains(m.sent[0].Text, "The Boathouse") {
		t.Fatalf("emails = %+v, want one mentioning the property", m.sent)
	}
	if len(s.sent) != 0 {
		t.Errorf("inquiry ack should not text")
	}

	// No email on the inquiry: nothing goes out.
	n.InquiryReceived("Anon", "", "The Boathouse")
	if len(m.sent) != 1 {
		t.Errorf("emails sent = %d, want still 1", len(m.sent))
	}
}
