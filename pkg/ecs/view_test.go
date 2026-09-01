package ecs

import (
	"sync"
	"testing"
)

// View is the immutable off-loop read snapshot. These tests cover the read
// mirrors (ViewQuery/ViewGet/ViewChanged/ViewHas), that a View is a frozen deep
// copy (later mutation does not leak in), and — under -race — that reading a
// View from another goroutine while the store keeps mutating is safe.

func TestViewReadMirrors(t *testing.T) {
	s := New()
	s.SetTick(5)
	a := s.Create()
	Add(s, a, Position{X: 1, Y: 2})
	Add(s, a, Health{HP: 10})
	b := s.Create()
	Add(s, b, Position{X: 3, Y: 4})

	v := s.View()

	if v.Tick() != 5 {
		t.Fatalf("View.Tick = %d, want 5", v.Tick())
	}
	if v.Len() != 2 {
		t.Fatalf("View.Len = %d, want 2", v.Len())
	}

	// ViewGet
	if p, ok := ViewGet[Position](v, a); !ok || p.X != 1 || p.Y != 2 {
		t.Fatalf("ViewGet Position(a) = %+v ok=%v", p, ok)
	}
	if h, ok := ViewGet[Health](v, a); !ok || h.HP != 10 {
		t.Fatalf("ViewGet Health(a) = %+v ok=%v", h, ok)
	}
	if _, ok := ViewGet[Health](v, b); ok {
		t.Fatal("ViewGet Health(b) should be absent")
	}

	// ViewHas
	if !ViewHas[Position](v, a) || !ViewHas[Position](v, b) {
		t.Fatal("ViewHas Position should be true for a and b")
	}
	if ViewHas[Health](v, b) {
		t.Fatal("ViewHas Health(b) should be false")
	}

	// ViewQuery: both positions present.
	seen := map[Entity]Position{}
	for e, p := range ViewQuery[Position](v) {
		seen[e] = p
	}
	if len(seen) != 2 || seen[a].X != 1 || seen[b].X != 3 {
		t.Fatalf("ViewQuery Position = %+v", seen)
	}
}

func TestViewIsFrozenCopy(t *testing.T) {
	s := New()
	s.SetTick(1)
	e := s.Create()
	Add(s, e, Position{X: 1, Y: 1})

	v := s.View()

	// Mutate the live store after the snapshot; the View must not observe it.
	s.SetTick(2)
	Set(s, e, Position{X: 99, Y: 99})
	f := s.Create()
	Add(s, f, Position{X: 7, Y: 7})

	if p, _ := ViewGet[Position](v, e); p.X != 1 {
		t.Fatalf("View saw post-snapshot mutation: %+v", p)
	}
	if ViewHas[Position](v, f) {
		t.Fatal("View saw entity created after the snapshot")
	}
	if v.Len() != 1 {
		t.Fatalf("View.Len = %d, want 1 (frozen at snapshot)", v.Len())
	}
}

func TestViewChangedFrozenDirtySet(t *testing.T) {
	s := New()
	s.SetTick(1)
	e1 := s.Create()
	Add(s, e1, Health{HP: 1}) // stamped at tick 1
	s.SetTick(2)
	e2 := s.Create()
	Add(s, e2, Health{HP: 2}) // stamped at tick 2

	v := s.View()

	// Changed since tick 1: only e2.
	got := map[Entity]int{}
	for e, h := range ViewChanged[Health](v, 1) {
		got[e] = h.HP
	}
	if len(got) != 1 || got[e2] != 2 {
		t.Fatalf("ViewChanged(1) = %+v, want only e2", got)
	}
	// Changed since 0: both.
	count := 0
	for range ViewChanged[Health](v, 0) {
		count++
	}
	if count != 2 {
		t.Fatalf("ViewChanged(0) count = %d, want 2", count)
	}
}

// TestViewOffLoopReadDuringMutation is the concurrency guarantee: a View taken
// at one instant is read from a separate goroutine while the "loop" goroutine
// keeps mutating the same store. Run under -race to catch any aliasing between
// the View copy and the live columns.
func TestViewOffLoopReadDuringMutation(t *testing.T) {
	s := New()
	const n = 200
	ents := make([]Entity, n)
	for i := range ents {
		ents[i] = s.Create()
		Add(s, ents[i], Position{X: float64(i)})
	}

	v := s.View() // captured once; readers below touch only this

	// Writer: the "loop" keeps mutating the live store, never the View. It runs
	// until the readers signal done (its own goroutine, separate from the reader
	// WaitGroup so we never wait on it).
	stop := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		tick := Tick(1)
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, e := range ents {
				Set(s, e, Position{X: float64(tick)})
			}
			x := s.Create()
			Add(s, x, Health{HP: int(tick)})
			tick++
			s.SetTick(tick)
		}
	}()

	// Readers: hammer the immutable View concurrently.
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for iter := 0; iter < 500; iter++ {
				sum := 0.0
				for _, p := range ViewQuery[Position](v) {
					sum += p.X
				}
				_ = sum
				for _, e := range ents {
					_, _ = ViewGet[Position](v, e)
				}
				_ = v.Len()
			}
		}()
	}

	// Let readers run to completion, then stop the writer.
	readers.Wait()
	close(stop)
	<-writerDone

	// The View must still reflect exactly the snapshot instant: n Position rows,
	// each X equal to its original index (the writer's changes never leaked in).
	got := 0
	for e, p := range ViewQuery[Position](v) {
		// find original index
		for i := range ents {
			if ents[i] == e {
				if p.X != float64(i) {
					t.Fatalf("View Position(%d) X=%v leaked a mutation", i, p.X)
				}
			}
		}
		got++
	}
	if got != n {
		t.Fatalf("View Position rows = %d, want %d", got, n)
	}
}
