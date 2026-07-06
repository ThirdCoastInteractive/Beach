package web

import (
	"context"
	"net/http"

	authx "github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/auth"

	fwauth "github.com/ThirdCoastInteractive/the-beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ui/specimen"
)

// Routes registers the wall, the post action, image upload, and file serving,
// then the gate, admin, analytics, and email routes.
func (a *App) Routes(app *beach.App) {
	app.Page("/", a.home)
	app.Page("/compose", a.compose)
	app.Page("/p/{id}", a.postView)
	// Datastar-only feed batches, fetched by the intersect sentinels; a direct
	// navigation 404s (no Page on the View).
	app.Page("/feed/newer", a.feedMore(true))
	app.Page("/feed/older", a.feedMore(false))
	app.Page("/apply", a.applyPage)
	app.Page("/login", a.loginPage)
	app.Page("/u/{handle}", a.profileView)
	app.Page("/terms", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: termsPage(a.live().AdminEmail)}, nil
	})

	// The framework's living showcase — every driftwood component and chart
	// type with sample data — mounted so a hush checkout doubles as a styling
	// reference.
	app.Page("/specimen", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: specimen.Page()}, nil
	})

	// Posting is gated on the "post" permission — the gate is always on (Postgres
	// is required), so every write route carries the guard.
	postGuard := []beach.Guard{app.Can("post")}
	app.Action("/posts", a.createPost, postGuard...)
	app.Action("/p/{id}/vote", a.votePost, postGuard...)
	app.Action("/p/{id}/flag", a.flagPost, postGuard...)
	app.Raw(http.MethodPost, "/upload", a.uploadImage, postGuard...)

	// Admin: the application queue, plus the suite (admin.go) — the hub, the
	// moderation review, the everything browser, the member ledger, and the
	// one moderation action their forms post to.
	adminGuard := []beach.Guard{app.Can("admin")}
	app.Page("/admin/queue", a.queuePage, adminGuard...)
	app.Raw(http.MethodPost, "/admin/decide", a.doDecide, adminGuard...)
	app.Page("/admin", a.adminHub, adminGuard...)
	app.Page("/admin/review", a.adminReview, adminGuard...)
	app.Page("/admin/posts", a.adminPosts, adminGuard...)
	app.Page("/admin/members", a.adminMembers, adminGuard...)
	app.Page("/admin/outbox", a.adminOutbox, adminGuard...)
	app.Raw(http.MethodPost, "/admin/moderate", a.doModerate, adminGuard...)
	// Toggling a member's admin role revokes their sessions (admin.go), so a
	// demotion takes hold on their next request rather than at cookie expiry.
	app.Raw(http.MethodPost, "/admin/role", a.doMemberRole, adminGuard...)
	// The live policy editor: read on every page through a.live(), edited here
	// and effective on the next request, no restart.
	app.Page("/admin/settings", a.adminSettings, adminGuard...)
	app.Raw(http.MethodPost, "/admin/settings", a.doSaveSettings, adminGuard...)

	// Admin analytics: the dashboard page plus its deferred widget fragments —
	// every route behind the same admin guard, since the charts and tables show
	// everything the wall hides (an unguarded IP firehose would be a leak).
	app.Page("/admin/analytics", a.analyticsPage, adminGuard...)
	app.Page("/admin/analytics/events", anlWidget("anl-events", a.anlEvents), adminGuard...)
	app.Page("/admin/analytics/posts", anlWidget("anl-posts", a.anlPosts), adminGuard...)
	app.Page("/admin/analytics/votes", anlWidget("anl-votes", a.anlVotes), adminGuard...)
	app.Page("/admin/analytics/countries", anlWidget("anl-countries", a.anlCountries), adminGuard...)
	app.Page("/admin/analytics/ips", anlWidget("anl-ips", a.anlIPs), adminGuard...)
	app.Page("/admin/analytics/recent", anlWidget("anl-recent", a.anlRecent), adminGuard...)

	// Application + auth (plain form posts → 303).
	app.Raw(http.MethodPost, "/apply", a.doApply)
	app.Raw(http.MethodPost, "/login", a.doLogin)
	app.Raw(http.MethodGet, "/logout", a.logout)

	// Member email (verify.go): the verification link is public — it arrives
	// by mail, signed in or not; the set/resend forms check the session inside.
	app.Page("/email/verify/{token}", a.verifyEmailPage)
	app.Raw(http.MethodPost, "/email/set", a.doSetEmail)
	app.Raw(http.MethodPost, "/email/resend", a.doResendEmail)

	// The other doors (invites.go): capped open registration and invite
	// redemption — both create accounts directly; minting is member-gated.
	app.Raw(http.MethodPost, "/register", a.doRegister)
	app.Page("/invite/{code}", a.invitePage)
	app.Raw(http.MethodPost, "/invite/{code}", a.doRedeem)
	app.Raw(http.MethodPost, "/invites", a.doMint, postGuard...)
}

// Login/logout (the auth handlers) — plain form posts. loginPage maps ?err to
// a banner; doLogin authenticates and sets the cookie; logout revokes it.

// loginPage maps the ?err code to a banner message and renders the sign-in form.
func (a *App) loginPage(c *beach.Ctx) (beach.View, error) {
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
func (a *App) doLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token, _, _, err := a.auth.Authn.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		// The failed handle is signal, not identity — brute-force runs show up
		// as one handle hammered from one IP.
		a.anl.Track(r, "login_fail", r.FormValue("username"), 0)
		dest := "/login?err=1"
		if err == fwauth.ErrAccountLocked {
			dest = "/login?err=locked"
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	a.anl.Track(r, "login_ok", r.FormValue("username"), 0)
	a.auth.SetSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// logout revokes the session and clears the cookie.
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	// Read the handle before the revoke, while the principal still resolves.
	handle, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	a.anl.Track(r, "logout", handle, 0)
	if ck, err := r.Cookie(authx.CookieName); err == nil {
		_ = a.auth.Sessions.Revoke(r.Context(), ck.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: authx.CookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ReplayReputation rebuilds the in-memory Repute engine from Postgres at boot:
// every post emits "post", every reaction emits "like"/"dislike" (plus the
// half-weight chain echo to the author's inviter — HydrateChains must have run
// first), and every verified member email emits "email-verified".
func (a *App) ReplayReputation(ctx context.Context) error {
	authors, err := authx.ScanStrings(ctx, a.pool, `SELECT author FROM hush_posts ORDER BY id`)
	if err != nil {
		return err
	}
	for _, author := range authors {
		a.rep.Emit("post", author)
	}

	rows, err := a.pool.Query(ctx,
		`SELECT p.author, r.weight FROM hush_reactions r
		 JOIN hush_posts p ON p.id = r.post_id ORDER BY r.created_at, r.post_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var author string
		var weight int16
		if err := rows.Scan(&author, &weight); err != nil {
			return err
		}
		kind, chainKind := "like", "chain-like"
		if weight < 0 {
			kind, chainKind = "dislike", "chain-dislike"
		}
		a.rep.Emit(kind, author)
		if inviter := a.inviterOf(author); inviter != "" {
			a.rep.Emit(chainKind, inviter)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	verified, err := authx.ScanStrings(ctx, a.pool,
		`SELECT u.username FROM hush_member_emails e
		 JOIN users u ON u.id = e.user_id WHERE e.verified_at IS NOT NULL`)
	if err != nil {
		return err
	}
	for _, handle := range verified {
		a.rep.Emit("email-verified", handle)
	}
	return nil
}

// HydrateChains is the exported boot hook main calls.
func (a *App) HydrateChains(ctx context.Context) error { return a.hydrateChains(ctx) }
