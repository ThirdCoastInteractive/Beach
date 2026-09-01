package web

import (
	"net/http"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
)

// loginPage renders the sign-in form (shell.templ loginView).
func (a *app) loginPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	return beach.View{Page: a.loginView(authed, c.Query("err"))}, nil
}

// doLogin handles POST /login. It is a Raw handler (not an ActionFunc)
// because login is a plain form POST (progressive enhancement): read the form
// fields, then redirect with a 303. No SSE, no inline script — the strict CSP
// forbids the SDK's script-based redirect, and a real navigation is what
// login wants.
func (a *app) doLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token, _, _, err := a.authn.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		dest := "/login?err=1"
		if err == auth.ErrAccountLocked {
			dest = "/login?err=locked"
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	a.setSessionCookieW(w, token)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
