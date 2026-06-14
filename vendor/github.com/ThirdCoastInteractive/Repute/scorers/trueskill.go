package scorers

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/ThirdCoastInteractive/Repute"
)

// TrueSkill implements a 1v1 form of Microsoft's TrueSkill: each actor's skill
// is a Gaussian (mu, sigma); matches update both via the standard factor-graph
// equations for the two-player case. Like Glicko-2 it models uncertainty, but
// the math comes from a Bayesian skill model and tends to converge faster.
// Draws are supported via the DrawProbability margin.
//
// The public reputation value is the conservative skill estimate mu - 3*sigma
// (TrueSkill's own leaderboard convention). Full (mu, sigma) is kept internally
// and exposed via TrueSkill.Player.
//
// Payload: match{A, B, Score} where Score==Draw (0.5) is treated as a draw and
// any other value as a win for the higher-scoring side. Stateful + Resettable.
type TrueSkill struct {
	Track repute.Track
	Kinds []string

	Mu              float64 // initial mean, default 25
	Sigma           float64 // initial std, default 25/3
	Beta            float64 // skill-to-performance noise, default Sigma/2
	Tau             float64 // dynamics (per-match mu drift), default Sigma/100
	DrawProbability float64 // default 0.10

	players map[repute.ActorID]tsplayer
}

type tsplayer struct {
	mu, sigma float64
}

func (t *TrueSkill) Name() string { return "trueskill" }
func (t *TrueSkill) Reset()       { t.players = nil }

func (t *TrueSkill) track() repute.Track {
	if t.Track == "" {
		return "debater"
	}
	return t.Track
}
func (t *TrueSkill) Scorable(kind string) bool { return matchKind(t.Kinds, kind) }

func (t *TrueSkill) defaults() (mu, sigma, beta, tau, draw float64) {
	mu = t.Mu
	if mu == 0 {
		mu = 25
	}
	sigma = t.Sigma
	if sigma == 0 {
		sigma = 25.0 / 3.0
	}
	beta = t.Beta
	if beta == 0 {
		beta = sigma / 2
	}
	tau = t.Tau
	if tau == 0 {
		tau = sigma / 100
	}
	draw = t.DrawProbability
	if draw == 0 {
		draw = 0.10
	}
	return
}

func (t *TrueSkill) get(a repute.ActorID) tsplayer {
	if t.players == nil {
		t.players = make(map[repute.ActorID]tsplayer)
	}
	if p, ok := t.players[a]; ok {
		return p
	}
	mu, sigma, _, _, _ := t.defaults()
	return tsplayer{mu: mu, sigma: sigma}
}

// Player returns the (mu, sigma) skill estimate and whether the actor is known.
func (t *TrueSkill) Player(a repute.ActorID) (mu, sigma float64, seen bool) {
	if t.players == nil {
		return 0, 0, false
	}
	p, ok := t.players[a]
	return p.mu, p.sigma, ok
}

type tsSnap struct {
	Mu    float64 `json:"mu"`
	Sigma float64 `json:"sigma"`
}

func (t *TrueSkill) ScorerSnapshot() (json.RawMessage, error) {
	m := make(map[string]tsSnap, len(t.players))
	for k, v := range t.players {
		m[string(k)] = tsSnap{Mu: v.mu, Sigma: v.sigma}
	}
	return json.Marshal(m)
}

func (t *TrueSkill) LoadScorerSnapshot(data json.RawMessage) error {
	var m map[string]tsSnap
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	t.players = make(map[repute.ActorID]tsplayer, len(m))
	for k, v := range m {
		t.players[repute.ActorID(k)] = tsplayer{mu: v.Mu, sigma: v.Sigma}
	}
	return nil
}

func (t *TrueSkill) Score(ev repute.Event, _ repute.View) ([]repute.Delta, error) {
	var m match
	if err := ev.DecodePayload(&m); err != nil {
		return nil, err
	}
	if m.A == "" || m.B == "" {
		return nil, fmt.Errorf("trueskill: match needs both A and B")
	}
	_, _, beta, tau, drawProb := t.defaults()

	pa := t.get(m.A)
	pb := t.get(m.B)

	// Add dynamics noise.
	pa.sigma = math.Sqrt(pa.sigma*pa.sigma + tau*tau)
	pb.sigma = math.Sqrt(pb.sigma*pb.sigma + tau*tau)

	// Determine winner ordering. Score>0.5 => A wins, <0.5 => B wins, ==0.5 draw.
	draw := math.Abs(m.Score-0.5) < 1e-9
	winner, loser := pa, pb
	swapped := false
	if m.Score < 0.5 {
		winner, loser = pb, pa
		swapped = true
	}

	c := math.Sqrt(2*beta*beta + winner.sigma*winner.sigma + loser.sigma*loser.sigma)
	dMu := (winner.mu - loser.mu) / c
	drawMargin := drawMarginFor(drawProb, beta)
	eps := drawMargin / c

	var vWin, wWin float64
	if draw {
		vWin = vDraw(dMu, eps)
		wWin = wDraw(dMu, eps)
	} else {
		vWin = vWinFn(dMu, eps)
		wWin = wWinFn(dMu, eps)
	}

	// Update winner.
	winMuMul := winner.sigma * winner.sigma / c
	winner.mu += winMuMul * vWin
	winner.sigma *= math.Sqrt(1 - (winner.sigma*winner.sigma/(c*c))*wWin)

	// Update loser (mirror sign on v).
	loseMuMul := loser.sigma * loser.sigma / c
	loser.mu -= loseMuMul * vWin
	loser.sigma *= math.Sqrt(1 - (loser.sigma*loser.sigma/(c*c))*wWin)

	if swapped {
		pa, pb = loser, winner
	} else {
		pa, pb = winner, loser
	}
	t.players[m.A] = pa
	t.players[m.B] = pb

	tr := t.track()
	return []repute.Delta{
		{Actor: m.A, Track: tr, Amount: pa.mu - 3*pa.sigma, Mode: repute.Set,
			Reason: fmt.Sprintf("trueskill mu=%.2f sigma=%.2f vs %s", pa.mu, pa.sigma, m.B)},
		{Actor: m.B, Track: tr, Amount: pb.mu - 3*pb.sigma, Mode: repute.Set,
			Reason: fmt.Sprintf("trueskill mu=%.2f sigma=%.2f vs %s", pb.mu, pb.sigma, m.A)},
	}, nil
}

func normPdf(x float64) float64 { return math.Exp(-x*x/2) / math.Sqrt(2*math.Pi) }
func normCdf(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }

func drawMarginFor(drawProb, beta float64) float64 {
	// Inverse-cdf approximation: margin = sqrt2*beta * Phi^{-1}((p+1)/2).
	return math.Sqrt2 * beta * normPpf((drawProb+1)/2)
}

func vWinFn(t, eps float64) float64 {
	denom := normCdf(t - eps)
	if denom < 1e-12 {
		return -t + eps
	}
	return normPdf(t-eps) / denom
}
func wWinFn(t, eps float64) float64 {
	v := vWinFn(t, eps)
	return v * (v + t - eps)
}
func vDraw(t, eps float64) float64 {
	denom := normCdf(eps-t) - normCdf(-eps-t)
	if math.Abs(denom) < 1e-12 {
		if t < 0 {
			return -t - eps
		}
		return -t + eps
	}
	return (normPdf(-eps-t) - normPdf(eps-t)) / denom
}
func wDraw(t, eps float64) float64 {
	denom := normCdf(eps-t) - normCdf(-eps-t)
	if math.Abs(denom) < 1e-12 {
		return 1
	}
	v := vDraw(t, eps)
	return v*v + ((eps-t)*normPdf(eps-t)-(-eps-t)*normPdf(-eps-t))/denom
}

// normPpf is the inverse standard-normal CDF (Acklam's rational approximation).
func normPpf(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	a := []float64{-3.969683028665376e+01, 2.209460984245205e+02, -2.759285104469687e+02, 1.383577518672690e+02, -3.066479806614716e+01, 2.506628277459239e+00}
	b := []float64{-5.447609879822406e+01, 1.615858368580409e+02, -1.556989798598866e+02, 6.680131188771972e+01, -1.328068155288572e+01}
	c := []float64{-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e+00, -2.549732539343734e+00, 4.374664141464968e+00, 2.938163982698783e+00}
	d := []float64{7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e+00, 3.754408661907416e+00}
	plow := 0.02425
	phigh := 1 - plow
	switch {
	case p < plow:
		q := math.Sqrt(-2 * math.Log(p))
		return (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	case p <= phigh:
		q := p - 0.5
		r := q * q
		return (((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q /
			(((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
	default:
		q := math.Sqrt(-2 * math.Log(1-p))
		return -(((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	}
}
