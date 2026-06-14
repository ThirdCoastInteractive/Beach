package repute

import (
	"encoding/json"
	"fmt"
	"time"
)

// Snapshot is a serializable capture of an Engine's projected state at a given
// LastSeq. It is a CACHE, not an authority: the event log remains the source of
// truth, and a snapshot only saves refolding the entire log on restart. Restore
// a snapshot, then replay events with Seq > LastSeq to catch up.
//
// Stateful scorers that implement Snapshotable have their caches persisted in
// ScorerStates, keyed by Scorer.Name(). This allows full warm-start without
// replaying from seq=0. For pure additive/Set projections the snapshot is
// sufficient on its own even without scorer states.
type Snapshot struct {
	LastSeq      uint64                    `json:"last_seq"`
	Additive     []snapAdditive            `json:"additive"`
	Ratings      []snapRating              `json:"ratings"`
	RulesetID    string                    `json:"ruleset_id,omitempty"`
	ScorerStates map[string]json.RawMessage `json:"scorer_states,omitempty"`
}

type snapAdditive struct {
	Actor  ActorID   `json:"actor"`
	Track  Track     `json:"track"`
	Amount float64   `json:"amount"`
	At     time.Time `json:"at"`
	Decay  string    `json:"decay,omitempty"`
}

type snapRating struct {
	Actor ActorID `json:"actor"`
	Track Track   `json:"track"`
	Value float64 `json:"value"`
}

// Snapshot serializes the engine's current projected state, including any
// stateful scorer caches from scorers implementing Snapshotable.
func (e *Engine) Snapshot() Snapshot {
	s := Snapshot{
		LastSeq:   e.state.lastSeq,
		RulesetID: e.RulesetID(),
	}
	for k, list := range e.state.additive {
		for _, ad := range list {
			s.Additive = append(s.Additive, snapAdditive{
				Actor: k.actor, Track: k.track, Amount: ad.amount, At: ad.at, Decay: ad.decay,
			})
		}
	}
	for k, v := range e.state.rating {
		s.Ratings = append(s.Ratings, snapRating{Actor: k.actor, Track: k.track, Value: v})
	}
	for _, sc := range e.scorers {
		if snap, ok := sc.(Snapshotable); ok {
			data, err := snap.ScorerSnapshot()
			if err != nil {
				continue
			}
			if s.ScorerStates == nil {
				s.ScorerStates = make(map[string]json.RawMessage)
			}
			s.ScorerStates[sc.Name()] = data
		}
	}
	return s
}

// LoadSnapshot replaces the engine's state with the snapshot's, including
// restoring any stateful scorer caches from scorers implementing Snapshotable.
// Follow with ReplayAppend over SourceAfter(snapshot.LastSeq) to catch up.
func (e *Engine) LoadSnapshot(s Snapshot) error {
	st := NewState()
	st.lastSeq = s.LastSeq
	for _, a := range s.Additive {
		k := trackKey{a.Actor, a.Track}
		st.additive[k] = append(st.additive[k], additiveDelta{amount: a.Amount, at: a.At, decay: a.Decay})
		st.note(a.Actor, a.Track)
	}
	for _, r := range s.Ratings {
		st.rating[trackKey{r.Actor, r.Track}] = r.Value
		st.note(r.Actor, r.Track)
	}
	e.state = st
	if len(s.ScorerStates) > 0 {
		for _, sc := range e.scorers {
			snap, ok := sc.(Snapshotable)
			if !ok {
				continue
			}
			data, exists := s.ScorerStates[sc.Name()]
			if !exists {
				continue
			}
			if err := snap.LoadScorerSnapshot(data); err != nil {
				return fmt.Errorf("repute: load scorer snapshot %q: %w", sc.Name(), err)
			}
		}
	}
	return nil
}

// Marshal encodes the snapshot as JSON.
func (s Snapshot) Marshal() ([]byte, error) { return json.Marshal(s) }

// UnmarshalSnapshot decodes a snapshot from JSON.
func UnmarshalSnapshot(b []byte) (Snapshot, error) {
	var s Snapshot
	err := json.Unmarshal(b, &s)
	return s, err
}
