// Package authx is hush's auth wiring over the framework's pkg/auth: the pool's
// session + auth + app migrations, the role/permission seeding, the principal
// resolver, and the session cookie. It also holds the legal-assent helpers
// (agreements.go). Named authx so the local import never collides with the
// framework pkg/auth it builds on.
package authx

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/ThirdCoastInteractive/the-beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/ThirdCoastInteractive/the-beach/pkg/passwords"
	"github.com/ThirdCoastInteractive/the-beach/pkg/pg"
	"github.com/ThirdCoastInteractive/the-beach/pkg/session"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CookieName is hush's session cookie.
const CookieName = "hush_session"

// Auth wires the framework session store + authenticator over hush's pool and
// owns the role/principal plumbing. Authn and Sessions are exported so the web
// layer's login/logout handlers drive them directly.
type Auth struct {
	pool     *pgxpool.Pool
	Authn    *auth.Authenticator
	Sessions *session.Store
	release  bool
}

// New builds the session store + authenticator over the pool. release flips the
// session cookie's Secure attribute (set in production).
func New(pool *pgxpool.Pool, release bool) *Auth {
	sessions := session.NewStore(pool, session.Config{CookieName: CookieName})
	return &Auth{
		pool:     pool,
		Sessions: sessions,
		Authn:    auth.NewAuthenticator(pool, sessions, auth.LockoutConfig{}),
		release:  release,
	}
}

// Migrate runs the framework's session + auth migrations and hush's own schema,
// each in its own goose version table (see pg.Migrate). appMigrations is hush's
// embedded migrations/ FS (it lives with main.go, which embeds it).
func (a *Auth) Migrate(ctx context.Context, appMigrations fs.FS) error {
	if err := pg.Migrate(ctx, a.pool, session.Migrations); err != nil {
		return err
	}
	if err := pg.Migrate(ctx, a.pool, auth.Migrations); err != nil {
		return err
	}
	return pg.Migrate(ctx, a.pool, appMigrations)
}

// ensureRole upserts a role by slug and grants it the given permissions.
func (a *Auth) ensureRole(ctx context.Context, slug string, perms ...string) (int64, error) {
	var rid int64
	if err := a.pool.QueryRow(ctx,
		`INSERT INTO roles (slug) VALUES ($1)
		 ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug RETURNING id`, slug).Scan(&rid); err != nil {
		return 0, err
	}
	for _, p := range perms {
		if _, err := a.pool.Exec(ctx,
			`INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`, rid, p); err != nil {
			return 0, err
		}
	}
	return rid, nil
}

// SeedAdmin creates the two roles (admin: post+admin, member: post) and a single
// admin account so the gate is usable out of the box. password is the first-boot
// credential (dev default "password"); an existing hash is left alone. Idempotent.
func (a *Auth) SeedAdmin(ctx context.Context, password string) error {
	if password == "" {
		password = "password"
	}
	adminRole, err := a.ensureRole(ctx, "admin", "post", "admin")
	if err != nil {
		return err
	}
	if _, err := a.ensureRole(ctx, "member", "post"); err != nil {
		return err
	}

	var uid int64
	if err := a.pool.QueryRow(ctx,
		`INSERT INTO users (username) VALUES ('admin')
		 ON CONFLICT (username) DO UPDATE SET username = EXCLUDED.username RETURNING id`).Scan(&uid); err != nil {
		return err
	}
	var hasCred bool
	_ = a.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_credentials_local WHERE user_id = $1)`, uid).Scan(&hasCred)
	if !hasCred {
		hash, err := passwords.Hash(password)
		if err != nil {
			return err
		}
		if _, err := a.pool.Exec(ctx,
			`INSERT INTO user_credentials_local (user_id, password_hash) VALUES ($1, $2)`, uid, hash); err != nil {
			return err
		}
	}
	_, err = a.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, adminRole)
	return err
}

// PrincipalResolver turns a session user id into a Principal with its roles and
// permissions, wired as cfg.Principals so c.Principal()/Can() work.
func (a *Auth) PrincipalResolver(ctx context.Context, userID int64) (*beach.Principal, error) {
	var username string
	if err := a.pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username); err != nil {
		return nil, err
	}
	roles, err := ScanStrings(ctx, a.pool,
		`SELECT r.slug FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1 ORDER BY r.slug`, userID)
	if err != nil {
		return nil, err
	}
	perms, err := ScanStrings(ctx, a.pool,
		`SELECT DISTINCT rp.permission FROM user_roles ur JOIN role_permissions rp ON rp.role_id = ur.role_id WHERE ur.user_id = $1 ORDER BY rp.permission`, userID)
	if err != nil {
		return nil, err
	}
	return &beach.Principal{UserID: userID, Username: username, Roles: roles, Permissions: perms}, nil
}

// ScanStrings runs a single-column text query and collects the rows.
func ScanStrings(ctx context.Context, pool *pgxpool.Pool, q string, args ...any) ([]string, error) {
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

// SetSessionCookie writes hush's session cookie. Secure rides the release flag.
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
