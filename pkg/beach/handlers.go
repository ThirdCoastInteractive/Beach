package beach

import (
	"context"
	"errors"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/coder/websocket"

	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
)

// The four typed handler shapes. Each is a function from *Ctx to a typed result
// plus an error; the framework adapts each to an http.HandlerFunc, owning the
// branch (PageFunc), the method/Datastar discipline (ActionFunc), the SSE
// loop (StreamFunc), and the upgrade + connection lifecycle (SocketFunc).
// Handlers stay standalone factory functions that take only their deps — never
// methods on a server struct.

// PageFunc handles anything you can navigate to. It returns one View that knows
// how to render both ways: the framework writes the full document on a normal GET
// and patches the fragment on a Datastar request. You cannot forget the branch,
// and a separate fragment route has nothing to attach to.
type PageFunc func(c *Ctx) (View, error)

// ActionFunc handles a Datastar-only mutation. Navigation to an action route is a
// 404; the Datastar-Request check and method discipline are the framework's. The
// handler does the mutation and returns the patches to apply, which the framework
// streams over a one-shot SSE response.
type ActionFunc func(c *Ctx) (Patches, error)

// StreamFunc declares an SSE subscription. It returns a Sub (topics + optional
// catch-up); the framework runs the loop — subscribe to the hub, replay the
// since-cursor catch-up, select over the hub channel and the request context,
// flush per event, and clean up on disconnect.
type StreamFunc func(c *Ctx) (Sub, error)

// SocketFunc owns an upgraded WebSocket connection for its lifetime. It runs
// after a successful upgrade (guards run before, so a rejection is plain HTTP,
// never a half-open socket) and holds the connection for as long as it runs.
// Returning nil closes with 1000 (normal); returning an unexpected error
// closes with 1011 and logs. A peer close, canceled context, or ErrSocketClosed
// returned from the read loop counts as normal.
type SocketFunc func(c *Ctx, s *Socket) error

// --- adapters ---

// adaptPage turns a PageFunc into an http.HandlerFunc that performs the
// dual-purpose branch.
func (a *App) adaptPage(h PageFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := a.newCtx(w, r)
		view, err := h(c)
		if err != nil {
			a.writeError(c, err)
			return
		}
		if c.IsDatastar() {
			a.streamView(c, view)
			return
		}
		a.writePage(c, view)
	}
}

// writePage renders the full document for a navigation.
func (a *App) writePage(c *Ctx, view View) {
	if view.Page == nil {
		// A View with no Page on navigation is a programming error; surface it
		// rather than writing an empty 200.
		a.writeError(c, ErrNotFound)
		return
	}
	status := view.Status
	if status == 0 {
		status = http.StatusOK
	}
	c.W.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.W.WriteHeader(status)
	if err := view.Page.Render(c.Context(), c.W); err != nil {
		a.log.Error("page render", "path", c.R.URL.Path, "err", err)
	}
}

// streamView patches a View's fragment (or its page) over a one-shot SSE
// response, the Datastar branch of a PageFunc.
func (a *App) streamView(c *Ctx, view View) {
	frag := view.Fragment
	if frag == nil {
		frag = view.Page
	}
	if frag == nil {
		a.writeError(c, ErrNotFound)
		return
	}
	sse := datastar.NewSSE(c.W, c.R, datastar.WithCompression(a.cfg.SSECompression))
	sp := streamPatcher{sse: sse}
	if err := sp.Patch(Patch{Fragment: frag, Target: view.Target, Mode: view.Mode, ViewTransition: view.ViewTransition}); err != nil {
		a.log.Error("fragment patch", "path", c.R.URL.Path, "err", err)
	}
}

// adaptAction turns an ActionFunc into an http.HandlerFunc. It rejects plain
// navigation (a non-Datastar request) with 404 — actions are Datastar-only — then
// runs the mutation and streams the returned patches.
func (a *App) adaptAction(h ActionFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := a.newCtx(w, r)
		if !c.IsDatastar() {
			// Actions have no navigable representation.
			http.NotFound(w, r)
			return
		}
		patches, err := h(c)
		if err != nil {
			a.writeError(c, err)
			return
		}
		sse := datastar.NewSSE(c.W, c.R, datastar.WithCompression(a.cfg.SSECompression))
		sp := streamPatcher{sse: sse}
		for _, p := range patches {
			if err := sp.Patch(p); err != nil {
				a.log.Error("action patch", "path", c.R.URL.Path, "err", err)
				return
			}
		}
	}
}

// adaptStream turns a StreamFunc into an http.HandlerFunc that runs the SSE loop:
// subscribe to the declared topics, run catch-up, then fan hub events to the
// client until the request context is cancelled (client disconnect or shutdown).
func (a *App) adaptStream(h StreamFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := a.newCtx(w, r)
		sub, err := h(c)
		if err != nil {
			a.writeError(c, err)
			return
		}
		if a.hub == nil {
			// A stream with no hub configured can still serve catch-up, but live
			// events are impossible — report it loudly in the log and continue
			// with catch-up only.
			a.log.Warn("stream: no hub configured; live events disabled", "path", c.R.URL.Path)
		}

		mode := a.cfg.SSECompression
		if sub.Compression != nil {
			mode = *sub.Compression
		}
		sse := datastar.NewSSE(c.W, c.R, datastar.WithCompression(mode))
		sp := streamPatcher{sse: sse}

		// Catch-up replay first (completeness), then live (liveness).
		if sub.CatchUp != nil {
			if err := sub.CatchUp(c.Query("since"), sp); err != nil {
				a.log.Error("stream catch-up", "path", c.R.URL.Path, "err", err)
			}
		}

		// A visitor who paused live updates gets the catch-up render and nothing
		// after it (WCAG 2.2.2). Note 3 of that criterion is explicit that a
		// paused stream owes no replay of what it missed, which is why simply
		// dropping the topics is a complete answer rather than a lossy one.
		//
		// This sits in the adapter, not in the handler, so it holds for an app
		// that never renders a control: the preference cannot be routed around
		// by a StreamFunc that forgot to check it.
		if !LiveUpdates(c.Context()) {
			sub.Topics = nil
		}

		if a.hub == nil || len(sub.Topics) == 0 {
			return
		}

		hsub := a.hub.Subscribe(sub.Topics...)
		defer hsub.Close()

		ctx := c.Context()
		// A quiet hub never writes, and a dropped TCP client then never
		// cancels ctx. Ping every 15s so a hung-up peer fails the write
		// and we close the subscription.
		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ping.C:
				if err := sse.Ping(); err != nil {
					return
				}
			case ev, ok := <-hsub.C:
				if !ok {
					return
				}
				// Hub events carry a pre-rendered fragment. By default Datastar
				// morphs it by the fragment's own id; an event may instead carry an
				// append/prepend mode and a target container id so a high-volume
				// feed inserts a single line rather than re-rendering the container.
				opts := patchOptions(ev.Target, PatchMode(ev.DatastarMode()))
				if err := sse.PatchElements(string(ev.Bytes), opts...); err != nil {
					// Write failure means the client is gone; end the loop.
					return
				}
			}
		}
	}
}

// errSocketPanic marks a SocketFunc panic so the close-code mapping (1011)
// stays in one place. The recover here, not the recoverPanic middleware, must
// catch it: after the hijack a 500 page has no transport, a close frame does.
var errSocketPanic = errors.New("beach: socket handler panicked")

// adaptSocket turns a SocketFunc into an http.HandlerFunc that upgrades the
// connection and owns its lifecycle. The upgrade happens after the fixed
// middleware stack and any guards, so auth context is populated and a rejected
// guard returns plain HTTP, never a half-open socket. The handler runs on the
// request goroutine for the connection's lifetime; returning nil closes 1000,
// a peer close / canceled context closes 1000, anything else closes 1011.
func (a *App) adaptSocket(h SocketFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := a.cfg.Sockets.withDefaults()
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: cfg.Origins,
		})
		if err != nil {
			// Accept wrote the HTTP rejection (403 bad origin, 4xx bad handshake).
			a.log.Warn("socket: upgrade rejected", "path", r.URL.Path, "err", err)
			return
		}
		start := time.Now()

		// The request context is unreliable after a hijack; keep its values
		// (session, principal) but let the socket own its cancellation.
		s := newSocket(context.WithoutCancel(r.Context()), conn, cfg)
		a.sockets.add(s)
		defer a.sockets.remove(s)
		defer s.teardown()

		c := a.newCtx(w, r)
		err = func() (err error) {
			defer func() {
				if v := recover(); v != nil {
					a.log.Error("socket: panic recovered",
						"path", r.URL.Path,
						"panic", v,
						"stack", string(debug.Stack()),
					)
					err = errSocketPanic
				}
			}()
			return h(c, s)
		}()

		code, reason := websocket.StatusNormalClosure, ""
		if !socketCloseIsNormal(err) {
			code, reason = websocket.StatusInternalError, "internal error"
			a.log.Error("socket: handler error", "path", r.URL.Path, "err", err)
		}
		_ = s.Close(int(code), reason)
		a.log.Info("socket: closed",
			"path", r.URL.Path,
			"code", s.closeCode,
			"dur", time.Since(start).String(),
		)
	}
}

// adaptRaw wraps an http.HandlerFunc untouched — the escape hatch's adapter is a
// no-op so a raw handler keeps full control of the response.
func adaptRaw(h http.HandlerFunc) http.HandlerFunc { return h }
