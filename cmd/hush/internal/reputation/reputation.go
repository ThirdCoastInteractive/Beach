// Package reputation is hush's standing engine and its live gate-and-moderation
// policy. Repute (reputation.go) folds posting activity into author and chain
// scores; Settings (settings.go) holds the Postgres-backed policy snapshot
// behind an atomic pointer. TierOf and InviteAllowance are the pure rules the
// web layer applies, given a policy snapshot.
package reputation

import (
	"sync"
	"time"

	repute "github.com/ThirdCoastInteractive/Repute"
	"github.com/ThirdCoastInteractive/Repute/scorers"
)

// hush scores two reputation dimensions: standing earned by authoring posts
// that land (author), and the standing of your invite chain (chain) — when an
// invitee's post draws a 👍 or 👎, a half-weight echo lands on whoever invited
// them. The chain track throttles invite allowances (see InviteAllowance).
const (
	authorTrack = repute.Track("author")
	chainTrack  = repute.Track("chain")
)

// Repute wraps a Repute engine with a mutex — the engine is single-goroutine,
// and hush's handlers are concurrent. Participation scorers fold "post", "like",
// "dislike", and "email-verified" events onto the author track — dislikes carry
// negative Points, so the score is the author's net standing; reading it is
// Score(actor). The engine is in-memory: in database mode the wall replays its
// Postgres history into it at boot, the eventsource/pg adapter being the
// eventual persistent path.
type Repute struct {
	mu  sync.Mutex
	eng *repute.Engine
}

// New builds the engine. Event weights are kept low so the mechanic is easy to
// see: a post is worth 1, a 👍 on your post is worth 2, a 👎 costs 2 — standing
// tracks your like/dislike ratio, not raw volume. emailPoints is the one-time
// bump for verifying an email (HUSH_EMAIL_POINTS), baked in at construction.
func New(emailPoints float64) *Repute {
	eng := repute.New([]repute.Scorer{
		scorers.Participation{Track: authorTrack, Kinds: []string{"post"}, Points: 1},
		scorers.Participation{Track: authorTrack, Kinds: []string{"like"}, Points: 2},
		scorers.Participation{Track: authorTrack, Kinds: []string{"dislike"}, Points: -2},
		// Verifying an email is a one-time standing bump — a reachable member
		// is worth more than an anonymous drive-by.
		scorers.Participation{Track: authorTrack, Kinds: []string{"email-verified"}, Points: emailPoints},
		// Half-weight echoes of an invitee's reactions, credited to the inviter.
		scorers.Participation{Track: chainTrack, Kinds: []string{"chain-like"}, Points: 1},
		scorers.Participation{Track: chainTrack, Kinds: []string{"chain-dislike"}, Points: -1},
	})
	return &Repute{eng: eng}
}

// Emit folds one event (kind "post", "like", "dislike", "email-verified", or
// the "chain-" echoes) crediting — or dinging — the actor on its track.
// Anonymous/open-mode actors ("") are skipped — scoring needs an identity.
func (r *Repute) Emit(kind, author string) {
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

// Score returns an author's current standing on the author track.
func (r *Repute) Score(author string) float64 {
	if author == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.eng.Score(repute.ActorID(author), authorTrack, time.Now())
}

// ChainScore returns a member's standing on the chain track — how their
// invitees' posts have landed.
func (r *Repute) ChainScore(member string) float64 {
	if member == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.eng.Score(repute.ActorID(member), chainTrack, time.Now())
}

// TierOf maps an author-track score to a capability tier, against the live
// thresholds (seeded by HUSH_TIER1 / HUSH_TIER2, then editable at
// /admin/settings).
func TierOf(score float64, s Live) int {
	switch {
	case score >= s.Tier2:
		return 2
	case score >= s.Tier1:
		return 1
	default:
		return 0
	}
}

// InviteAllowance is the total number of codes a member may have minted, given
// their tier and chain standing. A tier-1 member starts with InviteBase codes;
// every two points of chain standing buys another, every two lost burns one — a
// chain at −2·base zeroes it. Hard-capped at InviteMax so a beloved chain can't
// mint the whole site. Below tier 1 nobody invites.
func InviteAllowance(tier int, chain float64, s Live) int {
	if tier < 1 {
		return 0
	}
	n := s.InviteBase + int(chain)/2
	if n < 0 {
		n = 0
	}
	if n > s.InviteMax {
		n = s.InviteMax
	}
	return n
}
