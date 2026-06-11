// Command pantry is the-beach's CRUD / apigen / auth / analytics showcase: an
// at-home grocery ERP. Household inventory (items with photos, quantities,
// expiry), storage locations, and shopping lists, behind two roles
// (household-admin vs household-member); a dashboard of deferred chart widgets
// (spend line, category stacked bar, expiry calendar heatmap, waste gauge); and
// a full i18n catalog (en-US, es-ES).
//
// Run it:
//
//	go build -o bin/pantry ./examples/pantry
//	PORT=8080 ./bin/pantry
//
// Without a database the binary still serves: the dashboard, inventory,
// locations, and shopping-list pages render from an in-memory store seeded at
// boot, and the home route needs no DB at all. Set DATABASE_URL to a Postgres
// DSN to enable auth (login, RBAC-gated writes) and the persistent tables; boot
// then runs the framework's auth+session migrations and this app's pantry
// migration.
//
//	DATABASE_URL=postgres://user:pass@localhost/pantry PORT=8080 ./bin/pantry
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

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/auth"
	"github.com/ThirdCoastInteractive/the-beach/i18n"
	"github.com/ThirdCoastInteractive/the-beach/pg"
	"github.com/ThirdCoastInteractive/the-beach/session"
	_ "github.com/ThirdCoastInteractive/the-beach/ui/driftwood" // registers the "driftwood" kit

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed static
var staticFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed catalog.json locales/*.json
var localesFS embed.FS

const sessionCookie = "pantry_session"

// app holds the wired dependencies the handlers close over. authn/sessions/pool
// are nil when no DATABASE_URL is configured (database-less mode).
type app struct {
	store    *memStore
	cat      *i18n.Catalog
	authn    *auth.Authenticator
	sessions *session.Store
	pool     *pgxpool.Pool
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

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("pantry: static sub-fs: %v", err)
	}

	a := &app{store: newMemStore(), release: os.Getenv("RELEASE") != ""}

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
		Kit:     beach.KitStatic("driftwood"),
		Logger:  logger,
	}

	// Optional Postgres: enables auth (login + RBAC) and the persistent schema.
	// Without it the app runs database-less on the in-memory store.
	ctx := context.Background()
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pool, perr := openPool(ctx, dsn)
		if perr != nil {
			logger.Error("pantry: database connect failed; running database-less", "err", perr)
		} else {
			a.pool = pool
			if err := a.migrate(ctx); err != nil {
				logger.Error("pantry: migrate failed; running database-less", "err", err)
				pool.Close()
				a.pool = nil
			} else {
				a.sessions = session.NewStore(pool, session.Config{CookieName: sessionCookie})
				a.authn = auth.NewAuthenticator(pool, a.sessions, auth.LockoutConfig{})
				cfg.Sessions = a.sessions
				cfg.Principals = a.principalResolver
				logger.Info("pantry: database mode (auth enabled)")
			}
		}
	}

	beachApp := beach.New(cfg)
	a.routes(beachApp)

	// Serve via our own server so the i18n middleware can wrap the framework's
	// full handler stack (App.Handler()) at the edge — it resolves the request
	// locale (cookie/Accept-Language) onto the context before any page renders.
	handler := cat.Middleware(beachApp.Handler())

	logger.Info("pantry: starting", "addr", addr, "db", a.pool != nil)
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("pantry: %v", err)
	}
}

// openPool builds a pgx pool and pings it, returning an error (rather than
// panicking like pg.MustPool) so a misconfigured DATABASE_URL degrades to
// database-less mode instead of crashing the binary.
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

// routes registers every page, widget, action, and the login flow, plus the
// i18n middleware. Writes are guarded by Can("pantry:write"); location deletes
// would be Can("pantry:admin") — the auth story is visible right here.
func (a *app) routes(app *beach.App) {
	// Read pages — all work without a DB.
	app.Page("/", a.dashboardPage)
	app.Page("/items", a.itemsPage)
	app.Page("/locations", a.locationsPage)
	app.Page("/lists", a.listsPage)
	app.Page("/login", a.loginPage)

	// Deferred dashboard widgets (fed by sample analytics data).
	app.Page("/widgets/spend", widgetPage("w-spend", func(*beach.Ctx) string { return spendSVG() }))
	app.Page("/widgets/category", widgetPage("w-category", func(*beach.Ctx) string { return categorySVG() }))
	app.Page("/widgets/expiry", widgetPage("w-expiry", func(c *beach.Ctx) string { return expirySVG(a.store.listItems()) }))
	app.Page("/widgets/waste", widgetPage("w-waste", func(*beach.Ctx) string { return wasteSVG() }))

	// Write actions — guarded by the pantry:write permission. Without a
	// PrincipalResolver (no DB) the guard denies everyone, which is the safe
	// read-only degradation.
	app.Action("/items", a.createItem, app.Can("pantry:write"))
	app.Action("/locations", a.createLocation, app.Can("pantry:write"))

	// Login: Raw POST so it can set the cookie and issue a Datastar redirect.
	app.Raw(http.MethodPost, "/login", a.doLogin)
	app.Raw(http.MethodGet, "/logout", a.logout)
}

// principalResolver turns a session user id into a rich auth.Principal by
// reading roles+permissions back from the RBAC tables. Wired as cfg.Principals
// when the DB is present, so c.Principal()/Can() work in guards and handlers.
func (a *app) principalResolver(ctx context.Context, userID int64) (*beach.Principal, error) {
	if a.pool == nil {
		return nil, nil
	}
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
	if a.sessions != nil {
		if ck, err := r.Cookie(sessionCookie); err == nil {
			_ = a.sessions.Revoke(r.Context(), ck.Value)
		}
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
