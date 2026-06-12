package main

import (
	"sync"
	"time"

	repute "github.com/ThirdCoastInteractive/Repute"
	"github.com/ThirdCoastInteractive/Repute/scorers"
)

// hush scores two reputation dimensions: standing earned by authoring posts
// that land (author), and the standing of your invite chain (chain) — when an
// invitee's post draws a 👍 or 👎, a half-weight echo lands on whoever invited
// them. The chain track throttles invite allowances (see inviteAllowance).
const (
	authorTrack = repute.Track("author")
	chainTrack  = repute.Track("chain")
)

// Tier thresholds. Kept low so the mechanic is easy to see: a post is worth 1,
// a 👍 on your post is worth 2, a 👎 costs 2 — standing tracks your
// like/dislike ratio, not raw volume.
const (
	tier1Score = 3.0 // unlocks the profile
	tier2Score = 8.0 // unlocks DMs (future)
)

// reputation wraps a Repute engine with a mutex — the engine is single-goroutine,
// and hush's handlers are concurrent. Three Participation scorers fold "post",
// "like", and "dislike" events onto the author track — dislikes carry negative
// Points, so the score is the author's net standing; reading it is
// e.Score(actor, track, now). It is in-memory (matching the in-memory wall);
// the eventsource/pg adapter is the persistent path when posts move to Postgres.
type reputation struct {
	mu  sync.Mutex
	eng *repute.Engine
}

func newReputation() *reputation {
	eng := repute.New([]repute.Scorer{
		scorers.Participation{Track: authorTrack, Kinds: []string{"post"}, Points: 1},
		scorers.Participation{Track: authorTrack, Kinds: []string{"like"}, Points: 2},
		scorers.Participation{Track: authorTrack, Kinds: []string{"dislike"}, Points: -2},
		// Half-weight echoes of an invitee's reactions, credited to the inviter.
		scorers.Participation{Track: chainTrack, Kinds: []string{"chain-like"}, Points: 1},
		scorers.Participation{Track: chainTrack, Kinds: []string{"chain-dislike"}, Points: -1},
	})
	return &reputation{eng: eng}
}

// emit folds one event (kind "post", "like", "dislike", or their "chain-"
// echoes) crediting — or dinging — the actor on its track. Anonymous/open-mode
// actors ("") are skipped — scoring needs an identity.
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

// chainScore returns a member's standing on the chain track — how their
// invitees' posts have landed.
func (r *reputation) chainScore(member string) float64 {
	if member == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.eng.Score(repute.ActorID(member), chainTrack, time.Now())
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
