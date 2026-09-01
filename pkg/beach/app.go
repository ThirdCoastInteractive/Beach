// Package beach is Beach's HTTP layer: the framework's front door. It is the
// module root (package beach) rather than a sub-package — services build their
// main.go against beach.New and the three typed handler shapes.
//
// The builder collapses the boilerplate every service would otherwise repeat: a
// fixed middleware stack (request log, recover, security headers + CSP, optional
// session auth), GET /healthz, static file serving with boot-time SHA256 ETags
// and immutable cache headers, and graceful shutdown. Routes register against
// typed handler shapes — PageFunc, ActionFunc, StreamFunc — that encode the
// house rules by construction, with guards (Authed, Role, Can) that make the
// auth story visible in the route table.
//
// Echo is named in the architecture doc, but the frozen go.mod ships no web
// framework, so the implementation is stdlib net/http with the Go 1.22 ServeMux
// pattern router underneath. App.Mux() and App.Raw are the escape hatches for
// the genuinely weird (payment webhooks, plain http.Handler middleware).
package beach

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
	"github.com/ThirdCoastInteractive/Beach/pkg/theme"
)

// Config configures a new App. The zero value is usable: every field worth
// setting has a sensible default.
type Config struct {
	// Service names the running service (used in logs and the default Server
	// header). e.g. "live".
	Service string

	// Release reports whether this is a released (production) binary. It selects
	// stable commit-based asset versioning over dev timestamps, hardens cookies,
	// and silences the dev page-size warnings.
	Release bool

	// CSP is the Content-Security-Policy preset applied by the secure-headers
	// middleware. A zero CSP falls back to CSPDefault().
	CSP CSP

	// Static is the filesystem served under /static (with boot-time ETags and
	// immutable cache headers). When nil, the framework serves the embedded
	// internal/view/static tree (app.css, the Datastar client, and the chart
	// modules).
	Static fs.FS

	// Sessions, when non-nil, enables session-backed auth: OptionalAuth runs in
	// the middleware stack and guards (Authed/Role) resolve against it. Left nil,
	// the app runs without a database and every principal is anonymous (nil),
	// unless AnonymousSessions is set.
	Sessions *session.Store

	// AnonymousSessions, when non-nil, enables a cookie-only anonymous identity:
	// the middleware mints a stable per-browser id (no database) and c.Principal()
	// returns a well-defined anonymous principal carrying it, instead of nil. It is
	// the DB-less path for a fully-ephemeral app that wants a stable visitor
	// identity without minting its own cookie. The anonymous principal holds no
	// roles or permissions, so guards (Authed/Role/Can) still reject it. Ignored
	// when Sessions is set — a DB-backed app resolves real identities there.
	AnonymousSessions *session.AnonStore

	// Principals, when non-nil, resolves a rich auth.Principal from the
	// session-bound user id. Optional; without it c.User() still works from the
	// session and c.Principal() is nil.
	Principals PrincipalResolver

	// Tokens, when non-nil, resolves a rich auth.Principal from a raw bearer
	// token. It backs the TokenAuthed() guard for non-interactive callers
	// (service accounts, CLIs). Without it TokenAuthed() rejects every request,
	// which is the safe failure.
	Tokens TokenResolver

	// Theme pins the design tokens this app serves, overriding the preset the
	// framework ships. Nil falls through to ThemeSource, then to the preset.
	Theme *theme.Theme

	// ThemeSource resolves the theme at boot and on App.ReloadTheme — the hook
	// for an app that stores its palette rather than compiling it in. Ignored
	// when Theme is set.
	//
	// A resolving *function* rather than a table the framework owns: the
	// framework owns tables for auth and sessions and nothing else, and this way
	// an app can back its theme with a row, a file or an env var, exactly as it
	// already does for principals.
	ThemeSource ThemeSource

	// Hub is the in-memory topic fan-out backing StreamFunc subscriptions. A nil
	// hub disables live events (catch-up still runs); set one to serve realtime
	// streams.
	Hub *hub.Hub

	// Healthz is the readiness probe behind GET /healthz. A nil probe always
	// reports healthy (200 ok) — fine for a database-less app.
	Healthz HealthFunc

	// Logger is the structured logger used by the request-logging and recover
	// middleware. Defaults to slog.Default().
	Logger *slog.Logger

	// ShutdownTimeout bounds graceful shutdown. Defaults to 15s.
	ShutdownTimeout time.Duration

	// SSECompression overrides the default SSE compression mode for StreamFunc /
	// ActionFunc responses. The zero value is the gzip default.
	SSECompression SSEMode

	// Sockets tunes the WebSocket machinery behind App.Socket (message size
	// cap, keepalive cadence, write-queue depth, allowed origins). The zero
	// value applies the documented defaults; same-origin only.
	Sockets SocketConfig

	// Locales, when non-nil, resolves each request's locale (cookie, then
	// Accept-Language, then the catalog default) and carries it and the catalog
	// on the request context. Two things then follow without any further
	// wiring: i18n.T resolves this app's strings, and the driftwood page shell
	// declares the matching <html lang> and dir — which is what a screen reader
	// reads to pick its voice and pronunciation rules (WCAG 3.1.1).
	//
	// New also registers it with i18n.SetDefault, so i18n.T resolves this app's
	// strings *off* a request too — inside a hub.Ticker, a background job, any
	// render with no request context to carry the catalog. Without that the
	// promise above holds only on the request path, and a fragment pushed over
	// SSE renders its own catalog keys as literal text.
	//
	// Left nil, the app runs monolingual: i18n.T falls back to the framework's
	// embedded catalog, the shell declares lang="en", and the feature costs
	// nothing. The middleware runs outermost, before request logging, so a
	// logged or recovered request already knows its locale.
	Locales *i18n.Catalog

	// Middleware are app-supplied net/http wrappers applied around the whole
	// handler, outside the fixed framework stack. They run outermost-first: the
	// first entry is the outermost wrapper, so it sees the request first and the
	// response last. A wrapper here runs before request logging, recover, secure
	// headers and auth — the place for cross-cutting concerns (an i18n locale
	// resolver, a custom request id) the framework does not own. Left nil, the
	// handler is exactly the fixed stack, unchanged.
	Middleware []func(http.Handler) http.Handler
}

// HealthFunc is the readiness probe behind /healthz. Returning an error renders
// a 503; nil renders 200.
type HealthFunc func(ctx context.Context) error

// PrincipalResolver turns a session user id into a rich principal. It is called
// once per authenticated request when Config.Principals is set. Returning a nil
// principal (and nil error) leaves the request anonymous at the principal level
// while still carrying the session user.
type PrincipalResolver func(ctx context.Context, userID int64) (*Principal, error)

// TokenResolver verifies a raw bearer token and returns the principal it
// resolves to. It backs the TokenAuthed() guard. A non-nil error (or a nil
// principal) means the token is invalid; the guard rejects with 401. It wraps
// auth.Authenticator.ResolveToken.
type TokenResolver func(ctx context.Context, token string) (*Principal, error)

// App is a configured Beach application. Build one with New, register routes
// with Page/Action/Stream/Raw, then Start. The zero App is not usable.
type App struct {
	cfg     Config
	mux     *http.ServeMux
	log     *slog.Logger
	static  *staticHandler
	hub     *hub.Hub
	srv     *http.Server
	sockets socketRegistry
	// theme holds the rendered token block. An atomic swap means a reload never
	// hands a request half a palette.
	theme atomic.Value // *renderedTheme
}

// New builds an App from cfg. It wires the static handler (computing boot-time
// ETags) and registers GET /healthz and the static tree. It does not bind a
// socket; that happens in Start.
func New(cfg Config) *App {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 15 * time.Second
	}
	if cfg.CSP.IsZero() {
		cfg.CSP = CSPDefault()
	}

	// The app's catalog becomes i18n's package default, so a render with no
	// request context behind it — a hub.Ticker fanning a fragment, a background
	// job — still resolves this app's strings rather than emitting raw keys.
	// SetDefault is documented as a boot-time call, which is exactly here.
	if cfg.Locales != nil {
		i18n.SetDefault(cfg.Locales)
	}

	a := &App{
		cfg: cfg,
		mux: http.NewServeMux(),
		log: cfg.Logger,
		hub: cfg.Hub,
	}

	// Static tree, overlaid framework-first: the framework's canonical assets
	// (app.css, the Datastar client, the chart modules) always serve and take
	// precedence, then the app's own static (images, etc.). Apps never copy the
	// stylesheet or datastar.js — they get the framework copies here.
	staticFS := fs.FS(embeddedStatic())
	if cfg.Static != nil {
		staticFS = overlayFS{layers: []fs.FS{embeddedStatic(), cfg.Static}}
	}
	sh, err := newStaticHandler(staticFS, cfg.Release)
	if err != nil {
		// A broken static tree is a boot-time programming error.
		panic(fmt.Sprintf("beach: static handler: %v", err))
	}
	a.static = sh

	a.mux.Handle("GET /static/", http.StripPrefix("/static/", sh))
	a.mux.HandleFunc("GET /healthz", a.handleHealthz)
	// The framework owns the preference route because it owns the two things it
	// switches off: the stream adapter honours the live-updates pause, and the
	// kit's toast reads the auto-dismiss flag. An app gets both Level A
	// mechanisms by rendering one component (see driftwood.LiveToggle).
	a.mux.HandleFunc("POST "+PrefsPath, a.handlePrefs)

	// The design tokens. Derived once here rather than per request: it is a few
	// hundred microseconds, which is nothing at boot and real money on a
	// framework that budgets pages in kilobytes.
	if err := a.loadTheme(context.Background()); err != nil {
		// A configured source that cannot resolve at boot is a misconfiguration
		// worth failing loudly for, but not worth taking the app down over —
		// the shipped preset is a working palette, and a site with the default
		// colours beats a site that will not start.
		a.log.Error("beach: theme source failed at boot; serving the shipped preset", "err", err)
		a.cfg.Theme, a.cfg.ThemeSource = nil, nil
		if err := a.loadTheme(context.Background()); err != nil {
			panic("beach: shipped theme preset does not derive: " + err.Error())
		}
	}
	a.mux.HandleFunc("GET "+ThemePath, a.handleTheme)

	return a
}

// Mux exposes the underlying ServeMux for advanced wiring (mounting a sub-router,
// a third-party handler). Prefer Page/Action/Stream/Raw; this is the escape hatch.
func (a *App) Mux() *http.ServeMux { return a.mux }

// Logger returns the app's structured logger.
func (a *App) Logger() *slog.Logger { return a.log }

// AssetURL returns the cache-busted URL for a static asset path (e.g.
// "css/app.css" -> "/static/css/app.css?v=..."). Templates call this so
// every deploy busts caches without a manifest.
func (a *App) AssetURL(path string) string {
	return a.static.assetURL(path)
}

// handleHealthz runs the readiness probe. /healthz is skipped by the request
// logger so probes do not flood the log.
func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Healthz != nil {
		if err := a.cfg.Healthz(r.Context()); err != nil {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handler returns the fully wrapped root handler: the route mux behind the fixed
// middleware stack. It is built fresh by Start (and by Handler for tests).
func (a *App) handler() http.Handler {
	var h http.Handler = a.mux

	// Innermost first; outermost wraps last so it runs first on the way in.
	// Order on the way in: requestLog -> recover -> secureHeaders -> prefs -> auth.
	h = a.injectAuth(h)
	h = a.prefsMiddleware(h)
	h = a.secureHeaders(h)
	h = a.recoverPanic(h)
	h = a.requestLog(h)

	// App-supplied middleware wraps outside the fixed stack, outermost-first: the
	// first entry ends up the outermost wrapper, so it runs first on the way in.
	for i := len(a.cfg.Middleware) - 1; i >= 0; i-- {
		if mw := a.cfg.Middleware[i]; mw != nil {
			h = mw(h)
		}
	}

	// Locale resolution wraps everything, so every layer inside — app
	// middleware included — sees a request that already knows its language.
	if a.cfg.Locales != nil {
		h = a.cfg.Locales.Middleware(h)
	}
	return h
}

// Handler returns the app as a single http.Handler with the full middleware
// stack applied. Useful for httptest and for embedding the app in another server.
func (a *App) Handler() http.Handler {
	return a.handler()
}

// Start binds addr and serves until an interrupt (SIGINT/SIGTERM), then shuts
// down gracefully within ShutdownTimeout.
func (a *App) Start(addr string) error {
	a.srv = &http.Server{
		Addr:              addr,
		Handler:           a.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		a.log.Info("beach: listening", "service", a.cfg.Service, "addr", addr)
		if err := a.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errc:
		return err
	case sig := <-stop:
		a.log.Info("beach: shutting down", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		return fmt.Errorf("beach: graceful shutdown: %w", err)
	}
	return nil
}

// Shutdown triggers graceful shutdown: live WebSockets get close 1001 (going
// away, their contexts cancel), the HTTP server drains normal requests, and
// socket handlers are waited on until ctx expires. Hijacked connections are
// invisible to http.Server.Shutdown, which is why the sockets close first and
// are waited on explicitly.
func (a *App) Shutdown(ctx context.Context) error {
	a.sockets.closeAll()
	var err error
	if a.srv != nil {
		err = a.srv.Shutdown(ctx)
	}
	a.sockets.wait(ctx)
	return err
}

// pathIsExempt reports whether p should skip request logging (probes and static
// assets are noise).
func pathIsExempt(p string) bool {
	return p == "/healthz" || strings.HasPrefix(p, "/static/")
}
