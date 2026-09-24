// Command booking-manager is Beach's small-operator showcase: a
// self-hostable manager for a handful of seasonal rentals (cottages,
// cabins, getaways). Guest intake from a public inquiry form works a
// pipeline into dated bookings on a per-property calendar; confirmation
// notifies the guest by email (pkg/mailer) and text (pkg/sms) and programs a
// per-stay door code through the smart-lock boundary (internal/locks). Around
// the stays sits the rest of a tiny operation: property details with standing
// key codes, a hiring pipeline feeding staff, shift scheduling, a time clock,
// and supply inventory with par levels — behind two roles (owner vs staff)
// and the framework's living component showcase at /specimen.
//
// Run it as a container stack — the app and its Postgres:
//
//	cd cmd/examples/booking-manager
//	cp .env.example .env
//	docker compose up                     # → http://localhost:8080  (admin / password)
//
// Postgres is the only requirement and the system of record (properties,
// bookings, staffing, inventory, auth + RBAC, sessions) — the point of the
// app is that a seasonal operator can run the whole thing themselves. Mail,
// SMS, and the smart lock are all optional: unconfigured they fall back to
// log transports, so a fresh checkout "delivers" to the terminal. Run outside
// compose with:
//
//	DATABASE_URL=postgres://user:pass@localhost/booking ./bin/booking-manager
package main

import (
	"context"
	"embed"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/mailer"
	"github.com/ThirdCoastInteractive/Beach/pkg/passwords"
	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
	"github.com/ThirdCoastInteractive/Beach/pkg/sms"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/locks"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/notify"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/web"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Load and validate config first: a missing Postgres DSN aborts here with
	// the list, before anything serves. There is no database-less mode.
	conf := loadConfig()
	release := conf.Production()
	addr := ":" + conf.Port

	cfg := beach.Config{
		Service: "booking-manager",
		Release: release,
		Logger:  logger,
	}

	// Postgres is required: it holds the whole operation plus auth (login +
	// RBAC) and sessions, and runs the framework's auth+session migrations
	// ahead of the booking schema. Each step dies loudly rather than degrading.
	ctx := context.Background()
	pool, err := pg.Pool(ctx, conf.DSN)
	if err != nil {
		log.Fatalf("booking-manager: database connect failed: %v", err)
	}
	if err := migrate(ctx, pool); err != nil {
		log.Fatalf("booking-manager: migrate failed: %v", err)
	}
	st, err := store.New(ctx, pool)
	if err != nil {
		log.Fatalf("booking-manager: store init failed: %v", err)
	}
	sessions := session.NewStore(pool, session.Config{CookieName: web.SessionCookie})
	authn := auth.NewAuthenticator(pool, sessions, auth.LockoutConfig{})
	cfg.Sessions = sessions
	if err := seedAdmin(ctx, pool); err != nil {
		log.Fatalf("booking-manager: seed admin failed: %v", err)
	}

	// Guest messaging and the smart lock pick their transports from config:
	// Mailgun/SMTP and Twilio in production, log transports on a fresh
	// checkout. The lock boundary has no hardware implementation yet — the
	// log provider narrates what a real one would program.
	mlr := mailer.New(mailer.Config{
		FromName: conf.MailFromName, FromAddr: conf.MailFromAddr,
		MailgunKey: conf.MailgunKey, MailgunDomain: conf.MailgunDomain,
		SMTPHost: conf.SMTPHost, SMTPPort: conf.SMTPPort,
		SMTPUsername: conf.SMTPUsername, SMTPPassword: conf.SMTPPassword,
	})
	txts := sms.New(sms.Config{
		FromNumber:                conf.SMSFromNumber,
		TwilioAccountSID:          conf.TwilioAccountSID,
		TwilioAuthToken:           conf.TwilioAuthToken,
		TwilioMessagingServiceSID: conf.TwilioMessagingSID,
	})
	guests := notify.New(mlr, txts, logger)
	var lock locks.Provider = &locks.LogProvider{}

	// Construct the web app (the DI container the handlers hang off) and wire
	// its routes + principal resolver onto the framework app.
	a := web.New(st, authn, sessions, pool, guests, lock, release)
	cfg.Principals = a.PrincipalResolver

	beachApp := beach.New(cfg)
	a.Routes(beachApp)

	logger.Info("booking-manager: starting", "addr", addr, "login", "admin / password")
	srv := &http.Server{Addr: addr, Handler: beachApp.Handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("booking-manager: %v", err)
	}
}

// migrate runs the framework's auth+session migrations and the booking
// schema. Each set is rooted at a "migrations" dir, as pg.Migrate expects;
// they live in separate embed.FS values so they run independently.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pg.Migrate(ctx, pool, session.Migrations); err != nil {
		return err
	}
	if err := pg.Migrate(ctx, pool, auth.Migrations); err != nil {
		return err
	}
	return pg.Migrate(ctx, pool, migrationsFS)
}

// seedAdmin idempotently creates a single owner user (admin / password) so
// the app is usable out of the box: sign in and you run the whole operation.
// Safe to run on every boot.
func seedAdmin(ctx context.Context, pool *pgxpool.Pool) error {
	var uid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username) VALUES ('admin')
		 ON CONFLICT (username) DO UPDATE SET username = EXCLUDED.username
		 RETURNING id`).Scan(&uid); err != nil {
		return err
	}
	var hasCred bool
	_ = pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_credentials_local WHERE user_id = $1)`, uid).Scan(&hasCred)
	if !hasCred {
		hash, err := passwords.Hash("password")
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO user_credentials_local (user_id, password_hash) VALUES ($1, $2)`, uid, hash); err != nil {
			return err
		}
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id)
		 SELECT $1, id FROM roles WHERE slug = 'owner'
		 ON CONFLICT DO NOTHING`, uid)
	return err
}
