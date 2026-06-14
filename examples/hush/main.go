// Command hush is the-beach's custom-layout + client-JS showcase: a gated,
// anonymous content board rendered as a gapless wall of square tiles. See
// docs/architecture/18-hush.md for the full plan.
//
// Run it as a container stack — the app, its Postgres, and its ClickHouse,
// only the web port published:
//
//	cd examples/hush
//	cp .env.example .env                  # then edit it (secrets)
//	docker compose up --build             # → http://localhost:8080
//
// Config arrives entirely from the container environment (compose's env_file +
// environment): the wall and the signup gate — applications, accounts, RBAC,
// sessions — live in Postgres; analytics in ClickHouse; media in Cloudflare R2
// + Images. All of it is required — a missing variable aborts boot rather than
// degrading. The /compose page hosts the WebGPU camera island, and Repute
// scores posting activity.
//
// This file is the thin root: load config, open the pool, run migrations,
// construct each concern (internal/store, content, reputation, media, mail,
// analytics, authx), hand them to internal/web, and serve. Every concern is an
// app-local internal package; the HTTP layer (internal/web) wires the routes.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"log/slog"
	"os"

	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/analytics"
	authx "github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/auth"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/content"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/mail"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/media"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/reputation"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/store"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/web"

	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/ThirdCoastInteractive/the-beach/pkg/mailer"
	"github.com/ThirdCoastInteractive/the-beach/pkg/pg"
)

//go:embed static
var staticFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	release := os.Getenv("RELEASE") != ""

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("hush: static sub-fs: %v", err)
	}

	// Load and validate config first: a missing required variable (Postgres,
	// ClickHouse, Cloudflare R2/Images) aborts here with the full list, before
	// anything serves. There is no degraded mode — the Beach stack is required.
	cfg := loadConfig()

	// A real mail transport is required: a hush that can't send verification or
	// invite mail is broken, not degraded. Build the strongest transport the
	// configured Mailgun/SMTP credentials support; fatal if neither is set.
	if !cfg.mailerConfigured() {
		log.Fatal("hush: no mail transport configured (set MAILGUN_KEY+MAILGUN_DOMAIN or SMTP_HOST; see mailer/mailer.go)")
	}
	mlr := mailer.New(mailer.Config{
		FromName:      cfg.MailFromName,
		FromAddr:      cfg.MailFromAddr,
		MailgunKey:    cfg.MailgunKey,
		MailgunDomain: cfg.MailgunDomain,
		SMTPHost:      cfg.SMTPHost,
		SMTPPort:      cfg.SMTPPort,
		SMTPUsername:  cfg.SMTPUsername,
		SMTPPassword:  cfg.SMTPPassword,
	})

	// Postgres is required: it holds the wall and the whole gate — applications,
	// accounts, RBAC, sessions, the live policy. There is no in-memory fallback;
	// a missing or broken database is fatal. Each step below dies loudly.
	ctx := context.Background()
	pool, err := pg.Pool(ctx, cfg.DSN)
	if err != nil {
		log.Fatalf("hush: database connect failed: %v", err)
	}

	// Auth wiring (sessions + authenticator + roles) runs the framework and app
	// migrations, then seeds the admin account.
	auth := authx.New(pool, release)
	if err := auth.Migrate(ctx, migrationsFS); err != nil {
		log.Fatalf("hush: migrate failed: %v", err)
	}

	// The wall store seeds itself on first boot, rendering the seed markdown
	// through content.RenderPost (passed in so store needn't import content).
	st, err := store.New(ctx, pool, content.RenderPost)
	if err != nil {
		log.Fatalf("hush: wall init failed: %v", err)
	}
	if err := auth.SeedAdmin(ctx); err != nil {
		log.Fatalf("hush: seed admin failed: %v", err)
	}

	// The live gate-and-moderation policy: seed it from env, then load the
	// authoritative table over the seed before anything serves.
	settings := reputation.NewSettings(pool, settingsSeed(cfg))
	if err := settings.Load(ctx, settingsSeed(cfg)); err != nil {
		log.Fatalf("hush: load settings failed: %v", err)
	}

	// Media (R2 + Cloudflare Images) and the reputation engine.
	med, err := media.New(media.Config{
		R2Endpoint:      cfg.R2Endpoint,
		R2Bucket:        cfg.R2Bucket,
		R2KeyID:         cfg.R2KeyID,
		R2Secret:        cfg.R2Secret,
		R2PublicURL:     cfg.R2PublicURL,
		CFImagesAccount: cfg.CFImagesAccount,
		CFImagesToken:   cfg.CFImagesToken,
		CFImagesHash:    cfg.CFImagesHash,
		CFImagesVariant: cfg.CFImagesVariant,
		CFImagesThumb:   cfg.CFImagesThumb,
	})
	if err != nil {
		log.Fatalf("hush: media: %v", err)
	}
	rep := reputation.New(cfg.EmailPoints)
	outbox := mail.New(pool, mlr)

	// ClickHouse: the observation store behind /admin/analytics — required, like
	// Postgres. A bad DSN or migration is fatal; there is no analytics-off mode.
	anl, err := analytics.New(ctx, cfg.CHDSN)
	if err != nil {
		log.Fatalf("hush: clickhouse init failed: %v", err)
	}

	// Construct the HTTP layer over the wired concerns.
	a := web.New(web.Deps{
		Config:    web.Config{BaseURL: cfg.BaseURL, EmailPoints: cfg.EmailPoints},
		Store:     st,
		Media:     med,
		Rep:       rep,
		Settings:  settings,
		Outbox:    outbox,
		Analytics: anl,
		Auth:      auth,
		Pool:      pool,
		Release:   release,
	})

	// chains feed the replay's echoes, so hydrate them first, then replay the
	// wall's Postgres history into the in-memory Repute engine.
	if err := a.HydrateChains(ctx); err != nil {
		log.Fatalf("hush: hydrate invite chains failed: %v", err)
	}
	if err := a.ReplayReputation(ctx); err != nil {
		log.Fatalf("hush: reputation replay failed: %v", err)
	}
	logger.Info("hush: ready", "login", "admin / password", "openSlots", settings.Live().OpenSlots)

	// The outbox pump — transactional email leaves the process only here (the
	// queue is a table). Boot's reputation replay never enqueues, so a restart
	// sends nothing twice.
	go outbox.Pump(ctx)

	beachApp := beach.New(beach.Config{
		Service:    "hush",
		Release:    release,
		Static:     sub,
		Logger:     logger,
		Sessions:   auth.Sessions,
		Principals: auth.PrincipalResolver,
	})
	a.Routes(beachApp)

	addr := ":" + cfg.Port
	logger.Info("hush: starting", "addr", addr)
	if err := beachApp.Start(addr); err != nil {
		log.Fatalf("hush: %v", err)
	}
}
