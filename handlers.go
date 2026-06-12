package beach

import (
	"net/http"

	"github.com/ThirdCoastInteractive/the-beach/datastar"
)

// The three typed handler shapes. Each is a function from *Ctx to a typed result
// plus an error; the framework adapts each to an http.HandlerFunc, owning the
// branch (PageFunc), the method/Datastar discipline (ActionFunc), and the SSE
// loop (StreamFunc). Handlers stay standalone factory functions that take only
// their deps — never methods on a server struct.

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

		if a.hub == nil || len(sub.Topics) == 0 {
			return
		}

		hsub := a.hub.Subscribe(sub.Topics...)
		defer hsub.Close()

		ctx := c.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-hsub.C:
				if !ok {
					return
				}
				// Hub events carry a pre-rendered fragment; patch it as an element
				// update so Datastar morphs it by the fragment's own id.
				if err := sse.PatchElements(string(ev.Bytes)); err != nil {
					// Write failure means the client is gone; end the loop.
					return
				}
			}
		}
	}
}

// adaptRaw wraps an http.HandlerFunc untouched — the escape hatch's adapter is a
// no-op so a raw handler keeps full control of the response.
func adaptRaw(h http.HandlerFunc) http.HandlerFunc { return h }
