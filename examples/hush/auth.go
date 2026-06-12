package main

import (
	"context"
	"net/http"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/auth"
	"github.com/ThirdCoastInteractive/the-beach/passwords"
	"github.com/ThirdCoastInteractive/the-beach/pg"
	"github.com/ThirdCoastInteractive/the-beach/session"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "hush_session"

// openPool builds a pool and pings it, returning an error (not panicking) so a
// bad DATABASE_URL degrades to database-less mode instead of crashing.
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

// migrate runs the framework's session + auth migrations and hush's own schema,
// each in its own goose version table (see pg.Migrate).
func (a *app) migrate(ctx context.Context) error {
	if err := pg.Migrate(ctx, a.pool, session.Migrations); err != nil {
		return err
	}
	if err := pg.Migrate(ctx, a.pool, auth.Migrations); err != nil {
		return err
	}
	return pg.Migrate(ctx, a.pool, migrationsFS)
}

// ensureRole upserts a role by slug and grants it the given permissions.
func (a *app) ensureRole(ctx context.Context, slug string, perms ...string) (int64, error) {
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

// seedAdmin creates the two roles (admin: post+admin, member: post) and a single
// admin account (admin / password) so the gate is usable out of the box: sign in
// as admin to approve applicants. Idempotent.
func (a *app) seedAdmin(ctx context.Context) error {
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
		hash, err := passwords.Hash("password")
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

// principalResolver turns a session user id into a Principal with its roles and
// permissions, wired as cfg.Principals so c.Principal()/Can() work.
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

// acct summarizes the viewer: their handle (empty if not signed in), whether they
// may post, and whether they are an admin. Database-less mode is open: anyone can
// post (a single-user wall) and there is no admin.
func (a *app) acct(c *beach.Ctx) (handle string, canPost, admin bool) {
	if a.authn == nil {
		return "", true, false
	}
	if p, ok := c.Principal(); ok && p != nil {
		return p.Username, p.Can("post"), p.Can("admin")
	}
	return "", false, false
}

// loginPage maps the ?err code to a banner message and renders the sign-in
// form (loginFormPage in pages.templ), or a notice without a DB.
func (a *app) loginPage(c *beach.Ctx) (beach.View, error) {
	if a.authn == nil {
		return beach.View{Page: noticePage("sign-in needs the database.", "set DATABASE_URL to enable accounts.")}, nil
	}
	errMsg := ""
	switch c.Query("err") {
	case "1":
		errMsg = "invalid handle or password."
	case "locked":
		errMsg = "account locked. try again later."
	}
	return beach.View{Page: loginFormPage(errMsg)}, nil
}

// doLogin is a plain form POST: authenticate, set the session cookie, 303 home.
func (a *app) doLogin(w http.ResponseWriter, r *http.Request) {
	if a.authn == nil {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	token, _, _, err := a.authn.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		// The failed handle is signal, not identity — brute-force runs show up
		// as one handle hammered from one IP.
		a.track(r, "login_fail", r.FormValue("username"), 0)
		dest := "/login?err=1"
		if err == auth.ErrAccountLocked {
			dest = "/login?err=locked"
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	a.track(r, "login_ok", r.FormValue("username"), 0)
	a.setSessionCookieW(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// logout revokes the session and clears the cookie.
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	// Read the handle before the revoke, while the principal still resolves.
	handle, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	a.track(r, "logout", handle, 0)
	if a.sessions != nil {
		if ck, err := r.Cookie(sessionCookie); err == nil {
			_ = a.sessions.Revoke(r.Context(), ck.Value)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

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
