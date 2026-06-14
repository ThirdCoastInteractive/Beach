package ecs

import (
	"sort"
	"testing"
)

// Test component types. Distinct schema ids keep the package-global registry
// clean across test runs.
type Position struct {
	X, Y float64
}

type Velocity struct {
	DX, DY float64
}

type Health struct {
	HP int
}

func init() {
	Register[Position]("test.Position")
	Register[Velocity]("test.Velocity")
	Register[Health]("test.Health")
}

func TestEntityHandlePacking(t *testing.T) {
	tests := []struct {
		index, gen uint32
	}{
		{0, 0},
		{1, 0},
		{0, 1},
		{42, 7},
		{0xFFFFFFFF, 0xFFFFFFFF},
		{0x12345678, 0x9ABCDEF0},
	}
	for _, tc := range tests {
		e := makeEntity(tc.index, tc.gen)
		if e.index() != tc.index {
			t.Errorf("index(%d,%d) = %d, want %d", tc.index, tc.gen, e.index(), tc.index)
		}
		if e.generation() != tc.gen {
			t.Errorf("generation(%d,%d) = %d, want %d", tc.index, tc.gen, e.generation(), tc.gen)
		}
	}
}

func TestCreateAlive(t *testing.T) {
	s := New()
	if s.Len() != 0 {
		t.Fatalf("new store Len = %d, want 0", s.Len())
	}
	e := s.Create()
	if !s.Alive(e) {
		t.Fatal("created entity not alive")
	}
	if s.Len() != 1 {
		t.Fatalf("Len after Create = %d, want 1", s.Len())
	}
}

func TestStaleHandleDetection(t *testing.T) {
	s := New()
	e1 := s.Create()
	if !s.Destroy(e1) {
		t.Fatal("Destroy returned false for live entity")
	}
	if s.Alive(e1) {
		t.Fatal("destroyed handle still alive")
	}
	// Recreate: should reuse the slot but bump generation, so the old handle
	// must not alias the new entity.
	e2 := s.Create()
	if e2.index() != e1.index() {
		t.Fatalf("expected slot reuse: e2.index=%d e1.index=%d", e2.index(), e1.index())
	}
	if e2 == e1 {
		t.Fatal("recreated entity has identical handle (generation not bumped)")
	}
	if s.Alive(e1) {
		t.Fatal("stale handle reported alive after slot reuse")
	}
	if !s.Alive(e2) {
		t.Fatal("new handle not alive")
	}
}

func TestDestroyTwiceAndStale(t *testing.T) {
	s := New()
	e := s.Create()
	if !s.Destroy(e) {
		t.Fatal("first Destroy false")
	}
	if s.Destroy(e) {
		t.Fatal("second Destroy of stale handle returned true")
	}
}

func TestAddGetRemove(t *testing.T) {
	s := New()
	e := s.Create()

	if Has[Position](s, e) {
		t.Fatal("entity has Position before Add")
	}
	Add(s, e, Position{X: 1, Y: 2})
	if !Has[Position](s, e) {
		t.Fatal("entity lacks Position after Add")
	}
	p, ok := Get[Position](s, e)
	if !ok || p.X != 1 || p.Y != 2 {
		t.Fatalf("Get Position = %+v, %v", p, ok)
	}

	// Second component moves entity to a new archetype but keeps the first.
	Add(s, e, Velocity{DX: 3, DY: 4})
	p, ok = Get[Position](s, e)
	if !ok || p.X != 1 {
		t.Fatalf("Position lost after adding Velocity: %+v %v", p, ok)
	}
	v, ok := Get[Velocity](s, e)
	if !ok || v.DX != 3 {
		t.Fatalf("Get Velocity = %+v %v", v, ok)
	}

	// Remove the first; entity keeps the second.
	if !Remove[Position](s, e) {
		t.Fatal("Remove Position false")
	}
	if Has[Position](s, e) {
		t.Fatal("Position still present after Remove")
	}
	if v, ok := Get[Velocity](s, e); !ok || v.DX != 3 {
		t.Fatalf("Velocity lost after removing Position: %+v %v", v, ok)
	}
}

func TestAddOverwrite(t *testing.T) {
	s := New()
	e := s.Create()
	Add(s, e, Health{HP: 10})
	Add(s, e, Health{HP: 99}) // overwrite in place
	h, _ := Get[Health](s, e)
	if h.HP != 99 {
		t.Fatalf("overwrite HP = %d, want 99", h.HP)
	}
}

func TestSetMutate(t *testing.T) {
	s := New()
	e := s.Create()
	Add(s, e, Health{HP: 10})

	if !Set(s, e, Health{HP: 20}) {
		t.Fatal("Set false")
	}
	if h, _ := Get[Health](s, e); h.HP != 20 {
		t.Fatalf("after Set HP=%d want 20", h.HP)
	}

	if !Mutate(s, e, func(h *Health) { h.HP += 5 }) {
		t.Fatal("Mutate false")
	}
	if h, _ := Get[Health](s, e); h.HP != 25 {
		t.Fatalf("after Mutate HP=%d want 25", h.HP)
	}

	// Set/Mutate on missing component returns false.
	if Set(s, e, Position{}) {
		t.Fatal("Set on absent component returned true")
	}
}

func TestGetDeadEntity(t *testing.T) {
	s := New()
	e := s.Create()
	Add(s, e, Health{HP: 1})
	s.Destroy(e)
	if _, ok := Get[Health](s, e); ok {
		t.Fatal("Get on dead entity returned ok")
	}
	if Has[Health](s, e) {
		t.Fatal("Has on dead entity true")
	}
}

func TestQuery(t *testing.T) {
	s := New()
	want := map[Entity]Position{}
	for i := 0; i < 10; i++ {
		e := s.Create()
		p := Position{X: float64(i), Y: float64(-i)}
		Add(s, e, p)
		want[e] = p
		if i%2 == 0 {
			Add(s, e, Velocity{DX: float64(i)})
		}
	}
	// One entity with only Velocity (should not appear in Query[Position]).
	ev := s.Create()
	Add(s, ev, Velocity{DX: 100})

	got := map[Entity]Position{}
	for e, p := range Query[Position](s) {
		got[e] = p
	}
	if len(got) != len(want) {
		t.Fatalf("Query[Position] count = %d, want %d", len(got), len(want))
	}
	for e, p := range want {
		if got[e] != p {
			t.Errorf("entity %d: got %+v want %+v", e, got[e], p)
		}
	}
}

func TestQuery2(t *testing.T) {
	s := New()
	var both []Entity
	for i := 0; i < 6; i++ {
		e := s.Create()
		Add(s, e, Position{X: float64(i)})
		if i%2 == 0 {
			Add(s, e, Velocity{DX: float64(i)})
			both = append(both, e)
		}
	}
	got := map[Entity]bool{}
	for e, pair := range Query2[Position, Velocity](s) {
		got[e] = true
		if pair.A.X != pair.B.DX {
			t.Errorf("entity %d mismatched join: %+v", e, pair)
		}
	}
	if len(got) != len(both) {
		t.Fatalf("Query2 count = %d, want %d", len(got), len(both))
	}
	for _, e := range both {
		if !got[e] {
			t.Errorf("entity %d missing from Query2", e)
		}
	}
}

func TestQueryEarlyBreak(t *testing.T) {
	s := New()
	for i := 0; i < 5; i++ {
		e := s.Create()
		Add(s, e, Health{HP: i})
	}
	n := 0
	for range Query[Health](s) {
		n++
		break
	}
	if n != 1 {
		t.Fatalf("early break visited %d, want 1", n)
	}
}

func TestChangedTicks(t *testing.T) {
	s := New()
	s.SetTick(1)
	a := s.Create()
	Add(s, a, Health{HP: 1}) // stamped tick 1
	b := s.Create()
	Add(s, b, Health{HP: 2}) // stamped tick 1

	// Nothing changed strictly after tick 1.
	if n := count(Changed[Health](s, 1)); n != 0 {
		t.Fatalf("Changed since 1 = %d, want 0", n)
	}
	// Everything changed strictly after tick 0.
	if n := count(Changed[Health](s, 0)); n != 2 {
		t.Fatalf("Changed since 0 = %d, want 2", n)
	}

	s.SetTick(5)
	Mutate(s, a, func(h *Health) { h.HP = 100 }) // re-stamped tick 5

	got := map[Entity]Health{}
	for e, h := range Changed[Health](s, 1) {
		got[e] = h
	}
	if len(got) != 1 {
		t.Fatalf("Changed since 1 after mutate = %d, want 1", len(got))
	}
	if got[a].HP != 100 {
		t.Fatalf("changed entity HP = %d, want 100", got[a].HP)
	}
}

func TestChangedSurvivesMigration(t *testing.T) {
	// Adding a *different* component to an entity must preserve the original
	// component's tick (migration copies the stored stamp, not the current).
	s := New()
	s.SetTick(3)
	e := s.Create()
	Add(s, e, Health{HP: 7}) // tick 3
	s.SetTick(9)
	Add(s, e, Position{X: 1}) // migrates; Health stamp must stay 3

	if n := count(Changed[Health](s, 3)); n != 0 {
		t.Fatalf("Health changed since 3 = %d, want 0 (stamp should be preserved at 3)", n)
	}
	if n := count(Changed[Position](s, 3)); n != 1 {
		t.Fatalf("Position changed since 3 = %d, want 1", n)
	}
}

func TestRelationships(t *testing.T) {
	s := New()
	owner := s.Create()
	i1 := s.Create()
	i2 := s.Create()
	i3 := s.Create()

	s.Relate("owns", i1, owner)
	s.Relate("owns", i2, owner)
	s.Relate("owns", i3, owner)

	if tgt, ok := s.Target("owns", i1); !ok || tgt != owner {
		t.Fatalf("Target(owns,i1) = %d,%v", tgt, ok)
	}
	srcs := s.Sources("owns", owner)
	if len(srcs) != 3 {
		t.Fatalf("owner has %d items, want 3", len(srcs))
	}
	assertSet(t, srcs, []Entity{i1, i2, i3})

	// Re-relate i1 to a different owner: reverse index must update both ends.
	owner2 := s.Create()
	s.Relate("owns", i1, owner2)
	if got := s.Sources("owns", owner); len(got) != 2 {
		t.Fatalf("after re-relate owner has %d, want 2", len(got))
	}
	if got := s.Sources("owns", owner2); len(got) != 1 || got[0] != i1 {
		t.Fatalf("owner2 sources = %v, want [i1]", got)
	}

	// Unrelate.
	if !s.Unrelate("owns", i2) {
		t.Fatal("Unrelate false")
	}
	if _, ok := s.Target("owns", i2); ok {
		t.Fatal("i2 still has target after Unrelate")
	}
}

func TestRelationCleanupOnDestroy(t *testing.T) {
	s := New()
	owner := s.Create()
	item := s.Create()
	s.Relate("owns", item, owner)

	// Destroying the target purges the source edge.
	s.Destroy(owner)
	if _, ok := s.Target("owns", item); ok {
		t.Fatal("edge survived target destruction")
	}

	// Destroying the source purges the reverse entry.
	owner2 := s.Create()
	item2 := s.Create()
	s.Relate("owns", item2, owner2)
	s.Destroy(item2)
	if got := s.Sources("owns", owner2); len(got) != 0 {
		t.Fatalf("reverse index has %v after source destroyed, want empty", got)
	}
}

func TestRelateDeadEntity(t *testing.T) {
	s := New()
	a := s.Create()
	b := s.Create()
	s.Destroy(b)
	if s.Relate("owns", a, b) {
		t.Fatal("Relate to dead target returned true")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := New()
	s.SetTick(7)
	e1 := s.Create()
	Add(s, e1, Position{X: 1, Y: 2})
	Add(s, e1, Health{HP: 50})
	e2 := s.Create()
	Add(s, e2, Velocity{DX: 9})
	// Create and destroy one to exercise the freelist/generation restore.
	dead := s.Create()
	s.Destroy(dead)

	s.Relate("owns", e2, e1)

	data, err := s.Save()
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	r, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if r.Tick() != 7 {
		t.Errorf("restored tick = %d, want 7", r.Tick())
	}
	if r.Len() != s.Len() {
		t.Errorf("restored Len = %d, want %d", r.Len(), s.Len())
	}
	if !r.Alive(e1) || !r.Alive(e2) {
		t.Fatal("live entities not alive after restore")
	}
	if r.Alive(dead) {
		t.Fatal("dead entity alive after restore")
	}
	if p, ok := Get[Position](r, e1); !ok || p.X != 1 || p.Y != 2 {
		t.Errorf("restored Position = %+v %v", p, ok)
	}
	if h, ok := Get[Health](r, e1); !ok || h.HP != 50 {
		t.Errorf("restored Health = %+v %v", h, ok)
	}
	if v, ok := Get[Velocity](r, e2); !ok || v.DX != 9 {
		t.Errorf("restored Velocity = %+v %v", v, ok)
	}
	if tgt, ok := r.Target("owns", e2); !ok || tgt != e1 {
		t.Errorf("restored relation Target = %d %v", tgt, ok)
	}
}

func TestSnapshotPreservesTicks(t *testing.T) {
	s := New()
	s.SetTick(2)
	e := s.Create()
	Add(s, e, Health{HP: 1}) // tick 2
	s.SetTick(10)

	data, _ := s.Save()
	r, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The Health stamp must come back as 2, so Changed since 2 sees nothing.
	if n := count(Changed[Health](r, 2)); n != 0 {
		t.Fatalf("restored Changed since 2 = %d, want 0 (tick should be 2)", n)
	}
	if n := count(Changed[Health](r, 1)); n != 1 {
		t.Fatalf("restored Changed since 1 = %d, want 1", n)
	}
}

func TestSnapshotFreelistReuse(t *testing.T) {
	s := New()
	a := s.Create()
	b := s.Create()
	s.Destroy(a) // a's slot freed
	_ = b

	data, _ := s.Save()
	r, _ := Load(data)
	// Next Create in the restored store should reuse a's slot with bumped gen,
	// not collide with b.
	c := r.Create()
	if c.index() != a.index() {
		t.Fatalf("restored Create index = %d, want reuse of %d", c.index(), a.index())
	}
	if c == a {
		t.Fatal("reused slot kept stale generation")
	}
	if !r.Alive(c) || !r.Alive(b) {
		t.Fatal("entities not alive after restore+create")
	}
}

// helpers

func count[T any](seq func(func(Entity, T) bool)) int {
	n := 0
	for range seq {
		n++
	}
	return n
}

func assertSet(t *testing.T, got, want []Entity) {
	t.Helper()
	gs := append([]Entity(nil), got...)
	ws := append([]Entity(nil), want...)
	sort.Slice(gs, func(i, j int) bool { return gs[i] < gs[j] })
	sort.Slice(ws, func(i, j int) bool { return ws[i] < ws[j] })
	if len(gs) != len(ws) {
		t.Fatalf("set size %d, want %d", len(gs), len(ws))
	}
	for i := range gs {
		if gs[i] != ws[i] {
			t.Fatalf("set mismatch at %d: %d vs %d", i, gs[i], ws[i])
		}
	}
}
