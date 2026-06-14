package scorers

import (
	"fmt"
	"math"

	"github.com/ThirdCoastInteractive/Repute"
)

// ProperScore is a family of strictly proper scoring rules that reward a judge
// for being well-calibrated against the realized verdict. Each judge states a
// Probability that a reference outcome will occur; when the outcome Resolves,
// the rule pays more for confident-correct predictions and penalizes
// confident-wrong ones. Hedging at 0.5 earns near zero. This is the "calibrated,
// not conformist" track that avoids the consensus beauty-contest trap, because
// reward is tied to truth, not to agreeing with the crowd.
//
// Payload: verdict{Judge, Probability, Resolved}. Add-mode, accumulates on the
// judge track. Choose a Rule:
//
//   - Brier:        score = 1 - (p - o)^2 * 2, in [-1, 1]; gentle, bounded.
//   - Logarithmic:  score = log2(p if o==1 else 1-p), in (-inf, 0]; the most
//     theoretically pure rule, but unbounded penalty for confident-wrong.
//     Clamped at MinProb to bound the worst case.
//   - Spherical:    score = (p if o==1 else 1-p)/sqrt(p^2+(1-p)^2); bounded,
//     less harsh than log.
//
// Scale multiplies the raw score into reputation points (default 100).
type ProperScore struct {
	Track   repute.Track
	Kinds   []string
	Rule    Rule
	Scale   float64 // points multiplier, default 100
	MinProb float64 // clamp for log rule, default 0.01
	Decay   string  // optional decay name for emitted deltas
}

// Rule selects a proper scoring rule.
type Rule uint8

const (
	Brier Rule = iota
	Logarithmic
	Spherical
)

func (r Rule) String() string {
	switch r {
	case Brier:
		return "brier"
	case Logarithmic:
		return "log"
	case Spherical:
		return "spherical"
	default:
		return "unknown"
	}
}

func (p ProperScore) Name() string { return "properscore:" + p.Rule.String() }

func (p ProperScore) track() repute.Track {
	if p.Track == "" {
		return "judge"
	}
	return p.Track
}

func (p ProperScore) Scorable(kind string) bool {
	if len(p.Kinds) == 0 {
		return kind == "vote.resolved" || kind == "verdict.scored"
	}
	for _, k := range p.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func (p ProperScore) Score(ev repute.Event, _ repute.View) ([]repute.Delta, error) {
	var vd verdict
	if err := ev.DecodePayload(&vd); err != nil {
		return nil, err
	}
	if vd.Judge == "" {
		return nil, fmt.Errorf("properscore: verdict needs a Judge")
	}
	prob := clamp01(vd.Probability)
	o := vd.Resolved // expected 0 or 1
	scale := p.Scale
	if scale == 0 {
		scale = 100
	}

	var raw float64
	switch p.Rule {
	case Logarithmic:
		minp := p.MinProb
		if minp == 0 {
			minp = 0.01
		}
		pp := math.Max(minp, math.Min(1-minp, prob))
		if o >= 0.5 {
			raw = math.Log2(pp) + 1 // shift so p=0.5 -> 0, p=1 -> 1
		} else {
			raw = math.Log2(1-pp) + 1
		}
	case Spherical:
		norm := math.Sqrt(prob*prob + (1-prob)*(1-prob))
		if o >= 0.5 {
			raw = prob/norm - math.Sqrt2/2 // center so 0.5 -> 0
		} else {
			raw = (1-prob)/norm - math.Sqrt2/2
		}
		raw /= (1 - math.Sqrt2/2) // rescale max to 1
	default: // Brier
		// (p-o)^2 in [0,1]; map to [-1,1] reward, 0.5 guess -> 0.5 loss... use
		// 1 - 2*(p-o)^2 so perfect=1, p=0.5 -> 0.5, worst=-1.
		raw = 1 - 2*math.Pow(prob-o, 2)
	}

	return []repute.Delta{{
		Actor: vd.Judge, Track: p.track(), Amount: raw * scale, Mode: repute.Add,
		Decay:  p.Decay,
		Reason: fmt.Sprintf("%s p=%.2f o=%.0f → %+.1f", p.Rule, prob, o, raw*scale),
	}}, nil
}
