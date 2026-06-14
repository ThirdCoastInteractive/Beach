package repute

import (
	"sort"
	"time"
)

// trackKey indexes a single (actor, track) reputation cell.
type trackKey struct {
	actor ActorID
	track Track
}

// additiveDelta is a retained Add-mode delta awaiting decay at projection time.
type additiveDelta struct {
	amount float64
	at     time.Time
	decay  string
}

// State is the materialized projection of a delta stream. It holds, per
// (actor, track), the list of additive deltas (decayed lazily) and the current
// Set rating if any. State is rebuilt by Replay or advanced by Apply; it is a
// cache derived from the log, never an authority.
type State struct {
	lastSeq uint64

	// additive holds Add-mode deltas per cell.
	additive map[trackKey][]additiveDelta

	// rating holds the latest Set-mode value per cell.
	rating map[trackKey]float64

	// trackMembers indexes actors per track for ranking.
	trackMembers map[Track]map[ActorID]struct{}
}

// NewState returns an empty projection.
func NewState() *State {
	return &State{
		additive:     make(map[trackKey][]additiveDelta),
		rating:       make(map[trackKey]float64),
		trackMembers: make(map[Track]map[ActorID]struct{}),
	}
}

// LastSeq returns the Seq of the most recently applied event.
func (s *State) LastSeq() uint64 { return s.lastSeq }

func (s *State) note(actor ActorID, track Track) {
	m := s.trackMembers[track]
	if m == nil {
		m = make(map[ActorID]struct{})
		s.trackMembers[track] = m
	}
	m[actor] = struct{}{}
}

// ingest folds a single delta into the projection.
func (s *State) ingest(d Delta) {
	k := trackKey{d.Actor, d.Track}
	switch d.Mode {
	case Set:
		s.rating[k] = d.Amount
	default: // Add
		s.additive[k] = append(s.additive[k], additiveDelta{
			amount: d.Amount, at: d.At, decay: d.Decay,
		})
	}
	s.note(d.Actor, d.Track)
}

// scoreAt computes the decayed reputation of a cell as of time `now`, combining
// the Set rating (if any, undecayed) with the decayed sum of additive deltas.
func (s *State) scoreAt(dm *DecayModel, actor ActorID, track Track, now time.Time) float64 {
	k := trackKey{actor, track}
	var total float64
	if r, ok := s.rating[k]; ok {
		total += r
	}
	for _, ad := range s.additive[k] {
		elapsed := now.Sub(ad.at)
		if elapsed < 0 {
			elapsed = 0
		}
		total += dm.Resolve(ad.decay).Value(ad.amount, elapsed)
	}
	return total
}

// ratingOf returns the raw Set value of a cell.
func (s *State) ratingOf(actor ActorID, track Track) (float64, bool) {
	v, ok := s.rating[trackKey{actor, track}]
	return v, ok
}

// Leaderboard is a ranked snapshot of a track at a given as-of time.
type Leaderboard struct {
	Track   Track
	AsOf    time.Time
	Entries []LeaderboardEntry
}

// LeaderboardEntry is one ranked actor.
type LeaderboardEntry struct {
	Rank  int
	Actor ActorID
	Score float64
}

// leaderboard materializes a ranked, decayed view of a track. Ties broken by
// ActorID for determinism.
func (s *State) leaderboard(dm *DecayModel, track Track, now time.Time) Leaderboard {
	members := s.trackMembers[track]
	entries := make([]LeaderboardEntry, 0, len(members))
	for a := range members {
		entries = append(entries, LeaderboardEntry{
			Actor: a, Score: s.scoreAt(dm, a, track, now),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Score != entries[j].Score {
			return entries[i].Score > entries[j].Score
		}
		return entries[i].Actor < entries[j].Actor
	})
	for i := range entries {
		entries[i].Rank = i + 1
	}
	return Leaderboard{Track: track, AsOf: now, Entries: entries}
}
