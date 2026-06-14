package scorers

import (
	"fmt"

	"github.com/ThirdCoastInteractive/Repute"
)

// Participation grants a small, fast-decaying reward for showing up — casting a
// vote, submitting a round, completing a debate. Its purpose is cold-start: it
// lets brand-new accounts accrue a little standing so they aren't invisible,
// without distorting the skill signal (pair it with a short-half-life decay).
//
// Payload: participation{Actor} or just an event whose Actors[0] is the
// participant. Add-mode.
type Participation struct {
	Track  repute.Track
	Kinds  []string // which event kinds count as participation
	Points float64  // per-event reward, default 1
	Decay  string   // strongly recommend a short-half-life decay here
}

type participationPayload struct {
	Actor repute.ActorID `json:"actor"`
}

func (p Participation) Name() string { return "participation" }

func (p Participation) track() repute.Track {
	if p.Track == "" {
		return "participation"
	}
	return p.Track
}

func (p Participation) Scorable(kind string) bool {
	for _, k := range p.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func (p Participation) Score(ev repute.Event, _ repute.View) ([]repute.Delta, error) {
	pts := p.Points
	if pts == 0 {
		pts = 1
	}
	var actor repute.ActorID
	var pp participationPayload
	if err := ev.DecodePayload(&pp); err == nil && pp.Actor != "" {
		actor = pp.Actor
	} else if len(ev.Actors) > 0 {
		actor = ev.Actors[0]
	}
	if actor == "" {
		return nil, nil
	}
	return []repute.Delta{{
		Actor: actor, Track: p.track(), Amount: pts, Mode: repute.Add, Decay: p.Decay,
		Reason: fmt.Sprintf("participation: %s", ev.Kind),
	}}, nil
}

// consensusVote is the payload for ConsensusCalibration: a single judge's
// verdict on a debate, plus the eventual resolved winner. Reward depends on
// matching the verdict AND on how early/contrarian the vote was, encoded by
// CrowdShare — the fraction of the crowd that had already agreed with this
// judge at the time the vote was cast.
type consensusVote struct {
	Judge      repute.ActorID `json:"judge"`
	Pick       string         `json:"pick"`        // judge's chosen winner id
	Winner     string         `json:"winner"`      // resolved winner id
	CrowdShare float64        `json:"crowd_share"` // fraction already agreeing, [0,1]
}

// ConsensusCalibration rewards being correct, and rewards being correct EARLY
// more than being correct late — the explicit fix for "reward agreeing with
// consensus," which otherwise breeds conformity cascades. A judge who picks the
// eventual winner while few others had yet agreed (low CrowdShare) earns a
// contrarian bonus; piling onto an already-obvious consensus earns little. Wrong
// picks are penalized regardless of crowd.
//
// reward = Base * (1 + EarlyWeight*(1 - CrowdShare))   if Pick == Winner
//
//	= -Base                                        otherwise
//
// Payload: consensusVote. Add-mode on the judge track.
type ConsensusCalibration struct {
	Track       repute.Track
	Kinds       []string
	Base        float64 // base correct reward, default 100
	EarlyWeight float64 // bonus multiplier for contrarian-correct, default 1
	Decay       string
}

func (c ConsensusCalibration) Name() string { return "consensus_calibration" }

func (c ConsensusCalibration) track() repute.Track {
	if c.Track == "" {
		return "judge"
	}
	return c.Track
}

func (c ConsensusCalibration) Scorable(kind string) bool {
	if len(c.Kinds) == 0 {
		return kind == "vote.resolved" || kind == "consensus.vote"
	}
	for _, k := range c.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func (c ConsensusCalibration) Score(ev repute.Event, _ repute.View) ([]repute.Delta, error) {
	var cv consensusVote
	if err := ev.DecodePayload(&cv); err != nil {
		return nil, err
	}
	if cv.Judge == "" {
		return nil, fmt.Errorf("consensus_calibration: needs a Judge")
	}
	base := c.Base
	if base == 0 {
		base = 100
	}
	ew := c.EarlyWeight
	if ew == 0 {
		ew = 1
	}
	var amt float64
	if cv.Pick == cv.Winner && cv.Winner != "" {
		amt = base * (1 + ew*(1-clamp01(cv.CrowdShare)))
	} else {
		amt = -base
	}
	return []repute.Delta{{
		Actor: cv.Judge, Track: c.track(), Amount: amt, Mode: repute.Add, Decay: c.Decay,
		Reason: fmt.Sprintf("consensus pick=%s winner=%s crowd=%.2f → %+.1f",
			cv.Pick, cv.Winner, cv.CrowdShare, amt),
	}}, nil
}

// WeightedWin is a simple additive head-to-head scorer for callers who want
// transparent, decaying points rather than a rating system. The winner gains
// Points scaled by margin; the loser loses a fraction. Pairs naturally with
// decay to produce a "recent form" leaderboard.
//
// Payload: match{A, B, Score}. Add-mode.
type WeightedWin struct {
	Track     repute.Track
	Kinds     []string
	Points    float64 // full-win points, default 100
	LossRatio float64 // fraction of Points the loser sheds, default 0.5
	Decay     string
}

func (w WeightedWin) Name() string { return "weighted_win" }

func (w WeightedWin) track() repute.Track {
	if w.Track == "" {
		return "debater"
	}
	return w.Track
}
func (w WeightedWin) Scorable(kind string) bool { return matchKind(w.Kinds, kind) }

func (w WeightedWin) Score(ev repute.Event, _ repute.View) ([]repute.Delta, error) {
	var m match
	if err := ev.DecodePayload(&m); err != nil {
		return nil, err
	}
	if m.A == "" || m.B == "" {
		return nil, fmt.Errorf("weighted_win: needs A and B")
	}
	pts := w.Points
	if pts == 0 {
		pts = 100
	}
	lr := w.LossRatio
	if lr == 0 {
		lr = 0.5
	}
	tr := w.track()
	s := clamp01(m.Score) // A's margin
	// Center on 0.5: margin above 0.5 rewards A, below rewards B.
	aGain := pts * 2 * (s - 0.5)       // +pts at s=1, -pts at s=0
	bGain := pts * 2 * ((1 - s) - 0.5) // mirror
	if aGain < 0 {
		aGain *= lr
	}
	if bGain < 0 {
		bGain *= lr
	}
	return []repute.Delta{
		{Actor: m.A, Track: tr, Amount: aGain, Mode: repute.Add, Decay: w.Decay,
			Reason: fmt.Sprintf("weighted_win vs %s margin=%.2f → %+.1f", m.B, s, aGain)},
		{Actor: m.B, Track: tr, Amount: bGain, Mode: repute.Add, Decay: w.Decay,
			Reason: fmt.Sprintf("weighted_win vs %s margin=%.2f → %+.1f", m.A, 1-s, bGain)},
	}, nil
}
