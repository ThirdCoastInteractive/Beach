package ecs

import "testing"

// BenchmarkQueryIteration is the CI regression tripwire described in the doc:
// it catches a pathological O(n^2) in query iteration, it is not a competitive
// target. It iterates a two-component query over a population sized for the
// live slice (connected users + hot components).
func BenchmarkQueryIteration(b *testing.B) {
	s := New()
	const n = 10_000
	for i := 0; i < n; i++ {
		e := s.Create()
		Add(s, e, Position{X: float64(i), Y: float64(i)})
		Add(s, e, Velocity{DX: 1, DY: 1})
		if i%3 == 0 {
			Add(s, e, Health{HP: 100}) // mix archetypes
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sink float64
	for i := 0; i < b.N; i++ {
		for _, pair := range Query2[Position, Velocity](s) {
			sink += pair.A.X + pair.B.DX
		}
	}
	_ = sink
}

// BenchmarkChanged measures the dirty-set scan used every projection tick.
func BenchmarkChanged(b *testing.B) {
	s := New()
	const n = 10_000
	s.SetTick(1)
	for i := 0; i < n; i++ {
		e := s.Create()
		Add(s, e, Health{HP: i})
	}
	// Mark a tenth of them dirty at a later tick.
	s.SetTick(2)
	j := 0
	for e := range Query[Health](s) {
		if j%10 == 0 {
			Mutate(s, e, func(h *Health) { h.HP++ })
		}
		j++
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		for _, h := range Changed[Health](s, 1) {
			sink += h.HP
		}
	}
	_ = sink
}
