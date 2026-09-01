// Package authx is askmuskie's thin auth wiring over the framework's pkg/auth:
// the session store, the authenticator, the principal resolver, and the session
// cookie. askmuskie has no RBAC needs (chat/customize gate on "logged in", not
// on permissions), so there is no role seeding — registration just creates a
// user, and a session carries an empty role set.
package authx

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CookieName is askmuskie's session cookie.
const CookieName = "am_session"

// Auth bundles the session store + authenticator over the pool. Authn and
// Sessions are exported so the web handlers drive login/logout directly.
type Auth struct {
	pool     *pgxpool.Pool
	Authn    *auth.Authenticator
	Sessions *session.Store
	release  bool
}

// New builds the session store + authenticator over the pool. release flips the
// session cookie's Secure attribute on in production.
func New(pool *pgxpool.Pool, release bool) *Auth {
	sessions := session.NewStore(pool, session.Config{CookieName: CookieName})
	return &Auth{
		pool:     pool,
		Sessions: sessions,
		Authn:    auth.NewAuthenticator(pool, sessions, auth.LockoutConfig{}),
		release:  release,
	}
}

// Migrate runs the framework's session + auth migrations and askmuskie's own
// schema, each in its own goose version table (see pg.Migrate).
func (a *Auth) Migrate(ctx context.Context, appMigrations fs.FS) error {
	if err := pg.Migrate(ctx, a.pool, session.Migrations); err != nil {
		return err
	}
	if err := pg.Migrate(ctx, a.pool, auth.Migrations); err != nil {
		return err
	}
	return pg.Migrate(ctx, a.pool, appMigrations)
}

// PrincipalResolver turns a session user id into a Principal (username + empty
// roles/permissions), wired as beach.Config.Principals so c.Principal() works.
func (a *Auth) PrincipalResolver(ctx context.Context, userID int64) (*beach.Principal, error) {
	var username string
	if err := a.pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username); err != nil {
		return nil, err
	}
	return &beach.Principal{UserID: userID, Username: username}, nil
}

// SetSessionCookie writes askmuskie's session cookie. Secure rides the release flag.
func (a *Auth) SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.release,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(14 * 24 * time.Hour),
	})
}

// ClearSessionCookie expires the session cookie (logout).
func (a *Auth) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.release,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
