package mailer

import (
	"context"
	"fmt"
	"time"

	"github.com/mailgun/mailgun-go/v5"
)

// MailgunMailer delivers through the Mailgun HTTP API. It is the production
// transport: no port 25 reachability games, just an API key and a verified
// domain. Each Send gets a fresh client and a 10s deadline — transactional
// mail that can't go out quickly should fail loudly, not queue silently.
type MailgunMailer struct {
	APIKey string
	Domain string
	From   string
}

func (m *MailgunMailer) Send(msg *Email) error {
	if m.APIKey == "" || m.Domain == "" {
		return fmt.Errorf("mailgun mailer not configured")
	}
	mg := mailgun.NewMailgun(m.APIKey)

	from := m.From
	if from == "" {
		from = "noreply@" + m.Domain
	}

	message := mailgun.NewMessage(m.Domain, from, msg.Subject, msg.Text, msg.To)
	if msg.HTML != "" {
		message.SetHTML(msg.HTML)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := mg.Send(ctx, message)
	return err
}
