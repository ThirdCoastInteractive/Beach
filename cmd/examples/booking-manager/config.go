package main

import bcfg "github.com/ThirdCoastInteractive/Beach/pkg/config"

// appConfig is booking-manager's typed env, loaded once at boot by the
// framework config loader. It embeds Core (AppEnv, Port, DSN via
// POSTGRES_DSN|DATABASE_URL — which is required, no default). Postgres is the
// only store: seasonal operators run this themselves, so the stack stays one
// database. Mail and SMS are optional — unset, both fall back to log
// transports so a fresh checkout "delivers" to the terminal.
type appConfig struct {
	bcfg.Core

	MailFromName string `env:"MAIL_FROM_NAME" default:"Booking Manager"`
	MailFromAddr string `env:"MAIL_FROM_ADDR" default:""`

	MailgunKey    string `env:"MAILGUN_KEY" default:""`
	MailgunDomain string `env:"MAILGUN_DOMAIN" default:""`
	SMTPHost      string `env:"SMTP_HOST" default:""`
	SMTPPort      string `env:"SMTP_PORT" default:"587"`
	SMTPUsername  string `env:"SMTP_USERNAME" default:""`
	SMTPPassword  string `env:"SMTP_PASSWORD" default:""`

	SMSFromNumber      string `env:"SMS_FROM_NUMBER" default:""`
	TwilioAccountSID   string `env:"TWILIO_ACCOUNT_SID" default:""`
	TwilioAuthToken    string `env:"TWILIO_AUTH_TOKEN" default:""`
	TwilioMessagingSID string `env:"TWILIO_MESSAGING_SERVICE_SID" default:""`
}

// loadConfig loads and validates the environment, aborting with the missing
// list if Postgres is unconfigured.
func loadConfig() appConfig { return *bcfg.MustLoad[appConfig]() }
