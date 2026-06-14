package repute

import (
	"encoding/json"
	"time"
)

// ActorID identifies a participant whose reputation is tracked. It is opaque to
// the kernel; callers map it to whatever identity primitive they use.
type ActorID string

// Track names a reputation dimension (e.g. "debater", "judge"). Reputation is a
// vector over tracks, never a single scalar.
type Track string

// Event is an immutable, append-only record of something that happened. The
// kernel never interprets Payload; domain meaning lives entirely in the scorers
// the caller installs.
//
// Determinism contract: scoring depends only on the Event stream (in Seq order)
// and the installed ruleset. Time carries DOMAIN time and is the only clock a
// scorer may consult — never time.Now. This is what makes a leaderboard
// independently re-derivable from the log.
type Event struct {
	// Seq is a monotonic total order assigned by the EventSource on append.
	// Events are always folded in ascending Seq order.
	Seq uint64 `json:"seq"`

	// Time is the domain timestamp of the event, used by decay models. It must
	// be non-decreasing with Seq for decay to behave monotonically.
	Time time.Time `json:"time"`

	// Kind is a caller-defined discriminator, e.g. "debate.resolved". Scorers
	// match on it via Scorable.
	Kind string `json:"kind"`

	// Actors lists every participant this event touches. Convenience for
	// indexing and validation; scorers may also pull actors out of Payload.
	Actors []ActorID `json:"actors,omitempty"`

	// Payload is domain data the matching scorer decodes.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// DecodePayload unmarshals the event payload into v.
func (e Event) DecodePayload(v any) error {
	if len(e.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(e.Payload, v)
}

// NewEvent is a convenience constructor that marshals payload. Seq is left zero;
// the EventSource assigns it on append.
func NewEvent(t time.Time, kind string, payload any, actors ...ActorID) (Event, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Event{}, err
		}
		raw = b
	}
	return Event{Time: t, Kind: kind, Actors: actors, Payload: raw}, nil
}
