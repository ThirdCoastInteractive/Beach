package repute

import (
	"context"
	"sort"
	"time"
)

// EventTransform rewrites or drops events during counterfactual replay. Return
// the (possibly modified) event and true to keep it, or zero and false to drop.
// Transforms must be pure — no side effects, no wall-clock reads.
type EventTransform func(Event) (Event, bool)

// DropSeqs returns a transform that drops events with any of the given Seq values.
func DropSeqs(seqs ...uint64) EventTransform {
	set := make(map[uint64]struct{}, len(seqs))
	for _, s := range seqs {
		set[s] = struct{}{}
	}
	return func(e Event) (Event, bool) {
		_, drop := set[e.Seq]
		return e, !drop
	}
}

// DropActor returns a transform that drops every event listing actor in its Actors field.
func DropActor(actor ActorID) EventTransform {
	return func(e Event) (Event, bool) {
		for _, a := range e.Actors {
			if a == actor {
				return Event{}, false
			}
		}
		return e, true
	}
}

// DropIf returns a transform that drops events matching the predicate.
func DropIf(pred func(Event) bool) EventTransform {
	return func(e Event) (Event, bool) {
		if pred(e) {
			return Event{}, false
		}
		return e, true
	}
}

// MapEvents returns a transform that rewrites each event through fn. To drop
// an event from a MapEvents, wrap it with DropIf separately.
func MapEvents(fn func(Event) Event) EventTransform {
	return func(e Event) (Event, bool) {
		return fn(e), true
	}
}

// Identity is a transform that passes all events through unchanged.
func Identity() EventTransform {
	return func(e Event) (Event, bool) { return e, true }
}

// Compose chains transforms left to right. An event dropped by any transform
// is dropped from the stream.
func Compose(transforms ...EventTransform) EventTransform {
	return func(e Event) (Event, bool) {
		for _, xf := range transforms {
			var keep bool
			e, keep = xf(e)
			if !keep {
				return Event{}, false
			}
		}
		return e, true
	}
}

// TransformSource wraps an EventSource with a transform, yielding only events
// that survive the transform (with any rewrites applied). Dropped events are
// silently skipped; surviving events keep their original Seq.
func TransformSource(src EventSource, xf EventTransform) EventSource {
	return &xfSource{src: src, xf: xf}
}

type xfSource struct {
	src EventSource
	xf  EventTransform
}

func (x *xfSource) Next(ctx context.Context) (Event, bool, error) {
	for {
		ev, ok, err := x.src.Next(ctx)
		if err != nil || !ok {
			return ev, ok, err
		}
		out, keep := x.xf(ev)
		if keep {
			return out, true, nil
		}
	}
}

// DiffEntry describes one actor's movement between baseline and counterfactual.
type DiffEntry struct {
	Actor      ActorID
	BaseRank   int
	BaseScore  float64
	AltRank    int
	AltScore   float64
	RankDelta  int
	ScoreDelta float64
}

// LeaderboardDiff is the structured comparison between a baseline and
// counterfactual leaderboard on one track at one as-of time.
type LeaderboardDiff struct {
	Track   Track
	AsOf    time.Time
	Entries []DiffEntry
	Entered []ActorID
	Exited  []ActorID
}

// DiffLeaderboards computes the structured diff between two leaderboards.
// Entered actors appear in alt but not base; exited appear in base but not alt.
// Entries are sorted by absolute score delta descending (biggest movers first).
func DiffLeaderboards(base, alt Leaderboard) LeaderboardDiff {
	baseMap := make(map[ActorID]LeaderboardEntry, len(base.Entries))
	for _, e := range base.Entries {
		baseMap[e.Actor] = e
	}
	altMap := make(map[ActorID]LeaderboardEntry, len(alt.Entries))
	for _, e := range alt.Entries {
		altMap[e.Actor] = e
	}

	diff := LeaderboardDiff{Track: base.Track, AsOf: base.AsOf}

	all := make(map[ActorID]struct{})
	for _, e := range base.Entries {
		all[e.Actor] = struct{}{}
	}
	for _, e := range alt.Entries {
		all[e.Actor] = struct{}{}
	}

	for actor := range all {
		be, inBase := baseMap[actor]
		ae, inAlt := altMap[actor]
		switch {
		case inBase && !inAlt:
			diff.Exited = append(diff.Exited, actor)
		case !inBase && inAlt:
			diff.Entered = append(diff.Entered, actor)
			diff.Entries = append(diff.Entries, DiffEntry{
				Actor: actor, AltRank: ae.Rank, AltScore: ae.Score,
				ScoreDelta: ae.Score,
			})
		default:
			diff.Entries = append(diff.Entries, DiffEntry{
				Actor:      actor,
				BaseRank:   be.Rank,
				BaseScore:  be.Score,
				AltRank:    ae.Rank,
				AltScore:   ae.Score,
				RankDelta:  be.Rank - ae.Rank,
				ScoreDelta: ae.Score - be.Score,
			})
		}
	}

	sort.Slice(diff.Entries, func(i, j int) bool {
		ai := diff.Entries[i].ScoreDelta
		aj := diff.Entries[j].ScoreDelta
		if ai < 0 {
			ai = -ai
		}
		if aj < 0 {
			aj = -aj
		}
		if ai != aj {
			return ai > aj
		}
		return diff.Entries[i].Actor < diff.Entries[j].Actor
	})
	sort.Slice(diff.Entered, func(i, j int) bool { return diff.Entered[i] < diff.Entered[j] })
	sort.Slice(diff.Exited, func(i, j int) bool { return diff.Exited[i] < diff.Exited[j] })

	return diff
}

// Counterfactual replays the event source through a fresh engine built by
// newEngine, applying xf to each event, then diffs the resulting leaderboard
// against baseline. The baseline engine is never touched.
//
// src must be a fresh, re-readable EventSource positioned at the start of the
// log. newEngine must return a fresh engine with the same scorers and decay as
// the baseline (use a shared factory to guarantee this).
func Counterfactual(ctx context.Context, baseline Leaderboard, src EventSource,
	newEngine func() *Engine, xf EventTransform) (LeaderboardDiff, error) {

	alt := newEngine()
	xfSrc := TransformSource(src, xf)
	if err := alt.Replay(ctx, xfSrc); err != nil {
		return LeaderboardDiff{}, err
	}
	altBoard := alt.Leaderboard(baseline.Track, baseline.AsOf)
	return DiffLeaderboards(baseline, altBoard), nil
}
