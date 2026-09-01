package sim

import (
	"context"

	"github.com/ThirdCoastInteractive/Beach/pkg/ecs"
)

// Snapshot returns an immutable, off-loop-safe read view of the live store as of
// the tick it runs on. It is the first-class catch-up-render path: a handler
// serving a page-nav or opening an SSE stream takes a Snapshot once and renders
// the current board from it, with no per-read round-trip through the loop.
//
// How the concurrency guarantee holds: the copy (ecs.Store.View) runs inside a
// command on the loop goroutine, where store access is exclusive, so the View is
// a consistent deep copy of one tick's state. It shares no memory with the
// store, so the returned *ecs.View is safe to read from the handler goroutine
// while the loop keeps mutating. Read it with ecs.ViewQuery / ViewGet /
// ViewChanged / ViewHas.
//
// Snapshot blocks until the loop processes the capture command (like any Ask),
// returning ctx.Err() if ctx is cancelled first. Call it only once Run owns the
// loop (or while something is driving Tick); before Run, copy s.Store().View()
// directly on the constructing goroutine instead.
func (s *Sim) Snapshot(ctx context.Context) (*ecs.View, error) {
	return AskFunc(ctx, s, func(w *World) *ecs.View { return w.Store.View() })
}
