package main

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/the-beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/ThirdCoastInteractive/the-beach/pkg/passwords"

	"github.com/ThirdCoastInteractive/the-beach/cmd/askmuskie/internal/authx"
)

// authHandlers bundles the deps the auth routes need. Wired only when a database
// is present. The forms are plain POSTs: on success we set the session cookie and
// 303-redirect home; on failure we redirect home with an error code that
// reopens the modal (no inline script, so it works under the strict CSP).
type authHandlers struct {
	auth *authx.Auth
	pool *pgxpool.Pool
}

func (h *authHandlers) register(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	username := r.FormValue("username")
	password := r.FormValue("password")
	if r.FormValue("agree") == "" {
		redirectAuthErr(w, r, "register", "terms")
		return
	}
	ctx := r.Context()
	uid, err := auth.RegisterUser(ctx, h.pool, username, password)
	if err != nil {
		redirectAuthErr(w, r, "register", registerErrCode(err))
		return
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO askmuskie_players (user_id, tos_accepted_at) VALUES ($1, now())
		 ON CONFLICT (user_id) DO UPDATE SET tos_accepted_at = now()`, uid); err != nil {
		redirectAuthErr(w, r, "register", "generic")
		return
	}
	token, _, _, err := h.auth.Authn.Login(ctx, username, password)
	if err != nil {
		redirectAuthErr(w, r, "login", "generic")
		return
	}
	h.auth.SetSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *authHandlers) login(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token, _, _, err := h.auth.Authn.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		code := "badcreds"
		if errors.Is(err, auth.ErrAccountLocked) {
			code = "locked"
		}
		redirectAuthErr(w, r, "login", code)
		return
	}
	h.auth.SetSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *authHandlers) logout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(authx.CookieName); err == nil && ck.Value != "" {
		_ = h.auth.Sessions.Revoke(r.Context(), ck.Value)
	}
	h.auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// tos serves the terms page.
func (h *authHandlers) tos(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: tosPage()}, nil
}

// redirectAuthErr bounces home with an error code + which tab to reopen.
func redirectAuthErr(w http.ResponseWriter, r *http.Request, tab, code string) {
	q := url.Values{"ae": {code}, "at": {tab}}
	http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
}

func registerErrCode(err error) string {
	switch {
	case errors.Is(err, auth.ErrUsernameTaken):
		return "taken"
	case errors.Is(err, auth.ErrInvalidUsername):
		return "invalid"
	case errors.Is(err, passwords.ErrPasswordTooShort):
		return "short"
	case errors.Is(err, passwords.ErrPasswordTooLong):
		return "long"
	default:
		return "generic"
	}
}

// authErrMessage maps an error code to a user-facing message ("" = no error).
func authErrMessage(code string) string {
	switch code {
	case "taken":
		return "That name is taken."
	case "invalid":
		return "Name must be 3–32 letters, digits, _ or -."
	case "short":
		return "Password must be at least 8 characters."
	case "long":
		return "That password is too long."
	case "badcreds":
		return "Wrong username or password."
	case "locked":
		return "Too many attempts — try again in a bit."
	case "terms":
		return "Please accept the terms to register."
	case "generic":
		return "Something went wrong — try again."
	default:
		return ""
	}
}
