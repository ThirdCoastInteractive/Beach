package scorers

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/ThirdCoastInteractive/Repute"
)

// Glicko2 implements Glickman's Glicko-2 rating system. Unlike Elo it tracks,
// per actor, a rating deviation (RD, uncertainty) and volatility, which gives
// principled cold-start behavior and naturally widens uncertainty during
// inactivity. This makes it the recommended head-to-head system for the
// irregular, asynchronous cadence of recorded debates.
//
// The public reputation value emitted on the track is the conservative rating
// (Rating - 2*RD), so the leaderboard rewards proven performance over lucky
// streaks with high uncertainty. The full (rating, RD, volatility) triple is
// kept in internal state and exposed via Glicko2.Player.
//
// Payload: match{A, B, Score}. Stateful + Resettable.
//
// Note: textbook Glicko-2 batches a rating period's games before updating. This
// implementation updates per match (rating period = one game), which is the
// common online variant and keeps the engine's per-event model intact.
type Glicko2 struct {
	Track repute.Track
	Kinds []string

	// Tau constrains volatility change; smaller = more stable. 0.3..1.2 typical.
	Tau float64
	// InitialRating, InitialRD, InitialVol seed unseen players. Defaults follow
	// Glickman: 1500, 350, 0.06.
	InitialRating float64
	InitialRD     float64
	InitialVol    float64

	players map[repute.ActorID]g2player
}

type g2player struct {
	r   float64 // rating (Glicko scale)
	rd  float64 // rating deviation
	vol float64 // volatility
}

const g2scale = 173.7178 // conversion between Glicko and Glicko-2 scales

func (g *Glicko2) Name() string { return "glicko2" }

func (g *Glicko2) Reset() { g.players = nil }

func (g *Glicko2) track() repute.Track {
	if g.Track == "" {
		return "debater"
	}
	return g.Track
}

func (g *Glicko2) Scorable(kind string) bool { return matchKind(g.Kinds, kind) }

func (g *Glicko2) defaults() (tau, r, rd, vol float64) {
	tau = g.Tau
	if tau == 0 {
		tau = 0.5
	}
	r = g.InitialRating
	if r == 0 {
		r = 1500
	}
	rd = g.InitialRD
	if rd == 0 {
		rd = 350
	}
	vol = g.InitialVol
	if vol == 0 {
		vol = 0.06
	}
	return
}

func (g *Glicko2) get(a repute.ActorID) g2player {
	if g.players == nil {
		g.players = make(map[repute.ActorID]g2player)
	}
	if p, ok := g.players[a]; ok {
		return p
	}
	_, r, rd, vol := g.defaults()
	return g2player{r: r, rd: rd, vol: vol}
}

// Player returns the current (rating, RD, volatility) triple for an actor on
// the Glicko scale, and whether the actor has been seen.
func (g *Glicko2) Player(a repute.ActorID) (rating, rd, vol float64, seen bool) {
	if g.players == nil {
		return 0, 0, 0, false
	}
	p, ok := g.players[a]
	return p.r, p.rd, p.vol, ok
}

type g2snap struct {
	R   float64 `json:"r"`
	RD  float64 `json:"rd"`
	Vol float64 `json:"vol"`
}

func (g *Glicko2) ScorerSnapshot() (json.RawMessage, error) {
	m := make(map[string]g2snap, len(g.players))
	for k, v := range g.players {
		m[string(k)] = g2snap{R: v.r, RD: v.rd, Vol: v.vol}
	}
	return json.Marshal(m)
}

func (g *Glicko2) LoadScorerSnapshot(data json.RawMessage) error {
	var m map[string]g2snap
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	g.players = make(map[repute.ActorID]g2player, len(m))
	for k, v := range m {
		g.players[repute.ActorID(k)] = g2player{r: v.R, rd: v.RD, vol: v.Vol}
	}
	return nil
}

func (g *Glicko2) Score(ev repute.Event, _ repute.View) ([]repute.Delta, error) {
	var m match
	if err := ev.DecodePayload(&m); err != nil {
		return nil, err
	}
	if m.A == "" || m.B == "" {
		return nil, fmt.Errorf("glicko2: match needs both A and B")
	}
	tau, _, _, _ := g.defaults()

	pa := g.get(m.A)
	pb := g.get(m.B)
	sa := clamp01(m.Score)

	na := g2update(pa, pb, sa, tau)
	nb := g2update(pb, pa, 1-sa, tau)

	g.players[m.A] = na
	g.players[m.B] = nb

	// Conservative public rating: rating - 2*RD.
	va := na.r - 2*na.rd
	vb := nb.r - 2*nb.rd
	tr := g.track()
	return []repute.Delta{
		{Actor: m.A, Track: tr, Amount: va, Mode: repute.Set,
			Reason: fmt.Sprintf("glicko2 r=%.0f rd=%.0f vol=%.3f vs %s (S=%.2f)", na.r, na.rd, na.vol, m.B, sa)},
		{Actor: m.B, Track: tr, Amount: vb, Mode: repute.Set,
			Reason: fmt.Sprintf("glicko2 r=%.0f rd=%.0f vol=%.3f vs %s (S=%.2f)", nb.r, nb.rd, nb.vol, m.A, 1-sa)},
	}, nil
}

// g2update runs one Glicko-2 step for player p against opponent o with score s.
func g2update(p, o g2player, s, tau float64) g2player {
	// Step 2: to Glicko-2 scale.
	mu := (p.r - 1500) / g2scale
	phi := p.rd / g2scale
	muO := (o.r - 1500) / g2scale
	phiO := o.rd / g2scale

	gPhiO := 1 / math.Sqrt(1+3*phiO*phiO/(math.Pi*math.Pi))
	eVal := 1 / (1 + math.Exp(-gPhiO*(mu-muO)))

	// Step 3: variance.
	vInv := gPhiO * gPhiO * eVal * (1 - eVal)
	v := 1 / vInv

	// Step 4: delta.
	delta := v * gPhiO * (s - eVal)

	// Step 5: new volatility via iteration (Illinois algorithm).
	a := math.Log(p.vol * p.vol)
	f := func(x float64) float64 {
		ex := math.Exp(x)
		num := ex * (delta*delta - phi*phi - v - ex)
		den := 2 * (phi*phi + v + ex) * (phi*phi + v + ex)
		return num/den - (x-a)/(tau*tau)
	}
	var A, B float64
	A = a
	if delta*delta > phi*phi+v {
		B = math.Log(delta*delta - phi*phi - v)
	} else {
		k := 1.0
		for f(a-k*tau) < 0 {
			k++
		}
		B = a - k*tau
	}
	fA, fB := f(A), f(B)
	const eps = 1e-6
	for math.Abs(B-A) > eps {
		C := A + (A-B)*fA/(fB-fA)
		fC := f(C)
		if fC*fB <= 0 {
			A, fA = B, fB
		} else {
			fA /= 2
		}
		B, fB = C, fC
	}
	newVol := math.Exp(A / 2)

	// Step 6: pre-rating-period RD.
	phiStar := math.Sqrt(phi*phi + newVol*newVol)

	// Step 7: new RD and rating.
	newPhi := 1 / math.Sqrt(1/(phiStar*phiStar)+vInv)
	newMu := mu + newPhi*newPhi*gPhiO*(s-eVal)

	// Back to Glicko scale.
	return g2player{
		r:   newMu*g2scale + 1500,
		rd:  newPhi * g2scale,
		vol: newVol,
	}
}
