package sim

// rng is a tiny deterministic PRNG (splitmix64). It lives on the World so all
// randomness inside Commands and Systems flows through one seeded stream: the
// same seed plus the same ordered command/tick sequence yields the same numbers
// every run. That reproducibility is the whole point — dice rolls, loot drops,
// and procedural spawns must replay identically in tests and across restores.
//
// splitmix64 is chosen for being a single uint64 of state (trivial to snapshot
// or seed) with good statistical quality for game-grade randomness. It is not
// cryptographic; never use it for tokens or secrets.
type rng struct {
	state uint64
}

func newRNG(seed uint64) *rng { return &rng{state: seed} }

// next advances the generator and returns the next 64-bit value.
func (r *rng) next() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// intn returns a value in [0, n) without modulo bias, via rejection sampling.
func (r *rng) intn(n int) int {
	if n <= 0 {
		panic("sim: Intn requires n > 0")
	}
	un := uint64(n)
	// Largest multiple of un that fits in uint64; reject the tail above it.
	limit := (^uint64(0) / un) * un
	for {
		v := r.next()
		if v < limit {
			return int(v % un)
		}
	}
}
