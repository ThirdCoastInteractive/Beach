package color

import "math"

// The solver.
//
// Deriving a palette means answering the same question about twenty different
// tokens: "what is the colour of this hue that meets its contrast obligation
// against that background, with as much of the hue left in it as possible?"
//
// In OKLCH that question has a closed shape. Contrast is a ratio of relative
// luminances, luminance rises monotonically with L at a fixed hue and chroma
// fraction, and so contrast-against-a-fixed-background is monotonic in L too.
// A monotonic predicate bisects: twenty-odd iterations, no scanning.
//
// The framework's previous attempt walked the lightness axis in 0.002 steps and
// tested every stop, which cost about 4.5ms per theme. These bisect in tens of
// microseconds, which is what makes a theme cheap enough to derive at runtime
// rather than at build time.

// Solve finds the colour of hue h that meets ratio against bg, taking as much
// chroma as sRGB allows at whatever lightness it lands on.
//
// lighter picks the side of the background to search: true for ink on a dark
// surface, false for ink on a light one. The result is the lightness *closest to
// the background* that still clears the ratio — the most recessive legal
// answer, which is what a muted stop or a hairline edge wants. For a stop that
// should be as loud as possible, see [SolveVivid].
//
// chromaPct is the fraction of the hue's maximum chroma to use, 0–1. It is a
// fraction rather than an absolute because maximum chroma varies by nearly 3×
// across hues (see [MaxChroma]), so an absolute value makes some hues shout and
// others look dirty.
//
// ok is false when no lightness on that side clears the ratio — the honest
// answer when a caller asks a light-grey background for 4.5:1 of yellow.
func Solve(bg OKLCH, h, chromaPct, ratio float64, lighter bool) (c OKLCH, ok bool) {
	return solveWith(bg, ratio, lighter, func(l float64) OKLCH {
		return OKLCH{L: l, C: chromaPct * MaxChroma(l, h), H: h}
	})
}

// SolveFixed is [Solve] at a constant absolute chroma rather than a fraction of
// the hue's maximum.
//
// It is what the neutral ladder wants. A surface or a muted ink carries a
// *whisper* of colour — a few thousandths — and that whisper should be the same
// small amount at every rung. Asking for a fraction of the maximum instead would
// make the mid-tones markedly more colourful than the ends, which is precisely
// how a warm-grey ladder turns into a tan one.
func SolveFixed(bg OKLCH, h, chroma, ratio float64, lighter bool) (c OKLCH, ok bool) {
	return solveWith(bg, ratio, lighter, func(l float64) OKLCH {
		return OKLCH{L: l, C: chroma, H: h}.Clamp()
	})
}

// solveWith is the bisection both solvers share. at maps a lightness to the
// candidate colour, which is the only thing that differs between them.
func solveWith(bg OKLCH, ratio float64, lighter bool, at func(float64) OKLCH) (OKLCH, bool) {
	// The extreme end of the search range: if even white (or black) cannot make
	// the ratio, nothing on this side will, and there is no point bisecting.
	limit := 1.0
	if !lighter {
		limit = 0.0
	}
	if Contrast(at(limit), bg) < ratio {
		return OKLCH{}, false
	}

	// Bisect for the boundary. Invariant: `limit` clears the ratio, `near` does
	// not — so the crossing lies between them, and the answer is the value on
	// the clearing side once the interval is narrower than 8-bit output can show.
	near := bg.L
	for range 24 {
		mid := (near + limit) / 2
		if Contrast(at(mid), bg) >= ratio {
			limit = mid
		} else {
			near = mid
		}
	}
	return at(limit), true
}

// Need is one contrast obligation: a backdrop this colour will be seen against,
// and the ratio it owes there.
//
// Obligations are per-pair, not per-colour, and the ratios genuinely differ — an
// accent fill owes its own label 4.5:1 as text (SC 1.4.3) while owing a card
// only 3:1 as a focus indicator (SC 1.4.11). Collapsing that to a single ratio
// either over-constrains the colour and drains it, or under-constrains it and
// ships something unreadable.
type Need struct {
	Bg    OKLCH
	Ratio float64
}

// Text and NonText build the two obligations the framework uses, so a caller
// spells out which criterion it is invoking rather than repeating a bare number.
func Text(bg OKLCH) Need    { return Need{bg, AAText} }
func NonText(bg OKLCH) Need { return Need{bg, AANonText} }

// SolveVivid finds the colour of hue h that satisfies every obligation and
// carries the most chroma, rather than the least lightness.
//
// It is the objective for anything meant to be *seen* — the accent, the four
// status roles, the category colours. Solved for recession instead, a status
// colour spends all of its colour on the ratio and arrives as olive-for-good and
// mud-for-warning: technically legible, visually dead. Presence and recession
// are genuinely different goals, and a palette needs both.
//
// Two facts make this a search rather than a scan. Contrast against a fixed set
// of backdrops is monotonic in L, so the feasible lightnesses are a single
// interval and its edge bisects. And maximum chroma is unimodal in L — it rises
// from zero at black to a cusp and falls to zero at white — so the best point in
// that interval is a ternary search. Scanning the axis instead costs about
// fifteen times as much for an answer no different at eight bits per channel.
func SolveVivid(needs []Need, h, chromaPct float64) (c OKLCH, ok bool) {
	at := func(l float64) OKLCH {
		return OKLCH{L: l, C: chromaPct * MaxChroma(l, h), H: h}
	}

	// Which way feasibility lies: if the light end satisfies the obligations,
	// the feasible interval runs up from a lower bound, and vice versa.
	lightSide := Satisfies(at(0.98), needs)
	if !lightSide && !Satisfies(at(0.02), needs) {
		return OKLCH{}, false
	}

	// Bisect for the edge of the feasible interval.
	good, bad := 0.98, 0.02
	if !lightSide {
		good, bad = 0.02, 0.98
	}
	for range 20 {
		mid := (good + bad) / 2
		if Satisfies(at(mid), needs) {
			good = mid
		} else {
			bad = mid
		}
	}

	// Ternary-search the feasible interval for the chroma cusp.
	lo, hi := good, 0.98
	if !lightSide {
		lo, hi = 0.02, good
	}
	for range 24 {
		a := lo + (hi-lo)/3
		b := hi - (hi-lo)/3
		if MaxChroma(a, h) < MaxChroma(b, h) {
			lo = a
		} else {
			hi = b
		}
	}
	best := at((lo + hi) / 2)
	if !Satisfies(best, needs) {
		// The cusp sits outside the feasible interval; the boundary is then the
		// most colourful legal point, since chroma is monotonic up to the cusp.
		best = at(good)
	}
	if !Satisfies(best, needs) {
		return OKLCH{}, false
	}
	return best, true
}

// SolveAgainstAll is [Solve] against several obligations at once: the same
// colour has to hold up on the page, on a card and on a raised panel, and the
// binding constraint is whichever backdrop is hardest.
func SolveAgainstAll(needs []Need, h, chromaPct float64, lighter bool) (OKLCH, bool) {
	if len(needs) == 0 {
		return OKLCH{}, false
	}
	// Try each obligation as the binding one and keep the most recessive result
	// that satisfies them all. Guessing the hardest backdrop by lightness alone
	// is wrong once the ratios differ — a nearer surface asking for less can be
	// easier than a further one asking for more.
	return againstAll(needs, lighter, func(bg OKLCH, ratio float64) (OKLCH, bool) {
		return Solve(bg, h, chromaPct, ratio, lighter)
	})
}

// SolveAgainstAllFixed is [SolveAgainstAll] at a constant absolute chroma — the
// neutral-ladder counterpart, for a muted ink or an input edge that has to hold
// up on the page, on a card and on a raised panel at once.
func SolveAgainstAllFixed(needs []Need, h, chroma float64, lighter bool) (OKLCH, bool) {
	if len(needs) == 0 {
		return OKLCH{}, false
	}
	return againstAll(needs, lighter, func(bg OKLCH, ratio float64) (OKLCH, bool) {
		return SolveFixed(bg, h, chroma, ratio, lighter)
	})
}

// againstAll tries each obligation as the binding one and keeps the most
// recessive result that satisfies them all.
//
// Guessing the hardest backdrop by lightness alone is wrong once the ratios
// differ: a nearer surface asking for 4.5:1 can bind harder than a further one
// asking for 3:1, and picking by lightness would quietly under-solve it.
func againstAll(needs []Need, lighter bool, solve func(OKLCH, float64) (OKLCH, bool)) (OKLCH, bool) {
	var best OKLCH
	found := false
	for _, n := range needs {
		c, ok := solve(n.Bg, n.Ratio)
		if !ok || !Satisfies(c, needs) {
			continue
		}
		// More recessive means closer to the backdrops, which on the lighter
		// side means the lower lightness.
		if !found || (lighter && c.L < best.L) || (!lighter && c.L > best.L) {
			best, found = c, true
		}
	}
	return best, found
}

// Satisfies reports whether c meets every obligation.
func Satisfies(c OKLCH, needs []Need) bool {
	for _, n := range needs {
		if Contrast(c, n.Bg) < n.Ratio {
			return false
		}
	}
	return true
}

// Unmet returns the first obligation c fails, for an error message that names
// the actual problem instead of saying the derivation did not work.
func Unmet(c OKLCH, needs []Need) (Need, bool) {
	for _, n := range needs {
		if Contrast(c, n.Bg) < n.Ratio {
			return n, true
		}
	}
	return Need{}, false
}

// Mix interpolates between two colours in OKLCH, taking the shorter way around
// the hue circle.
//
// Interpolating hue the long way is the classic palette bug: a blend from red to
// magenta that travels 300° through green instead of 60° the other way, passing
// through colours neither endpoint suggested.
func Mix(a, b OKLCH, t float64) OKLCH {
	t = clamp01(t)
	dh := math.Mod(b.H-a.H+540, 360) - 180
	return OKLCH{
		L: a.L + (b.L-a.L)*t,
		C: a.C + (b.C-a.C)*t,
		H: math.Mod(a.H+dh*t+360, 360),
	}
}

// Grey is a neutral at the given lightness — a hue and a whisper of chroma.
//
// A surface at exactly zero chroma is the cold digital grey a dark UI reads as
// dead. A few thousandths of chroma on a warm hue is what makes it read as a
// material instead, and OKLCH is what makes "a few thousandths" mean the same
// small amount at every lightness on the ladder.
func Grey(l, h, c float64) OKLCH {
	return OKLCH{L: l, C: c, H: h}.Clamp()
}
