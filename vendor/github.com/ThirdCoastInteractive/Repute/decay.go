package repute

import (
	"math"
	"time"
)

// Decay computes the present value of an additive Delta given how much domain
// time has elapsed since the delta's source event. Decay is applied at
// projection time, so the same log yields a different "current" leaderboard at
// different as-of timestamps while remaining fully deterministic for any fixed
// as-of.
//
// Implementations must satisfy: Value(amount, 0) == amount, and Value must be
// non-increasing in |elapsed| for elapsed >= 0 (a delta never grows with age).
type Decay interface {
	// Name is the registry key referenced by Delta.Decay.
	Name() string
	// Value returns the decayed worth of `amount` after `elapsed` has passed.
	Value(amount float64, elapsed time.Duration) float64
}

// NoDecay keeps reputation forever. Lifetime-accumulation semantics.
type NoDecay struct{}

func (NoDecay) Name() string                                  { return "none" }
func (NoDecay) Value(amount float64, _ time.Duration) float64 { return amount }

// Exponential decays by a constant fraction per half-life: value = amount *
// 2^(-elapsed/HalfLife). The canonical "reflect current skill" model — old
// results fade smoothly and never fully vanish.
type Exponential struct {
	HalfLife time.Duration
	Label    string // optional registry name override
}

func (e Exponential) Name() string {
	if e.Label != "" {
		return e.Label
	}
	return "exponential"
}

func (e Exponential) Value(amount float64, elapsed time.Duration) float64 {
	if elapsed <= 0 || e.HalfLife <= 0 {
		return amount
	}
	return amount * math.Pow(2, -float64(elapsed)/float64(e.HalfLife))
}

// Linear decays to zero over Window, then stays zero. Sharp, predictable,
// "results older than Window don't count at all."
type Linear struct {
	Window time.Duration
	Label  string
}

func (l Linear) Name() string {
	if l.Label != "" {
		return l.Label
	}
	return "linear"
}

func (l Linear) Value(amount float64, elapsed time.Duration) float64 {
	if elapsed <= 0 || l.Window <= 0 {
		return amount
	}
	if elapsed >= l.Window {
		return 0
	}
	return amount * (1 - float64(elapsed)/float64(l.Window))
}

// Step keeps full value until After, then drops to Residual (a fraction in
// [0,1]) and holds. Models "counts fully for a season, then archived."
type Step struct {
	After    time.Duration
	Residual float64 // fraction retained after the cliff, 0..1
	Label    string
}

func (s Step) Name() string {
	if s.Label != "" {
		return s.Label
	}
	return "step"
}

func (s Step) Value(amount float64, elapsed time.Duration) float64 {
	if elapsed < s.After {
		return amount
	}
	return amount * s.Residual
}

// PowerLaw decays as value = amount / (1 + elapsed/Scale)^Exponent. Heavier
// early decay than exponential but a fatter tail — recent results dominate while
// distant ones still contribute a little. Good when recency matters a lot but
// you never want total erasure.
type PowerLaw struct {
	Scale    time.Duration
	Exponent float64
	Label    string
}

func (p PowerLaw) Name() string {
	if p.Label != "" {
		return p.Label
	}
	return "powerlaw"
}

func (p PowerLaw) Value(amount float64, elapsed time.Duration) float64 {
	if elapsed <= 0 || p.Scale <= 0 {
		return amount
	}
	x := 1 + float64(elapsed)/float64(p.Scale)
	exp := p.Exponent
	if exp == 0 {
		exp = 1
	}
	return amount / math.Pow(x, exp)
}

// Gaussian decays as value = amount * exp(-(elapsed^2)/(2*Sigma^2)). Stays near
// full value briefly, then falls off fast. Useful for "very recent form" tracks.
type Gaussian struct {
	Sigma time.Duration
	Label string
}

func (g Gaussian) Name() string {
	if g.Label != "" {
		return g.Label
	}
	return "gaussian"
}

func (g Gaussian) Value(amount float64, elapsed time.Duration) float64 {
	if elapsed <= 0 || g.Sigma <= 0 {
		return amount
	}
	x := float64(elapsed) / float64(g.Sigma)
	return amount * math.Exp(-(x*x)/2)
}

// Logistic holds value high, then drops through a smooth S-curve centered at
// Midpoint with steepness controlled by Steepness (larger = sharper). Models a
// grace period followed by decline.
type Logistic struct {
	Midpoint  time.Duration
	Steepness float64 // unitless; ~4..8 gives a recognizable knee
	Label     string
}

func (lg Logistic) Name() string {
	if lg.Label != "" {
		return lg.Label
	}
	return "logistic"
}

func (lg Logistic) Value(amount float64, elapsed time.Duration) float64 {
	if elapsed <= 0 || lg.Midpoint <= 0 {
		return amount
	}
	k := lg.Steepness
	if k == 0 {
		k = 6
	}
	x := float64(elapsed) / float64(lg.Midpoint)
	return amount / (1 + math.Exp(k*(x-1)))
}

// Floored wraps another Decay and guarantees the value never falls below Floor
// (as a fraction of the original amount). Use to keep a permanent residual of
// any accomplishment.
type Floored struct {
	Inner Decay
	Floor float64 // fraction in [0,1]
	Label string
}

func (f Floored) Name() string {
	if f.Label != "" {
		return f.Label
	}
	return "floored:" + f.Inner.Name()
}

func (f Floored) Value(amount float64, elapsed time.Duration) float64 {
	v := f.Inner.Value(amount, elapsed)
	floor := amount * f.Floor
	if amount >= 0 && v < floor {
		return floor
	}
	if amount < 0 && v > floor {
		return floor
	}
	return v
}

// DecayModel resolves a decay name to an implementation, with a default for the
// empty name. The engine uses it at projection time.
type DecayModel struct {
	byName map[string]Decay
	deflt  Decay
}

// NewDecayModel builds a model. The first decay given is the default (used when
// Delta.Decay is empty). Passing none yields NoDecay as default.
func NewDecayModel(decays ...Decay) *DecayModel {
	m := &DecayModel{byName: make(map[string]Decay)}
	if len(decays) == 0 {
		m.deflt = NoDecay{}
		m.byName["none"] = NoDecay{}
		return m
	}
	for i, d := range decays {
		m.byName[d.Name()] = d
		if i == 0 {
			m.deflt = d
		}
	}
	return m
}

// Register adds or overrides a decay by name.
func (m *DecayModel) Register(d Decay) { m.byName[d.Name()] = d }

// Resolve returns the decay for a name, or the default if unknown/empty.
func (m *DecayModel) Resolve(name string) Decay {
	if name == "" {
		return m.deflt
	}
	if d, ok := m.byName[name]; ok {
		return d
	}
	return m.deflt
}
