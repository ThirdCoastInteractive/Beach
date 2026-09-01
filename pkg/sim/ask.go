package sim

import "context"

// Ask is the request/reply path: enqueue a command that computes a value on the
// loop goroutine and wait for it. The classic case is "roll dice -> result" or
// "craft -> CraftResult" — a mutation whose outcome the handler needs to render
// a response.
//
// The caller wraps a domain command in an asking command that carries a reply
// channel and sends the result from inside Apply. Ask returns when the reply
// arrives or ctx is cancelled. Because Apply runs single-threaded at the top of
// a tick, the reply reflects a consistent store state.
//
// Usage from a handler:
//
//	res, err := sim.Ask(ctx, s, func(reply func(CraftResult)) sim.Command {
//	    return craftCmd{spec: spec, reply: reply}
//	})
//
// where craftCmd.Apply mutates the store and calls reply(result). The helper
// below removes that boilerplate for the common shape.
func Ask[R any](ctx context.Context, s *Sim, build func(reply func(R)) Command) (R, error) {
	var zero R
	ch := make(chan R, 1)
	cmd := build(func(r R) {
		// Apply runs on the loop goroutine; ch is buffered(1) so this never
		// blocks even if the caller already gave up (ctx cancelled).
		select {
		case ch <- r:
		default:
		}
	})
	if err := s.Send(ctx, cmd); err != nil {
		return zero, err
	}
	select {
	case r := <-ch:
		return r, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// AskFunc is the simplest Ask shape: run fn against the World on the loop and
// return its result. fn may both read and mutate the store. Use it when no
// dedicated Command type is warranted (one-off queries, dice rolls).
//
//	roll, err := sim.AskFunc(ctx, s, func(w *sim.World) int { return w.Intn(20) + 1 })
func AskFunc[R any](ctx context.Context, s *Sim, fn func(w *World) R) (R, error) {
	return Ask(ctx, s, func(reply func(R)) Command {
		return askFuncCmd[R]{fn: fn, reply: reply}
	})
}

// askFuncCmd adapts a plain func into a Command for AskFunc.
type askFuncCmd[R any] struct {
	fn    func(w *World) R
	reply func(R)
}

func (c askFuncCmd[R]) Apply(w *World) { c.reply(c.fn(w)) }
