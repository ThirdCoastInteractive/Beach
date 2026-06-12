// Package beach is the-beach's HTTP layer: the framework's front door. It is the
// module root (package beach) rather than a sub-package — services build their
// main.go against beach.New and the three typed handler shapes.
//
// The builder collapses the boilerplate every service would otherwise repeat: a
// fixed middleware stack (request log, recover, security headers + CSP, optional
// session auth, kit injection), GET /healthz, static file serving with boot-time
// SHA256 ETags and immutable cache headers, and graceful shutdown. Routes
// register against typed handler shapes — PageFunc, ActionFunc, StreamFunc —
// that encode the house rules by construction, with guards (Authed, Role, Can)
// that make the auth story visible in the route table.
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
	"syscall"
	"time"

	"github.com/ThirdCoastInteractive/the-beach/hub"
	"github.com/ThirdCoastInteractive/the-beach/session"
	"github.com/ThirdCoastInteractive/the-beach/ui"
)

// Config configures a new App. The zero value is usable: it yields a kit-less
// build that refuses to start (a kit is mandatory), so every field worth setting
// has a sensible default and the only hard requirement is a UI kit.
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
	// internal/view/static tree (the kit stylesheet and the Datastar client).
	Static fs.FS

	// Kit selects the UI kit injected into every request context, surfaced to
	// handlers as c.Kit(). Use KitStatic(name) or the NoopKit fallback. A nil
	// Kit means "use the registered mandatory kit"; if none is registered the
	// build refuses to Start.
	Kit KitSelector

	// Sessions, when non-nil, enables session-backed auth: OptionalAuth runs in
	// the middleware stack and guards (Authed/Role) resolve against it. Left nil,
	// the app runs without a database and every principal is anonymous (nil).
	Sessions *session.Store

	// Principals, when non-nil, resolves a rich auth.Principal from the
	// session-bound user id. Optional; without it c.User() still works from the
	// session and c.Principal() is nil.
	Principals PrincipalResolver

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
}

// HealthFunc is the readiness probe behind /healthz. Returning an error renders
// a 503; nil renders 200.
type HealthFunc func(ctx context.Context) error

// PrincipalResolver turns a session user id into a rich principal. It is called
// once per authenticated request when Config.Principals is set. Returning a nil
// principal (and nil error) leaves the request anonymous at the principal level
// while still carrying the session user.
type PrincipalResolver func(ctx context.Context, userID int64) (*Principal, error)

// App is a configured the-beach application. Build one with New, register routes
// with Page/Action/Stream/Raw, then Start. The zero App is not usable.
type App struct {
	cfg    Config
	mux    *http.ServeMux
	log    *slog.Logger
	static *staticHandler
	kit    ui.Kit
	hub    *hub.Hub
	srv    *http.Server
}

// New builds an App from cfg. It wires the static handler (computing boot-time
// ETags), resolves the UI kit, and registers GET /healthz and the static tree.
// It does not bind a socket; that happens in Start. A missing kit is reported at
// Start, not here, so tests can construct an App without registering a kit.
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

	a := &App{
		cfg: cfg,
		mux: http.NewServeMux(),
		log: cfg.Logger,
		hub: cfg.Hub,
	}

	// Resolve the kit eagerly so a misconfigured kit fails at build, not on the
	// first request. A nil selector defers to the registered mandatory kit.
	a.kit = resolveKit(cfg.Kit)

	// Static tree, overlaid framework-first: the framework's canonical assets
	// (the kit's app.css and the Datastar client) always serve and take
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

	return a
}

// Mux exposes the underlying ServeMux for advanced wiring (mounting a sub-router,
// a third-party handler). Prefer Page/Action/Stream/Raw; this is the escape hatch.
func (a *App) Mux() *http.ServeMux { return a.mux }

// Kit returns the resolved UI kit. Mostly for tests and diagnostics; handlers
// read the kit from the request context via c.Kit().
func (a *App) Kit() ui.Kit { return a.kit }

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
	// Order on the way in: requestLog -> recover -> secureHeaders -> auth -> kit.
	h = a.injectKit(h)
	h = a.injectAuth(h)
	h = a.secureHeaders(h)
	h = a.recoverPanic(h)
	h = a.requestLog(h)
	return h
}

// Handler returns the app as a single http.Handler with the full middleware
// stack applied. Useful for httptest and for embedding the app in another server.
// It panics if no UI kit is available, matching Start's mandatory-kit rule.
func (a *App) Handler() http.Handler {
	if a.kit == nil {
		panic("beach: no UI kit registered — set Config.Kit or import a kit package")
	}
	return a.handler()
}

// Start binds addr and serves until an interrupt (SIGINT/SIGTERM), then shuts
// down gracefully within ShutdownTimeout. It refuses to start if no UI kit is
// available — component lookup can never silently fall through to a blank page.
func (a *App) Start(addr string) error {
	if a.kit == nil {
		return errors.New("beach: refusing to start — no UI kit registered " +
			"(set Config.Kit, or import a kit package for its init() registration)")
	}

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
	if err := a.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("beach: graceful shutdown: %w", err)
	}
	return nil
}

// Shutdown triggers graceful shutdown of a running server (started via Start).
// It is a no-op if the server was never started.
func (a *App) Shutdown(ctx context.Context) error {
	if a.srv == nil {
		return nil
	}
	return a.srv.Shutdown(ctx)
}

// resolveKit turns a KitSelector into a concrete kit. A nil selector means "use
// the registered mandatory kit" — resolved lazily so an app that registers its
// kit via a blank import still works. resolveKit never panics: a missing kit is
// reported by Start.
func resolveKit(sel KitSelector) ui.Kit {
	if sel != nil {
		return sel()
	}
	// Defer to the registry's mandatory kit if one is registered. Mandatory()
	// panics when none exists, so guard it.
	if len(ui.Registered()) == 0 {
		return nil
	}
	return ui.Mandatory()
}

// pathIsExempt reports whether p should skip request logging (probes and static
// assets are noise).
func pathIsExempt(p string) bool {
	return p == "/healthz" || strings.HasPrefix(p, "/static/")
}
