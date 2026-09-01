package beach

import (
	"net/http"
	"strings"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
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
//
// Page("/") is the home page, an exact match — not ServeMux's catch-all (see
// register). For a true catch-all, register a "/{path...}" wildcard or use Mux().
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

// Socket registers a SocketFunc at GET pattern (WebSocket upgrades ride a GET).
// Guards run before the upgrade, so a rejection is a plain HTTP 401/403, never
// a half-open socket. The socket carries payloads that are not hypermedia —
// UI sync stays on Stream; a page wanting both keeps them side by side.
func (a *App) Socket(pattern string, h SocketFunc, guards ...Guard) {
	a.register(http.MethodGet, pattern, a.adaptSocket(h), guards)
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
//
// The bare "/" pattern means "the home page", so it registers as the exact-match
// "/{$}" — never ServeMux's match-everything subtree. Otherwise every stray path
// (a browser's /favicon.ico, a scanner's /wp-login.php) would render the home
// page and pollute whatever the handler tracks. Unknown paths 404.
func (a *App) register(method, pattern string, handler http.HandlerFunc, guards []Guard) {
	if pattern == "/" {
		pattern = "/{$}"
	}
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
// needs a principal in context — from a session PrincipalResolver
// (Config.Principals) or from TokenAuthed() for a bearer caller. With neither, no
// request has a principal and the guard 401s an anonymous request or 403s an
// authenticated one lacking the permission, which is the safe failure.
func (a *App) Can(permission string) Guard {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := a.newCtx(w, r)
			_, hasUser := c.User()
			p, hasPrincipal := c.Principal()
			if !hasUser && !hasPrincipal {
				a.writeError(c, ErrUnauthorized)
				return
			}
			if !p.Can(permission) {
				a.writeError(c, ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TokenAuthed requires a valid bearer token: it reads the Authorization header,
// resolves the token to a principal via the configured TokenResolver
// (Config.Tokens), and attaches that principal to the request context — the same
// principal a session login would carry, so Can(...) and InScope(...) compose on
// top. A missing or invalid token (or no configured resolver) gets 401 and the
// handler never runs.
func (a *App) TokenAuthed() Guard {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := a.newCtx(w, r)
			resolve := a.cfg.Tokens
			if resolve == nil {
				a.writeError(c, ErrUnauthorized)
				return
			}
			token, ok := bearerToken(r)
			if !ok {
				a.writeError(c, ErrUnauthorized)
				return
			}
			p, err := resolve(r.Context(), token)
			if err != nil || p == nil {
				a.writeError(c, ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
		})
	}
}

// InScope requires the principal to act within the given owner/customer scope.
// It registers on a route exactly like Can(...): an anonymous, unscoped, or
// wrong-scope principal is rejected with 403 before the handler body runs. The
// scope string is opaque to Beach — the app decides what a scope means.
func (a *App) InScope(scope string) Guard {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := a.newCtx(w, r)
			p, _ := c.Principal()
			if !p.InScope(scope) {
				a.writeError(c, ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts the credential from an "Authorization: Bearer <token>"
// header. ok is false when the header is absent or not a Bearer credential.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
