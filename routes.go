package beach

import (
	"net/http"
)

// Route registration. Each typed shape registers under its natural HTTP method
// (Page/Stream: GET, Action: POST) and a ServeMux pattern. Guards are applied as
// ordered wrappers so the auth story is visible at the registration site, not
// buried in the handler — which is the point of putting them in the route table.

// Guard wraps a handler with an access check. It runs after the auth middleware
// has populated the request context, so it can read the user/principal; on
// failure it writes the mapped error (401/403) and the wrapped handler never
// runs. Authed/Role/Can return Guards.
type Guard func(http.Handler) http.Handler

// Page registers a PageFunc at GET pattern. Tabs, filters, pagination and sort
// reuse this URL with query params — they never get their own routes. Optional
// guards wrap it outermost-first.
func (a *App) Page(pattern string, h PageFunc, guards ...Guard) {
	a.register(http.MethodGet, pattern, a.adaptPage(h), guards)
}

// Action registers an ActionFunc at POST pattern. Actions are Datastar-only; a
// plain navigation gets a 404 from the adapter.
func (a *App) Action(pattern string, h ActionFunc, guards ...Guard) {
	a.register(http.MethodPost, pattern, a.adaptAction(h), guards)
}

// Stream registers a StreamFunc at GET pattern. The framework runs the SSE loop;
// the handler only declares topics and catch-up.
func (a *App) Stream(pattern string, h StreamFunc, guards ...Guard) {
	a.register(http.MethodGet, pattern, a.adaptStream(h), guards)
}

// Raw is the documented escape hatch: a plain http.HandlerFunc mounted at
// method+pattern, outside the typed shapes. Payment webhooks live here — they
// must mount outside auth and the Datastar discipline to run reliably. Guards may
// still wrap a raw handler when wanted.
func (a *App) Raw(method, pattern string, h http.HandlerFunc, guards ...Guard) {
	a.register(method, pattern, adaptRaw(h), guards)
}

// register installs handler at "METHOD pattern" on the mux, wrapping it with the
// guards (outermost-first so the first guard listed runs first).
func (a *App) register(method, pattern string, handler http.HandlerFunc, guards []Guard) {
	var h http.Handler = handler
	for i := len(guards) - 1; i >= 0; i-- {
		h = guards[i](h)
	}
	a.mux.Handle(method+" "+pattern, h)
}

// --- guards ---

// Authed requires an authenticated session: a request without one gets 401 and
// the handler never runs. A PageFunc behind Authed() can assume c.User() is
// present.
func (a *App) Authed() Guard {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := a.newCtx(w, r)
			if _, ok := c.User(); !ok {
				a.writeError(c, ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Role requires the session to carry the given role slug: 401 when
// unauthenticated, 403 when authenticated but lacking the role.
func (a *App) Role(role string) Guard {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := a.newCtx(w, r)
			u, ok := c.User()
			if !ok {
				a.writeError(c, ErrUnauthorized)
				return
			}
			if !u.HasRole(role) {
				a.writeError(c, ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Can requires the principal to hold the given "resource:action" permission. It
// needs a configured PrincipalResolver (Config.Principals); without one no
// request has a principal and the guard always 403s, which is the safe failure.
func (a *App) Can(permission string) Guard {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := a.newCtx(w, r)
			if _, ok := c.User(); !ok {
				a.writeError(c, ErrUnauthorized)
				return
			}
			p, _ := c.Principal()
			if !p.Can(permission) {
				a.writeError(c, ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
