package beach

import (
	"context"
	"net/http"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
)

// Principal is re-exported so handler signatures and guard call sites name a
// single beach.Principal rather than reaching into the auth package. It is the
// resolved identity for an authenticated request (user id, username, roles,
// flattened permissions).
type Principal = auth.Principal

// User is the small session identity (id + role slugs). A handler usually reads
// the richer Principal, but User is always available when a session is present.
type User = session.User

// Ctx is the request-scoped context the typed handlers receive. It wraps the
// http.ResponseWriter and *http.Request and exposes the session user and the
// auth principal, plus the small helpers (path/query params, binding) the
// house handlers lean on. Construct one only through the framework; handlers
// receive *Ctx, never build it.
type Ctx struct {
	W http.ResponseWriter
	R *http.Request

	app *App
}

// newCtx builds a Ctx for a request. Internal: the typed-handler adapters call it.
func (a *App) newCtx(w http.ResponseWriter, r *http.Request) *Ctx {
	return &Ctx{W: w, R: r, app: a}
}

// Context returns the request's context.Context, for cancellation and deadlines.
func (c *Ctx) Context() context.Context { return c.R.Context() }

// User returns the session identity and whether one is present. Anonymous
// requests return the zero User and false. Behind a guard (Authed/Role/Can) the
// user is guaranteed present, so guarded handlers can ignore the bool.
func (c *Ctx) User() (User, bool) {
	return session.UserFrom(c.R.Context())
}

// MustUser returns the session user, assuming a guard guaranteed it. It returns
// the zero User when unauthenticated; use it only inside guarded handlers.
func (c *Ctx) MustUser() User {
	u, _ := session.UserFrom(c.R.Context())
	return u
}

// Principal returns the rich auth principal and whether one is present. It is nil
// for an unauthenticated request and for apps that configured no
// PrincipalResolver. With Config.AnonymousSessions it is the cookie-only
// anonymous principal (IsAnonymous() true) instead of nil. A nil principal
// answers false to every Can/HasRole check, so handlers can call methods on it
// without a nil guard.
func (c *Ctx) Principal() (*Principal, bool) {
	return auth.PrincipalFrom(c.R.Context())
}

// MustPrincipal returns the principal (possibly nil). The *Principal methods are
// nil-safe, so this is convenient inside guarded handlers.
func (c *Ctx) MustPrincipal() *Principal {
	p, _ := auth.PrincipalFrom(c.R.Context())
	return p
}

// AnonID returns the stable cookie-only anonymous session id and whether one is
// present. It is set by Config.AnonymousSessions; an app without it (or behind a
// real login) has no anonymous id. Apps key ephemeral per-visitor state on it.
func (c *Ctx) AnonID() (string, bool) {
	return session.AnonFrom(c.R.Context())
}

// IsDatastar reports whether the request came from Datastar's fetch client. The
// PageFunc adapter uses this to choose full-document vs. fragment; handlers that
// branch manually can read it too.
func (c *Ctx) IsDatastar() bool { return datastar.IsDatastar(c.R) }

// Param returns a path parameter captured by the ServeMux pattern (e.g. the
// "{id}" segment of "/coins/purchase/{id}"). Missing params return "".
func (c *Ctx) Param(name string) string { return c.R.PathValue(name) }

// Query returns a URL query parameter (the first value), or "" when absent. Tabs,
// filters, pagination, and sort live in query params on the page URL — they never
// get their own routes.
func (c *Ctx) Query(name string) string { return c.R.URL.Query().Get(name) }

// FormValue returns a POST/PUT form value (parsing the body on first use).
func (c *Ctx) FormValue(name string) string { return c.R.FormValue(name) }

// Header returns a request header value.
func (c *Ctx) Header(name string) string { return c.R.Header.Get(name) }

// SetHeader sets a response header. Useful for cache directives on a fragment
// response or a redirect target.
func (c *Ctx) SetHeader(name, value string) { c.W.Header().Set(name, value) }

// SetCookie writes a Set-Cookie response header. An action that mints a session
// calls this before returning its Patches (the response headers are flushed when
// the SSE stream opens, after the handler body runs), so a Patch{Redirect: ...}
// can carry the cookie on the same response — the post-login bounce without a Raw
// handler.
func (c *Ctx) SetCookie(cookie *http.Cookie) { http.SetCookie(c.W, cookie) }

// Redirect writes an HTTP redirect to the given location with the given status
// (use http.StatusSeeOther for POST-redirect-GET). For Datastar requests prefer
// returning a Patches that updates the URL; this is the plain-navigation form.
func (c *Ctx) Redirect(status int, location string) {
	http.Redirect(c.W, c.R, location, status)
}
