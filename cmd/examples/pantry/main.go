// Command pantry is Beach's CRUD / apigen / auth / analytics showcase: an
// at-home grocery ERP. Household inventory (items with photos, quantities,
// expiry), storage locations, and shopping lists, behind two roles
// (household-admin vs household-member); a dashboard of deferred chart widgets
// (spend line, category stacked bar, expiry calendar heatmap, waste gauge); a
// full i18n catalog (en-US, es-ES); and the framework's living component
// showcase mounted at /specimen.
//
// Run it as a container stack — the app, its Postgres, and its ClickHouse:
//
//	cd examples/pantry
//	cp .env.example .env
//	docker compose up                     # → http://localhost:8080
//
// pantry requires the full Beach stack: Postgres is the system of record
// (inventory, auth + RBAC, sessions) and ClickHouse is the activity firehose
// behind the dashboard. Both DSNs are required — a missing one aborts boot with
// the list, there is no database-less mode. Run outside compose with both set:
//
//	DATABASE_URL=postgres://user:pass@localhost/pantry \
//	CLICKHOUSE_DSN=clickhouse://default:@localhost:9000/pantry ./bin/pantry
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ch"
	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/Beach/pkg/passwords"
	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/analytics"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/store"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/web"
)

//go:embed static
var staticFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed catalog.json locales/*.json
var localesFS embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("pantry: static sub-fs: %v", err)
	}

	// Load and validate config first: a missing Postgres or ClickHouse DSN aborts
	// here with the list, before anything serves. There is no database-less mode.
	conf := loadConfig()
	release := conf.Production()
	addr := ":" + conf.Port

	// i18n catalog from the embedded locale files; falls back to the builtin
	// default if loading fails (it shouldn't).
	cat, err := i18n.Load(localesFS, "en-US")
	if err != nil {
		log.Fatalf("pantry: i18n: %v", err)
	}
	cat.Dev(!release)

	cfg := beach.Config{
		Service: "pantry",
		Release: release,
		Static:  sub,
		Logger:  logger,
	}

	// Postgres is required: it holds inventory, auth (login + RBAC), sessions, and
	// runs the framework's auth+session migrations plus the pantry schema. Each
	// step dies loudly rather than degrading.
	ctx := context.Background()
	pool, err := pg.Pool(ctx, conf.DSN)
	if err != nil {
		log.Fatalf("pantry: database connect failed: %v", err)
	}
	if err := migrate(ctx, pool); err != nil {
		log.Fatalf("pantry: migrate failed: %v", err)
	}
	st, err := store.New(ctx, pool)
	if err != nil {
		log.Fatalf("pantry: store init failed: %v", err)
	}
	sessions := session.NewStore(pool, session.Config{CookieName: web.SessionCookie})
	authn := auth.NewAuthenticator(pool, sessions, auth.LockoutConfig{})
	cfg.Sessions = sessions
	if err := seedAdmin(ctx, pool); err != nil {
		log.Fatalf("pantry: seed admin failed: %v", err)
	}

	// ClickHouse is required: the activity firehose behind the dashboard's
	// "Activity" line. A bad DSN or migration is fatal.
	conn, err := ch.Connect(ctx, conf.ClickhouseDSN)
	if err != nil {
		log.Fatalf("pantry: clickhouse connect failed: %v", err)
	}
	if err := ch.Migrate(ctx, conn, analytics.Migrations()); err != nil {
		log.Fatalf("pantry: clickhouse migrate failed: %v", err)
	}
	anl := analytics.New(conn)

	// Construct the web app (the DI container the handlers hang off) and wire its
	// routes + principal resolver onto the framework app.
	a := web.New(st, cat, authn, sessions, pool, anl, release)
	cfg.Principals = a.PrincipalResolver

	beachApp := beach.New(cfg)
	a.Routes(beachApp)

	// Serve via our own server so the i18n middleware can wrap the framework's
	// full handler stack (App.Handler()) at the edge — it resolves the request
	// locale (cookie/Accept-Language) onto the context before any page renders.
	handler := cat.Middleware(beachApp.Handler())

	logger.Info("pantry: starting", "addr", addr, "login", "admin / password")
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("pantry: %v", err)
	}
}

// migrate runs the framework's auth+session migrations and the pantry schema.
// Each set is rooted at a "migrations" dir, as pg.Migrate expects; they live in
// separate embed.FS values so they run independently.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pg.Migrate(ctx, pool, session.Migrations); err != nil {
		return err
	}
	if err := pg.Migrate(ctx, pool, auth.Migrations); err != nil {
		return err
	}
	return pg.Migrate(ctx, pool, migrationsFS)
}

// seedAdmin idempotently creates a single household-admin user (admin / password)
// with the pantry permissions, so the database-backed app is usable out of the
// box: sign in and you can manage the pantry. Safe to run on every boot.
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
	var rid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO roles (slug) VALUES ('household-admin')
		 ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug RETURNING id`).Scan(&rid); err != nil {
		return err
	}
	for _, perm := range []string{"pantry:read", "pantry:write", "pantry:admin"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`, rid, perm); err != nil {
			return err
		}
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, rid)
	return err
}
