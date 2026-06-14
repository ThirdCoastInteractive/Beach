package repute

import (
	"encoding/json"
	"time"
)

// Scorer is the pluggable unit of reputation logic. Implementations must be
// PURE with respect to the engine: no I/O, no time.Now, no global mutable
// state. All inputs come from the Event and the read-only View; all outputs are
// returned Deltas. This purity is what guarantees deterministic, replayable
// scoring.
//
// A scorer MAY keep internal state across calls (e.g. a Glicko-2 scorer caching
// rating deviations), but that state must be a deterministic function of the
// events it has seen, in Seq order. Stateful scorers should implement
// Resettable so Replay can rebuild cleanly.
type Scorer interface {
	// Name identifies the scorer for ruleset hashing and audit. Must be stable.
	Name() string

	// Scorable reports whether this scorer handles the given event Kind. The
	// engine skips scorers that return false, so this should be cheap.
	Scorable(kind string) bool

	// Score interprets the event and returns reputation deltas. view exposes
	// reputation as of just-before this event (see ordering note on View).
	Score(e Event, view View) ([]Delta, error)
}

// Resettable is implemented by stateful scorers so Replay can clear cached
// state before refolding the log.
type Resettable interface {
	// Reset clears all internal state so a fresh replay can rebuild from scratch.
	Reset()
}

// Snapshotable is implemented by stateful scorers (Glicko-2, TrueSkill) to
// persist their internal caches alongside the engine snapshot. Without this,
// restoring from a snapshot requires a full replay from seq=0 to rebuild
// scorer state. The JSON blob is keyed by Scorer.Name in the snapshot.
type Snapshotable interface {
	// ScorerSnapshot serializes the scorer's internal state as JSON.
	ScorerSnapshot() (json.RawMessage, error)

	// LoadScorerSnapshot restores internal state from a previously serialized blob.
	LoadScorerSnapshot(json.RawMessage) error
}

// View is a read-only snapshot of reputation presented to scorers during
// Score. It reflects all deltas from events with Seq strictly less than the
// event currently being scored (the log is totally ordered, so "before" is
// well defined). This enables confidence-weighting, vote-weighting by judge
// reputation, and cold-start logic — at the cost of order-dependence, which is
// acceptable precisely because ordering is deterministic.
type View interface {
	// Score returns the current decayed reputation of actor a on track t,
	// evaluated as of the event being scored.
	Score(a ActorID, t Track) float64

	// Rating returns the raw (un-decayed, last-Set) value for stateful tracks,
	// plus whether the actor has any rating on that track yet. Useful for
	// rating systems that need the prior absolute value (Elo, Glicko-2).
	Rating(a ActorID, t Track) (value float64, ok bool)

	// Rank returns the 1-based rank of actor a on track t and the total number
	// of ranked actors. rank is 0 if the actor is unranked.
	Rank(a ActorID, t Track) (rank, total int)

	// AsOf is the domain time of the event being scored, the only clock a
	// scorer may use.
	AsOf() time.Time
}
