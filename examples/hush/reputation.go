package main

import (
	"sync"
	"time"

	repute "github.com/ThirdCoastInteractive/Repute"
	"github.com/ThirdCoastInteractive/Repute/scorers"
)

// authorTrack is the single reputation dimension hush scores: standing earned by
// authoring posts that land.
const authorTrack = repute.Track("author")

// Tier thresholds. Kept low so the mechanic is easy to see: a post is worth 1,
// an upvote on your post is worth 2.
const (
	tier1Score = 3.0 // unlocks the profile
	tier2Score = 8.0 // unlocks DMs (future)
)

// reputation wraps a Repute engine with a mutex — the engine is single-goroutine,
// and hush's handlers are concurrent. Two Participation scorers fold "post" and
// "vote" events onto the author track; reading a score is e.Score(actor, track,
// now). It is in-memory (matching the in-memory wall); the eventsource/pg adapter
// is the persistent path when posts move to Postgres.
type reputation struct {
	mu  sync.Mutex
	eng *repute.Engine
}

func newReputation() *reputation {
	eng := repute.New([]repute.Scorer{
		scorers.Participation{Track: authorTrack, Kinds: []string{"post"}, Points: 1},
		scorers.Participation{Track: authorTrack, Kinds: []string{"vote"}, Points: 2},
	})
	return &reputation{eng: eng}
}

// emit folds one event (kind "post" or "vote") crediting author onto the track.
// Anonymous/open-mode authors ("") are skipped — scoring needs an identity.
func (r *reputation) emit(kind, author string) {
	if author == "" {
		return
	}
	ev, err := repute.NewEvent(time.Now(), kind, nil, repute.ActorID(author))
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = r.eng.Apply(ev)
}

// score returns an author's current standing on the author track.
func (r *reputation) score(author string) float64 {
	if author == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.eng.Score(repute.ActorID(author), authorTrack, time.Now())
}

// tierOf maps a score to a capability tier.
func tierOf(score float64) int {
	switch {
	case score >= tier2Score:
		return 2
	case score >= tier1Score:
		return 1
	default:
		return 0
	}
}
