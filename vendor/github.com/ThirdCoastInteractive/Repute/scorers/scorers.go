// Package scorers provides ready-made repute.Scorer implementations covering
// head-to-head rating systems, proper scoring rules, and truth-serum style
// consensus mechanisms, plus participation rewards. Install whichever match
// your domain; each is independent and opt-in.
//
// Payload contract: every scorer documents the Go struct it expects in
// Event.Payload (JSON). Callers emit events whose Kind matches the scorer's
// configured kind(s) and whose Payload marshals to that struct.
package scorers

import "github.com/ThirdCoastInteractive/Repute"

// Outcome is the result of a two-party head-to-head from the first party's
// perspective.
type Outcome float64

const (
	Loss Outcome = 0.0
	Draw Outcome = 0.5
	Win  Outcome = 1.0
)

// match is the common payload for head-to-head rating scorers. A and B are the
// two participants; Score is A's outcome (Win/Draw/Loss or any value in [0,1]
// to express a margin, e.g. 0.7 for a decisive but not total win).
type match struct {
	A     repute.ActorID `json:"a"`
	B     repute.ActorID `json:"b"`
	Score float64        `json:"score"` // A's result in [0,1]
}

// verdict is the common payload for judge-calibration scorers. Judge predicted
// Probability (in [0,1]) that the eventual winner would be the affirmative side
// (or any agreed reference outcome); Resolved is the realized outcome (1 if the
// reference side won, else 0).
type verdict struct {
	Judge       repute.ActorID `json:"judge"`
	Probability float64        `json:"probability"`
	Resolved    float64        `json:"resolved"` // 0 or 1
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
