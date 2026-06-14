package scorers

import (
	"fmt"
	"math"

	"github.com/ThirdCoastInteractive/Repute"
)

// Elo implements the classic Elo rating system as a stateful Set-mode scorer.
// Each match updates both participants' ratings toward the result, scaled by
// K. Ratings start at Initial for unseen actors. Margin is supported by passing
// a fractional Score in the match payload.
//
// Payload: match{A, B, Score}. Track: configurable, default "debater".
//
// Elo is simple and well understood but has no notion of rating uncertainty;
// prefer Glicko2 for irregular, asynchronous play. Provided because many
// callers want the familiar number.
type Elo struct {
	Track   repute.Track
	Kinds   []string
	K       float64 // update step, default 32
	Initial float64 // starting rating, default 1500
}

func (e Elo) Name() string { return "elo" }

func (e Elo) track() repute.Track {
	if e.Track == "" {
		return "debater"
	}
	return e.Track
}

func (e Elo) Scorable(kind string) bool { return matchKind(e.Kinds, kind) }

func (e Elo) Score(ev repute.Event, v repute.View) ([]repute.Delta, error) {
	var m match
	if err := ev.DecodePayload(&m); err != nil {
		return nil, err
	}
	if m.A == "" || m.B == "" {
		return nil, fmt.Errorf("elo: match needs both A and B")
	}
	k := e.K
	if k == 0 {
		k = 32
	}
	init := e.Initial
	if init == 0 {
		init = 1500
	}
	tr := e.track()

	ra := ratingOr(v, m.A, tr, init)
	rb := ratingOr(v, m.B, tr, init)

	ea := 1 / (1 + math.Pow(10, (rb-ra)/400))
	eb := 1 - ea
	sa := clamp01(m.Score)
	sb := 1 - sa

	na := ra + k*(sa-ea)
	nb := rb + k*(sb-eb)

	return []repute.Delta{
		{Actor: m.A, Track: tr, Amount: na, Mode: repute.Set,
			Reason: fmt.Sprintf("elo %.0f→%.0f vs %s (S=%.2f, E=%.2f)", ra, na, m.B, sa, ea)},
		{Actor: m.B, Track: tr, Amount: nb, Mode: repute.Set,
			Reason: fmt.Sprintf("elo %.0f→%.0f vs %s (S=%.2f, E=%.2f)", rb, nb, m.A, sb, eb)},
	}, nil
}

// ratingOr returns the actor's Set rating on track, or fallback if unrated.
func ratingOr(v repute.View, a repute.ActorID, t repute.Track, fallback float64) float64 {
	if r, ok := v.Rating(a, t); ok {
		return r
	}
	return fallback
}

func matchKind(kinds []string, kind string) bool {
	if len(kinds) == 0 {
		return kind == "match" || kind == "debate.resolved"
	}
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}
