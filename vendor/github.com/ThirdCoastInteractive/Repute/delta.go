package repute

import "time"

// Delta is a single reputation movement emitted by a Scorer. Deltas are
// append-only and never mutated; current reputation is the decayed sum of all
// deltas for an (actor, track) pair. Every Delta is auditable: Reason explains
// it in human terms and SourceSeq links it to the event that produced it.
type Delta struct {
	// Actor and Track identify which reputation dimension moves.
	Actor ActorID `json:"actor"`
	Track Track   `json:"track"`

	// Amount is the reputation change at the moment of the source event,
	// before any decay is applied.
	Amount float64 `json:"amount"`

	// Mode controls how this delta combines with prior state for scorers that
	// maintain a single evolving value (Elo, Glicko-2, TrueSkill). Additive
	// deltas (Brier, BTS, Participation) accumulate; Set deltas replace the
	// running track value. See ScoreMode.
	Mode ScoreMode `json:"mode"`

	// Reason is a human-readable, auditable explanation, e.g.
	// "won vs actor:bob, margin 0.62 (glicko2 +18.3)".
	Reason string `json:"reason"`

	// Decay names the decay policy governing this delta over time. Empty means
	// the engine's default decay model. Ignored for Set-mode deltas, whose
	// value is the rating itself.
	Decay string `json:"decay,omitempty"`

	// SourceSeq is the Seq of the Event that produced this delta. Set by the
	// engine, not the scorer.
	SourceSeq uint64 `json:"source_seq"`

	// At is the domain time of the source event, copied by the engine so decay
	// can be computed without re-reading the log.
	At time.Time `json:"at"`
}

// ScoreMode distinguishes accumulating deltas from rating replacements.
type ScoreMode uint8

const (
	// Add accumulates: the track value is the (decayed) sum of all Add deltas.
	// Used by proper scoring rules, BTS, participation, anything additive.
	Add ScoreMode = iota

	// Set replaces: the track value becomes Amount. Used by stateful rating
	// systems (Elo, Glicko-2, TrueSkill) that compute the new absolute rating
	// internally and emit it. Set deltas are not decayed by the engine; if a
	// rating system wants inactivity decay it emits it as part of scoring
	// (Glicko-2 RD inflation, for example).
	Set
)

func (m ScoreMode) String() string {
	switch m {
	case Add:
		return "add"
	case Set:
		return "set"
	default:
		return "unknown"
	}
}
