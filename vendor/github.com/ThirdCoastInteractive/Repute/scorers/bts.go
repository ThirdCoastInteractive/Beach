package scorers

import (
	"fmt"
	"math"

	"github.com/ThirdCoastInteractive/Repute"
)

// btsBallot is the payload for truth-serum mechanisms. Each judge supplies their
// own answer plus a prediction of how the population will answer. For the binary
// case used here, Answer is 0 or 1 and Prediction is the judge's estimate of the
// fraction of others who will answer 1.
type btsBallot struct {
	Judge      repute.ActorID `json:"judge"`
	Answer     int            `json:"answer"`     // 0 or 1
	Prediction float64        `json:"prediction"` // P(others answer 1), in [0,1]
}

// btsRound is the payload for a resolved poll: all ballots cast for one
// question. The scorer computes information/prediction scores across the whole
// population at once, so it fires on a single "round resolved" event rather than
// per ballot.
type btsRound struct {
	Question string      `json:"question"`
	Ballots  []btsBallot `json:"ballots"`
}

// BTS implements Prelec's Bayesian Truth Serum for binary answers. It rewards an
// answer that is more common than the population collectively predicted — the
// "surprisingly popular" signal — which is provably truth-inducing and is the
// principled antidote to rewarding mere conformity. A judge who holds private
// information that the crowd underweights scores highly even against the
// majority; a judge who simply follows the expected crowd does not.
//
// Information score (per judge for binary answer x_k):
//
//	log( x̄_k / ȳ_k )
//
// where x̄_k is the empirical frequency of the judge's answer and ȳ_k is the
// geometric-mean predicted frequency of that answer. Prediction score rewards
// accurate forecasts of the population via a KL term. Total = info + Alpha*pred.
//
// Payload: btsRound{Question, Ballots}. Add-mode on the judge track. Requires a
// genuine population (>= 2 ballots) to be meaningful.
type BTS struct {
	Track repute.Track
	Kinds []string
	Alpha float64 // weight on prediction score, default 1
	Scale float64 // points multiplier, default 100
	Decay string
}

func (b BTS) Name() string { return "bts" }

func (b BTS) track() repute.Track {
	if b.Track == "" {
		return "judge"
	}
	return b.Track
}

func (b BTS) Scorable(kind string) bool {
	if len(b.Kinds) == 0 {
		return kind == "poll.resolved" || kind == "bts.round"
	}
	for _, k := range b.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func (b BTS) Score(ev repute.Event, _ repute.View) ([]repute.Delta, error) {
	var round btsRound
	if err := ev.DecodePayload(&round); err != nil {
		return nil, err
	}
	n := len(round.Ballots)
	if n < 2 {
		return nil, nil // not enough population to score
	}
	alpha := b.Alpha
	if alpha == 0 {
		alpha = 1
	}
	scale := b.Scale
	if scale == 0 {
		scale = 100
	}

	// Empirical answer frequencies x̄ (fraction answering 1, and 0).
	var sumAns float64
	// Geometric mean of predictions, computed in log space, for answer "1".
	var logPredOne, logPredZero float64
	for _, bal := range round.Ballots {
		sumAns += float64(bal.Answer)
		p := clampEps(bal.Prediction)
		logPredOne += math.Log(p)
		logPredZero += math.Log(1 - p)
	}
	x1 := sumAns / float64(n)
	x0 := 1 - x1
	geoOne := math.Exp(logPredOne / float64(n))
	geoZero := math.Exp(logPredZero / float64(n))

	x1c, x0c := clampEps(x1), clampEps(x0)
	geoOnec, geoZeroc := clampEps(geoOne), clampEps(geoZero)

	out := make([]repute.Delta, 0, n)
	for _, bal := range round.Ballots {
		// Information score for the judge's chosen answer.
		var info float64
		if bal.Answer == 1 {
			info = math.Log(x1c / geoOnec)
		} else {
			info = math.Log(x0c / geoZeroc)
		}
		// Prediction score: penalize divergence of the judge's forecast from the
		// empirical distribution (relative entropy form, sign-flipped to reward).
		pj := clampEps(bal.Prediction)
		pred := x1c*math.Log(pj/x1c) + x0c*math.Log((1-pj)/x0c)
		total := (info + alpha*pred) * scale
		out = append(out, repute.Delta{
			Actor: bal.Judge, Track: b.track(), Amount: total, Mode: repute.Add,
			Decay: b.Decay,
			Reason: fmt.Sprintf("bts info=%.3f pred=%.3f ans=%d → %+.1f",
				info, pred, bal.Answer, total),
		})
	}
	return out, nil
}

// SurprisinglyPopular implements the lighter-weight Prelec/Seung/McCoy
// surprisingly-popular decision rule as a reward: the answer whose actual
// frequency most exceeds its predicted frequency is treated as correct, and
// judges who selected it are rewarded (others penalized). Use when you want the
// SP signal but not the full BTS per-judge information score.
//
// Payload: btsRound{Question, Ballots}. Add-mode on the judge track.
type SurprisinglyPopular struct {
	Track  repute.Track
	Kinds  []string
	Reward float64 // points for picking the SP answer, default 100
	Decay  string
}

func (s SurprisinglyPopular) Name() string { return "surprisingly_popular" }

func (s SurprisinglyPopular) track() repute.Track {
	if s.Track == "" {
		return "judge"
	}
	return s.Track
}

func (s SurprisinglyPopular) Scorable(kind string) bool {
	if len(s.Kinds) == 0 {
		return kind == "poll.resolved" || kind == "bts.round"
	}
	for _, k := range s.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func (s SurprisinglyPopular) Score(ev repute.Event, _ repute.View) ([]repute.Delta, error) {
	var round btsRound
	if err := ev.DecodePayload(&round); err != nil {
		return nil, err
	}
	n := len(round.Ballots)
	if n < 2 {
		return nil, nil
	}
	reward := s.Reward
	if reward == 0 {
		reward = 100
	}
	var sumAns, sumPred float64
	for _, bal := range round.Ballots {
		sumAns += float64(bal.Answer)
		sumPred += clamp01(bal.Prediction)
	}
	actualOne := sumAns / float64(n)
	predOne := sumPred / float64(n)
	// SP answer is whichever answer's actual frequency exceeds its predicted
	// frequency. For binary, that is answer 1 iff actualOne > predOne.
	spAnswer := 0
	if actualOne > predOne {
		spAnswer = 1
	}
	out := make([]repute.Delta, 0, n)
	for _, bal := range round.Ballots {
		amt := -reward
		if bal.Answer == spAnswer {
			amt = reward
		}
		out = append(out, repute.Delta{
			Actor: bal.Judge, Track: s.track(), Amount: amt, Mode: repute.Add,
			Decay: s.Decay,
			Reason: fmt.Sprintf("SP answer=%d (actual=%.2f pred=%.2f) judge=%d → %+.0f",
				spAnswer, actualOne, predOne, bal.Answer, amt),
		})
	}
	return out, nil
}

func clampEps(x float64) float64 {
	const eps = 1e-9
	if x < eps {
		return eps
	}
	if x > 1-eps {
		return 1 - eps
	}
	return x
}
