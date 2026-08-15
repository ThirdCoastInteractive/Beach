package main

import (
	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/reputation"
	bcfg "github.com/ThirdCoastInteractive/the-beach/pkg/config"
)

// config is hush's env-driven knobs, loaded once at boot by the framework config
// loader (bcfg.MustLoad). Three kinds live here:
//
//   - Infrastructure the app cannot run without (Postgres via Core.DSN,
//     ClickHouse, Cloudflare R2 + Images): tagged `required`, so a missing value
//     aborts boot rather than degrading. There is no in-memory mode — the Beach
//     stack is Postgres and/or ClickHouse, always.
//   - Mailer transport (Mailgun or SMTP): the fields are individually optional
//     because you pick one transport, but main.go requires that AT LEAST ONE is
//     configured (a hush with no way to send verification/invite mail is broken).
//   - Scoring weights the app reads straight off a.cfg (EmailPoints, BaseURL),
//     and the gate-and-moderation policy values (tiers, flag triggers, invite
//     throttle, open slots, admin email) which these only SEED. That policy then
//     lives in the database and is edited live at /admin/settings; the app reads
//     it through a.live(), never off a.cfg. See settings.go.
type config struct {
	bcfg.Core // AppEnv, Port, DSN (POSTGRES_DSN|DATABASE_URL), BaseURL (BASE_URL)

	// ClickHouse is the observation store behind /admin/analytics — required.
	CHDSN string `env:"CLICKHOUSE_DSN,required"`

	// Cloudflare R2 — the durable file store for post images and application
	// photos. All four are required; R2_PUBLIC_URL is optional (unset = presigned
	// links).
	R2Endpoint  string `env:"R2_ENDPOINT,required"`
	R2Bucket    string `env:"R2_BUCKET,required"`
	R2KeyID     string `env:"R2_ACCESS_KEY_ID,required"`
	R2Secret    string `env:"R2_SECRET_ACCESS_KEY,required"`
	R2PublicURL string `env:"R2_PUBLIC_URL,trimslash" default:""`

	// Cloudflare Images — image posts upload here and serve resized variants.
	// The three credentials are required; the variant names default.
	CFImagesAccount string `env:"CF_IMAGES_ACCOUNT_ID,required"`
	CFImagesToken   string `env:"CF_IMAGES_TOKEN,required"`
	CFImagesHash    string `env:"CF_IMAGES_HASH,required"`
	CFImagesVariant string `env:"CF_IMAGES_VARIANT" default:"public"`
	CFImagesThumb   string `env:"CF_IMAGES_THUMB" default:"thumb"`

	// Mailer transport. Individually optional (you configure Mailgun OR SMTP);
	// main.go fatals if neither is set. See mailer/mailer.go.
	MailFromName  string `env:"MAIL_FROM_NAME" default:""`
	MailFromAddr  string `env:"MAIL_FROM_ADDR" default:""`
	MailgunKey    string `env:"MAILGUN_KEY" default:""`
	MailgunDomain string `env:"MAILGUN_DOMAIN" default:""`
	SMTPHost      string `env:"SMTP_HOST" default:""`
	SMTPPort      string `env:"SMTP_PORT" default:"587"`
	SMTPUsername  string `env:"SMTP_USERNAME" default:""`
	SMTPPassword  string `env:"SMTP_PASSWORD" default:""`

	// EmailPoints is the author-track reputation granted for verifying an email
	// address. Baked into the Repute engine at boot, so it stays env.
	EmailPoints float64 `env:"HUSH_EMAIL_POINTS" default:"2"`

	// AdminPassword is the first-boot password for the seeded admin account.
	// Required when RELEASE is set — a public wall must not ship "password".
	AdminPassword string `env:"HUSH_ADMIN_PASSWORD" default:""`

	// First-boot policy seeds. These only seed an empty hush_settings table; once
	// seeded the database is authoritative (edited live at /admin/settings).
	Tier1      float64 `env:"HUSH_TIER1" default:"3"`
	Tier2      float64 `env:"HUSH_TIER2" default:"8"`
	FlagTier   int     `env:"HUSH_FLAG_TIER" default:"1"`
	FlagQuorum int     `env:"HUSH_FLAG_QUORUM" default:"3"`
	InviteBase int     `env:"HUSH_INVITE_BASE" default:"3"`
	InviteMax  int     `env:"HUSH_INVITE_MAX" default:"6"`
	OpenSlots  int     `env:"HUSH_OPEN_SLOTS" default:"0"`
	AdminEmail string  `env:"HUSH_ADMIN_EMAIL" default:""`
}

// loadConfig loads and validates the environment, aborting the process with the
// list of missing required variables if the stack is not fully configured.
func loadConfig() config {
	return *bcfg.MustLoad[config]()
}

// mailerConfigured reports whether at least one mail transport is set — a
// Mailgun key (with its domain) or an SMTP host. main.go refuses to boot
// otherwise.
func (c config) mailerConfigured() bool {
	return (c.MailgunKey != "" && c.MailgunDomain != "") || c.SMTPHost != ""
}

// settingsSeed is the boot default: the env-loaded config's policy values, used
// to seed an empty hush_settings table and to backfill any key it's missing.
func settingsSeed(c config) reputation.Live {
	return reputation.Live{
		Tier1:      c.Tier1,
		Tier2:      c.Tier2,
		FlagTier:   c.FlagTier,
		FlagQuorum: c.FlagQuorum,
		InviteBase: c.InviteBase,
		InviteMax:  c.InviteMax,
		OpenSlots:  c.OpenSlots,
		AdminEmail: c.AdminEmail,
	}
}
