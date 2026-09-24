// Package web is pantry's HTTP surface: the app DI container plus every page,
// widget, action, and the login flow. Handlers and views are methods on the app
// struct so they share one set of wired dependencies without mass-exporting or
// import cycles; main.go constructs the app and registers its routes.
package web

import (
	"context"
	"net/http"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/specimen"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/analytics"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/store"
)

// SessionCookie is the name of pantry's session cookie.
const SessionCookie = "pantry_session"

// app holds the wired dependencies the handlers close over. pantry requires the
// full Beach stack: Postgres (inventory, auth, sessions) and ClickHouse (the
// activity firehose behind the dashboard) are always present — there is no
// database-less mode.
type app struct {
	store    *store.Store
	cat      *i18n.Catalog
	authn    *auth.Authenticator
	sessions *session.Store
	pool     *pgxpool.Pool
	anl      *analytics.Analytics
	release  bool
}

// New wires the pantry web app from its already-constructed dependencies. It is
// the DI container the handlers and views hang off; main.go builds the stores
// and passes them in.
func New(st *store.Store, cat *i18n.Catalog, authn *auth.Authenticator, sessions *session.Store, pool *pgxpool.Pool, anl *analytics.Analytics, release bool) *app {
	return &app{
		store:    st,
		cat:      cat,
		authn:    authn,
		sessions: sessions,
		pool:     pool,
		anl:      anl,
		release:  release,
	}
}

// t translates a key against the request locale carried on ctx (set by the
// i18n middleware), using this app's catalog. Handlers call a.t rather than the
// package-level i18n.T so the pantry catalog — not the framework default — is
// consulted.
func (a *app) t(ctx context.Context, key string, args ...any) string {
	return a.cat.T(ctx, key, args...)
}

// Routes registers every page, widget, action, and the login flow. Writes are
// guarded by Can("pantry:write"); location deletes would be Can("pantry:admin")
// — the auth story is visible right here.
func (a *app) Routes(app *beach.App) {
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

// PrincipalResolver turns a session user id into a rich auth.Principal by
// reading roles+permissions back from the RBAC tables. Wired as cfg.Principals
// so c.Principal()/Can() work in guards and handlers.
func (a *app) PrincipalResolver(ctx context.Context, userID int64) (*beach.Principal, error) {
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
	if ck, err := r.Cookie(SessionCookie); err == nil {
		_ = a.sessions.Revoke(r.Context(), ck.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// setSessionCookieW writes the session cookie on a raw response.
func (a *app) setSessionCookieW(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.release,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(14 * 24 * time.Hour),
	})
}
