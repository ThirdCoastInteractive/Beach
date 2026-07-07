package sms

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TwilioSender delivers through the Twilio Messages API. It is the
// production transport: one form POST with basic auth, no SDK. Each Send
// gets a 10s deadline — transactional texts that can't go out quickly
// should fail loudly, not queue silently.
type TwilioSender struct {
	AccountSID string
	AuthToken  string
	// From is the sending E.164 number. MessagingServiceSID may be set
	// instead (or as well); Twilio requires at least one.
	From                string
	MessagingServiceSID string

	// BaseURL overrides the Twilio API root; tests point it at a local
	// server. Empty means the real API.
	BaseURL string
	// Client overrides the HTTP client. Nil means a client with a 10s
	// timeout.
	Client *http.Client
}

func (s *TwilioSender) Send(msg *Message) error {
	if s.AccountSID == "" || s.AuthToken == "" {
		return fmt.Errorf("twilio sender not configured")
	}
	if s.From == "" && s.MessagingServiceSID == "" {
		return fmt.Errorf("twilio sender has no from number or messaging service")
	}

	form := url.Values{}
	form.Set("To", msg.To)
	form.Set("Body", msg.Body)
	if s.MessagingServiceSID != "" {
		form.Set("MessagingServiceSid", s.MessagingServiceSID)
	} else {
		form.Set("From", s.From)
	}

	base := s.BaseURL
	if base == "" {
		base = "https://api.twilio.com"
	}
	endpoint := strings.TrimSuffix(base, "/") + "/2010-04-01/Accounts/" + url.PathEscape(s.AccountSID) + "/Messages.json"

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("twilio request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.AccountSID, s.AuthToken)

	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("twilio send: %s: %s", resp.Status, twilioError(resp.Body))
	}
	return nil
}

// twilioError pulls the human-readable message out of a Twilio error body,
// falling back to the raw bytes when it isn't the documented JSON shape.
func twilioError(r io.Reader) string {
	body, _ := io.ReadAll(io.LimitReader(r, 4<<10))
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		return e.Message
	}
	return strings.TrimSpace(string(body))
}
