package repute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// Engine folds an event stream into reputation. It is the heart of Repute:
// install scorers and a decay model, then Apply events (live) or Replay a log
// (rebuild). The engine is single-goroutine; wrap with your own locking if you
// share it.
type Engine struct {
	scorers []Scorer
	decay   *DecayModel
	state   *State

	// audit, when non-nil, receives every delta the engine produces, in order.
	audit func(Delta)

	hooks *Hooks
}

// Option configures an Engine.
type Option func(*Engine)

// WithDecay sets the decay model. Defaults to NoDecay-only.
func WithDecay(dm *DecayModel) Option { return func(e *Engine) { e.decay = dm } }

// WithAudit installs a sink that receives every emitted delta in Seq/scorer
// order. Use to persist the audit trail.
func WithAudit(sink func(Delta)) Option { return func(e *Engine) { e.audit = sink } }

// New builds an engine from a set of scorers and options. Scorer order is
// significant: scorers run in the given order for each event, and a scorer sees
// the View as mutated by earlier scorers on the same event.
func New(scorers []Scorer, opts ...Option) *Engine {
	e := &Engine{
		scorers: scorers,
		decay:   NewDecayModel(),
		state:   NewState(),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// view is the read-only View handed to scorers, pinned to the time of the
// event being scored.
type view struct {
	e   *Engine
	now time.Time
}

func (v view) Score(a ActorID, t Track) float64 {
	return v.e.state.scoreAt(v.e.decay, a, t, v.now)
}
func (v view) Rating(a ActorID, t Track) (float64, bool) {
	return v.e.state.ratingOf(a, t)
}
func (v view) Rank(a ActorID, t Track) (int, int) {
	lb := v.e.state.leaderboard(v.e.decay, t, v.now)
	for _, en := range lb.Entries {
		if en.Actor == a {
			return en.Rank, len(lb.Entries)
		}
	}
	return 0, len(lb.Entries)
}
func (v view) AsOf() time.Time { return v.now }

// Apply folds one event into state, running every matching scorer in order and
// returning the deltas produced. Events must be applied in ascending Seq order;
// Apply enforces this.
func (e *Engine) Apply(ev Event) ([]Delta, error) {
	start := time.Now()
	if ev.Seq != 0 && ev.Seq <= e.state.lastSeq {
		return nil, fmt.Errorf("repute: out-of-order event seq=%d, last=%d", ev.Seq, e.state.lastSeq)
	}
	v := view{e: e, now: ev.Time}
	var out []Delta
	for _, s := range e.scorers {
		if !s.Scorable(ev.Kind) {
			continue
		}
		deltas, err := s.Score(ev, v)
		if err != nil {
			return nil, fmt.Errorf("repute: scorer %q on seq=%d: %w", s.Name(), ev.Seq, err)
		}
		for i := range deltas {
			deltas[i].SourceSeq = ev.Seq
			deltas[i].At = ev.Time
			e.state.ingest(deltas[i])
			if e.audit != nil {
				e.audit(deltas[i])
			}
			out = append(out, deltas[i])
		}
	}
	if ev.Seq != 0 {
		e.state.lastSeq = ev.Seq
	}
	if e.hooks != nil && e.hooks.OnApply != nil {
		e.hooks.OnApply(ApplyInfo{Event: ev, Deltas: out, Duration: time.Since(start)})
	}
	return out, nil
}

// EventSource yields events in ascending Seq order for replay.
type EventSource interface {
	// Next returns the next event and true, or zero/false at end of stream.
	Next(ctx context.Context) (Event, bool, error)
}

// Replay rebuilds state from scratch over the entire log. Stateful scorers are
// Reset first. Deterministic: identical (log, ruleset) yields identical state.
func (e *Engine) Replay(ctx context.Context, src EventSource) error {
	start := time.Now()
	e.state = NewState()
	for _, s := range e.scorers {
		if r, ok := s.(Resettable); ok {
			r.Reset()
		}
	}
	var count int
	for {
		ev, ok, err := src.Next(ctx)
		if err != nil {
			return err
		}
		if !ok {
			if e.hooks != nil && e.hooks.OnReplay != nil {
				e.hooks.OnReplay(ReplayInfo{
					EventCount: count,
					LastSeq:    e.state.lastSeq,
					Duration:   time.Since(start),
				})
			}
			return nil
		}
		if _, err := e.Apply(ev); err != nil {
			return err
		}
		count++
	}
}

// ReplayAppend applies events from src without resetting state or scorers.
// Use after LoadSnapshot to catch up on events newer than the snapshot.
func (e *Engine) ReplayAppend(ctx context.Context, src EventSource) error {
	start := time.Now()
	var count int
	for {
		ev, ok, err := src.Next(ctx)
		if err != nil {
			return err
		}
		if !ok {
			if e.hooks != nil && e.hooks.OnReplay != nil {
				e.hooks.OnReplay(ReplayInfo{
					EventCount: count,
					LastSeq:    e.state.lastSeq,
					Duration:   time.Since(start),
				})
			}
			return nil
		}
		if _, err := e.Apply(ev); err != nil {
			return err
		}
		count++
	}
}

// Score returns the current decayed reputation of an actor on a track, as of
// the given time. Pass the wall-clock "now" you want the leaderboard evaluated
// at; decay is computed relative to it.
func (e *Engine) Score(a ActorID, t Track, asOf time.Time) float64 {
	start := time.Now()
	v := e.state.scoreAt(e.decay, a, t, asOf)
	if e.hooks != nil && e.hooks.OnScore != nil {
		e.hooks.OnScore(ScoreInfo{
			Actor: a, Track: t, Value: v, AsOf: asOf,
			Duration: time.Since(start),
		})
	}
	return v
}

// Leaderboard returns the ranked, decayed standings for a track as of asOf.
func (e *Engine) Leaderboard(t Track, asOf time.Time) Leaderboard {
	start := time.Now()
	lb := e.state.leaderboard(e.decay, t, asOf)
	if e.hooks != nil && e.hooks.OnLeaderboard != nil {
		e.hooks.OnLeaderboard(LeaderboardInfo{
			Track: t, Entries: len(lb.Entries), AsOf: asOf,
			Duration: time.Since(start),
		})
	}
	return lb
}

// Tracks returns all tracks that have at least one scored actor, sorted.
func (e *Engine) Tracks() []Track {
	ts := make([]Track, 0, len(e.state.trackMembers))
	for t := range e.state.trackMembers {
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	return ts
}

// RulesetID is a stable hash of the installed scorers (by name, in order) and
// the decay model's registered names. Publishing leaderboards as
// (RulesetID, asOf, scores) makes any change to scoring a visible, dated event
// rather than a silent edit.
func (e *Engine) RulesetID() string {
	h := sha256.New()
	for _, s := range e.scorers {
		fmt.Fprintf(h, "scorer:%s\n", s.Name())
	}
	names := make([]string, 0, len(e.decay.byName))
	for n := range e.decay.byName {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(h, "decay:%s\n", n)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
