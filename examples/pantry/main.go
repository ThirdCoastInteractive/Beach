// Command pantry is the-beach's CRUD / apigen / auth / analytics showcase: an
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

	"github.com/ThirdCoastInteractive/the-beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ch"
	"github.com/ThirdCoastInteractive/the-beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/the-beach/pkg/passwords"
	"github.com/ThirdCoastInteractive/the-beach/pkg/pg"
	"github.com/ThirdCoastInteractive/the-beach/pkg/session"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ui/specimen"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed static
var staticFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed catalog.json locales/*.json
var localesFS embed.FS

const sessionCookie = "pantry_session"

// app holds the wired dependencies the handlers close over. pantry requires the
// full Beach stack: Postgres (inventory, auth, sessions) and ClickHouse (the
// activity firehose behind the dashboard) are always present — there is no
// database-less mode.
type app struct {
	store    *pgStore
	cat      *i18n.Catalog
	authn    *auth.Authenticator
	sessions *session.Store
	pool     *pgxpool.Pool
	chConn   ch.Conn
	events   *ch.Batcher[event]
	release  bool
}

// t translates a key against the request locale carried on ctx (set by the
// i18n middleware), using this app's catalog. Handlers call a.t rather than the
// package-level i18n.T so the pantry catalog — not the framework default — is
// consulted.
func (a *app) t(ctx context.Context, key string, args ...any) string {
	return a.cat.T(ctx, key, args...)
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("pantry: static sub-fs: %v", err)
	}

	// Load and validate config first: a missing Postgres or ClickHouse DSN aborts
	// here with the list, before anything serves. There is no database-less mode.
	conf := loadConfig()
	a := &app{release: conf.Production()}
	addr := ":" + conf.Port

	// i18n catalog from the embedded locale files; falls back to the builtin
	// default if loading fails (it shouldn't).
	cat, err := i18n.Load(localesFS, "en-US")
	if err != nil {
		log.Fatalf("pantry: i18n: %v", err)
	}
	cat.Dev(!a.release)
	a.cat = cat

	cfg := beach.Config{
		Service: "pantry",
		Release: a.release,
		Static:  sub,
		Logger:  logger,
	}

	// Postgres is required: it holds inventory, auth (login + RBAC), sessions, and
	// runs the framework's auth+session migrations plus the pantry schema. Each
	// step dies loudly rather than degrading.
	ctx := context.Background()
	pool, err := openPool(ctx, conf.DSN)
	if err != nil {
		log.Fatalf("pantry: database connect failed: %v", err)
	}
	a.pool = pool
	if err := a.migrate(ctx); err != nil {
		log.Fatalf("pantry: migrate failed: %v", err)
	}
	store, err := newPGStore(ctx, pool)
	if err != nil {
		log.Fatalf("pantry: store init failed: %v", err)
	}
	a.store = store
	a.sessions = session.NewStore(pool, session.Config{CookieName: sessionCookie})
	a.authn = auth.NewAuthenticator(pool, a.sessions, auth.LockoutConfig{})
	cfg.Sessions = a.sessions
	cfg.Principals = a.principalResolver
	if err := a.seedAdmin(ctx); err != nil {
		log.Fatalf("pantry: seed admin failed: %v", err)
	}

	// ClickHouse is required: the activity firehose behind the dashboard's
	// "Activity" line. A bad DSN or migration is fatal.
	conn, err := ch.Connect(ctx, conf.ClickhouseDSN)
	if err != nil {
		log.Fatalf("pantry: clickhouse connect failed: %v", err)
	}
	a.chConn = conn
	if err := ch.Migrate(ctx, a.chConn, chMigrations()); err != nil {
		log.Fatalf("pantry: clickhouse migrate failed: %v", err)
	}
	a.events = ch.NewBatcher[event](a.chConn, "pantry_events", ch.Batch{})

	beachApp := beach.New(cfg)
	a.routes(beachApp)

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

// openPool builds a pgx pool and pings it, returning an error so the caller can
// log a clear "database connect failed" and exit (boot has no graceful path).
func openPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// migrate runs the framework's auth+session migrations and the pantry schema.
// Each set is rooted at a "migrations" dir, as pg.Migrate expects; they live in
// separate embed.FS values so they run independently.
func (a *app) migrate(ctx context.Context) error {
	if err := pg.Migrate(ctx, a.pool, session.Migrations); err != nil {
		return err
	}
	if err := pg.Migrate(ctx, a.pool, auth.Migrations); err != nil {
		return err
	}
	return pg.Migrate(ctx, a.pool, migrationsFS)
}

// seedAdmin idempotently creates a single household-admin user (admin / password)
// with the pantry permissions, so the database-backed app is usable out of the
// box: sign in and you can manage the pantry. Safe to run on every boot.
func (a *app) seedAdmin(ctx context.Context) error {
	var uid int64
	if err := a.pool.QueryRow(ctx,
		`INSERT INTO users (username) VALUES ('admin')
		 ON CONFLICT (username) DO UPDATE SET username = EXCLUDED.username
		 RETURNING id`).Scan(&uid); err != nil {
		return err
	}
	var hasCred bool
	_ = a.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_credentials_local WHERE user_id = $1)`, uid).Scan(&hasCred)
	if !hasCred {
		hash, err := passwords.Hash("password")
		if err != nil {
			return err
		}
		if _, err := a.pool.Exec(ctx,
			`INSERT INTO user_credentials_local (user_id, password_hash) VALUES ($1, $2)`, uid, hash); err != nil {
			return err
		}
	}
	var rid int64
	if err := a.pool.QueryRow(ctx,
		`INSERT INTO roles (slug) VALUES ('household-admin')
		 ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug RETURNING id`).Scan(&rid); err != nil {
		return err
	}
	for _, perm := range []string{"pantry:read", "pantry:write", "pantry:admin"} {
		if _, err := a.pool.Exec(ctx,
			`INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`, rid, perm); err != nil {
			return err
		}
	}
	_, err := a.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, rid)
	return err
}

// routes registers every page, widget, action, and the login flow, plus the
// i18n middleware. Writes are guarded by Can("pantry:write"); location deletes
// would be Can("pantry:admin") — the auth story is visible right here.
func (a *app) routes(app *beach.App) {
	// Read pages.
	app.Page("/", a.dashboardPage)
	app.Page("/items", a.itemsPage)
	app.Page("/locations", a.locationsPage)
	app.Page("/lists", a.listsPage)
	app.Page("/login", a.loginPage)

	// The living driftwood + chart showcase, handy while restyling the app.
	app.Page("/specimen", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: specimen.Page()}, nil
	})

	// Deferred dashboard widgets — each runs a live query (CH activity firehose +
	// Postgres inventory) and returns its chart fragment (see dashboard.go).
	app.Page("/widgets/spend", anlWidget("w-spend", a.wActivity))
	app.Page("/widgets/category", anlWidget("w-category", a.wCategory))
	app.Page("/widgets/expiry", anlWidget("w-expiry", a.wExpiry))
	app.Page("/widgets/waste", anlWidget("w-waste", a.wWaste))

	// Write actions are guarded by pantry:write — the auth story stays visible in
	// the route table.
	write := []beach.Guard{app.Can("pantry:write")}
	app.Action("/items", a.createItem, write...)
	app.Action("/locations", a.createLocation, write...)

	// Login: Raw POST so it can set the cookie and issue a Datastar redirect.
	app.Raw(http.MethodPost, "/login", a.doLogin)
	app.Raw(http.MethodGet, "/logout", a.logout)
}

// principalResolver turns a session user id into a rich auth.Principal by
// reading roles+permissions back from the RBAC tables. Wired as cfg.Principals
// so c.Principal()/Can() work in guards and handlers.
func (a *app) principalResolver(ctx context.Context, userID int64) (*beach.Principal, error) {
	var username string
	if err := a.pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username); err != nil {
		return nil, err
	}
	roles, err := scanStrings(ctx, a.pool,
		`SELECT r.slug FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1 ORDER BY r.slug`, userID)
	if err != nil {
		return nil, err
	}
	perms, err := scanStrings(ctx, a.pool,
		`SELECT DISTINCT rp.permission FROM user_roles ur JOIN role_permissions rp ON rp.role_id = ur.role_id WHERE ur.user_id = $1 ORDER BY rp.permission`, userID)
	if err != nil {
		return nil, err
	}
	return &beach.Principal{UserID: userID, Username: username, Roles: roles, Permissions: perms}, nil
}

func scanStrings(ctx context.Context, pool *pgxpool.Pool, q string, args ...any) ([]string, error) {
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// logout revokes the session and clears the cookie.
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(sessionCookie); err == nil {
		_ = a.sessions.Revoke(r.Context(), ck.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// setSessionCookieW writes the session cookie on a raw response.
func (a *app) setSessionCookieW(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.release,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(14 * 24 * time.Hour),
	})
}
