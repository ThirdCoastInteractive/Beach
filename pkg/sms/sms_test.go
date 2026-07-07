package sms

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewSelectsTransport(t *testing.T) {
	// twilio wins when sid, token, and a from number are present.
	s := New(Config{TwilioAccountSID: "AC123", TwilioAuthToken: "tok", FromNumber: "+15550001111"})
	tw, ok := s.(*TwilioSender)
	if !ok {
		t.Fatalf("twilio config: got %T", s)
	}
	if tw.AccountSID != "AC123" || tw.AuthToken != "tok" || tw.From != "+15550001111" {
		t.Errorf("twilio fields: %+v", tw)
	}

	// a messaging service stands in for the from number.
	s = New(Config{TwilioAccountSID: "AC123", TwilioAuthToken: "tok", TwilioMessagingServiceSID: "MG456"})
	if tw := s.(*TwilioSender); tw.MessagingServiceSID != "MG456" {
		t.Errorf("messaging service = %q", tw.MessagingServiceSID)
	}

	// a sid without a token is misconfiguration: fall back to logging.
	if s = New(Config{TwilioAccountSID: "AC123", FromNumber: "+15550001111"}); !isLogSender(s) {
		t.Errorf("sid without token: got %T", s)
	}

	// a sid + token without any sending identity is too.
	if s = New(Config{TwilioAccountSID: "AC123", TwilioAuthToken: "tok"}); !isLogSender(s) {
		t.Errorf("no from or service: got %T", s)
	}

	// nothing configured: the dev default.
	if s = New(Config{}); !isLogSender(s) {
		t.Errorf("empty config: got %T", s)
	}
}

func isLogSender(s Sender) bool {
	_, ok := s.(*LogSender)
	return ok
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("SMS_FROM_NUMBER", "+15550001111")
	t.Setenv("TWILIO_ACCOUNT_SID", "AC123")
	t.Setenv("TWILIO_AUTH_TOKEN", "tok")
	t.Setenv("TWILIO_MESSAGING_SERVICE_SID", "MG456")

	got := ConfigFromEnv()
	want := Config{
		FromNumber:                "+15550001111",
		TwilioAccountSID:          "AC123",
		TwilioAuthToken:           "tok",
		TwilioMessagingServiceSID: "MG456",
	}
	if got != want {
		t.Errorf("ConfigFromEnv = %+v, want %+v", got, want)
	}
}

func TestTwilioSend(t *testing.T) {
	var gotPath, gotAuthSID, gotAuthToken string
	var gotForm map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthSID, gotAuthToken, _ = r.BasicAuth()
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotForm = map[string]string{}
		for k := range r.PostForm {
			gotForm[k] = r.PostForm.Get(k)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"SM1"}`))
	}))
	defer srv.Close()

	tw := &TwilioSender{AccountSID: "AC123", AuthToken: "tok", From: "+15550001111", BaseURL: srv.URL}
	err := tw.Send(&Message{To: "+15552223333", Body: "your door code is 4821"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if gotPath != "/2010-04-01/Accounts/AC123/Messages.json" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuthSID != "AC123" || gotAuthToken != "tok" {
		t.Errorf("basic auth = %q / %q", gotAuthSID, gotAuthToken)
	}
	want := map[string]string{"To": "+15552223333", "Body": "your door code is 4821", "From": "+15550001111"}
	for k, v := range want {
		if gotForm[k] != v {
			t.Errorf("form %s = %q, want %q", k, gotForm[k], v)
		}
	}
}

func TestTwilioSendMessagingService(t *testing.T) {
	var gotForm map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotForm = map[string]string{}
		for k := range r.PostForm {
			gotForm[k] = r.PostForm.Get(k)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	tw := &TwilioSender{AccountSID: "AC123", AuthToken: "tok", MessagingServiceSID: "MG456", BaseURL: srv.URL}
	if err := tw.Send(&Message{To: "+15552223333", Body: "hi"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotForm["MessagingServiceSid"] != "MG456" {
		t.Errorf("MessagingServiceSid = %q", gotForm["MessagingServiceSid"])
	}
	if _, ok := gotForm["From"]; ok {
		t.Errorf("From should be absent when a messaging service is set: %v", gotForm)
	}
}

func TestTwilioSendAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":21211,"message":"The 'To' number is not a valid phone number.","status":400}`))
	}))
	defer srv.Close()

	tw := &TwilioSender{AccountSID: "AC123", AuthToken: "tok", From: "+15550001111", BaseURL: srv.URL}
	err := tw.Send(&Message{To: "nope", Body: "hi"})
	if err == nil {
		t.Fatal("want error on 400")
	}
	if want := "The 'To' number is not a valid phone number."; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
}

func TestUnconfiguredSendErrors(t *testing.T) {
	if err := (&TwilioSender{}).Send(&Message{To: "+15552223333"}); err == nil {
		t.Error("empty TwilioSender.Send: want error")
	}
	if err := (&TwilioSender{AccountSID: "AC123", AuthToken: "tok"}).Send(&Message{To: "+15552223333"}); err == nil {
		t.Error("no from/service TwilioSender.Send: want error")
	}
}
