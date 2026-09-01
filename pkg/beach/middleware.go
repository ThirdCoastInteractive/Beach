package beach

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
)

// The fixed middleware stack, in the order requests traverse it on the way in:
//
//	requestLog -> recover -> secureHeaders -> auth -> route
//
// Each is a plain net/http wrapper. The order is fixed by App.handler(); apps do
// not reorder it. Auth is the stage that populates the request context the typed
// handlers read back via *Ctx.

// statusRecorder captures the response status for the request log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

// Flush forwards to the underlying writer's flusher so SSE streaming keeps
// working through the recorder (StreamFunc relies on per-event flushes).
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying writer (the http.ResponseController pattern)
// so a WebSocket upgrade can reach the http.Hijacker through the recorder.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// requestLog logs one line per request at completion, skipping probes and static
// assets so they do not flood the log.
func (a *App) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pathIsExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		a.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"dur", time.Since(start).String(),
		)
	})
}

// recoverPanic turns a handler panic into a 500 and logs the stack, so one bad
// handler never takes the process down.
func (a *App) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				a.log.Error("panic recovered",
					"path", r.URL.Path,
					"panic", v,
					"stack", string(debug.Stack()),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// secureHeaders sets the security header preset and the CSP. Values are
// conservative defaults appropriate for an SSR app; the CSP comes from the
// builder's preset.
func (a *App) secureHeaders(next http.Handler) http.Handler {
	csp := a.cfg.CSP.String()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if a.cfg.Release {
			// HSTS only in release: a dev box on plain http must not be pinned.
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if a.cfg.Service != "" {
			h.Set("Server", a.cfg.Service)
		}
		next.ServeHTTP(w, r)
	})
}

// injectAuth attaches the session user (and, when a resolver is configured, the
// rich principal) to the request context. It is optional: without a session
// store the app runs database-free and every request is anonymous. Guards
// (Authed/Role/Can) enforce; this stage only populates.
func (a *App) injectAuth(next http.Handler) http.Handler {
	if a.cfg.Sessions == nil {
		// No DB-backed store. A cookie-only anonymous store still gives every
		// request a well-defined anonymous principal (a stable id, no privileges);
		// without one the app is fully anonymous (nil principal). Guards reject
		// either way.
		if a.cfg.AnonymousSessions != nil {
			return a.injectAnonymous(next)
		}
		return next
	}
	store := a.cfg.Sessions
	resolve := a.cfg.Principals
	// OptionalAuth attaches session.User to the context when a valid cookie is
	// present. We then optionally lift it to a Principal.
	return store.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if resolve != nil {
			if u, ok := session.UserFrom(r.Context()); ok {
				if p, err := resolve(r.Context(), u.ID); err == nil && p != nil {
					r = r.WithContext(auth.WithPrincipal(r.Context(), p))
				}
			}
		}
		next.ServeHTTP(w, r)
	}))
}

// injectAnonymous wraps the cookie-only anonymous store: AnonStore.Ensure mints
// or reads the stable per-browser id, then this attaches the explicit anonymous
// principal carrying it so c.Principal() is well-defined (not nil) without a
// database. The anonymous principal holds no roles or permissions, so guards
// still reject it.
func (a *App) injectAnonymous(next http.Handler) http.Handler {
	anon := a.cfg.AnonymousSessions
	return anon.Ensure(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := session.AnonFrom(r.Context()); ok {
			r = r.WithContext(auth.WithPrincipal(r.Context(), auth.AnonymousPrincipal(id)))
		}
		next.ServeHTTP(w, r)
	}))
}
