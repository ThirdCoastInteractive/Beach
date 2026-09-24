package ecs

import "iter"

// A View is an immutable, off-loop-safe read snapshot of a Store's live slice.
//
// Why it exists: once the sim's loop goroutine owns a Store, reading it from any
// other goroutine is a data race. The CBOR Save/Load path (snapshot.go) proves a
// consistent copy can be taken on the loop, but it is heavyweight (full marshal)
// and aimed at persistence. View is the lightweight in-memory equivalent for the
// catch-up-render path: a handler that needs the current board state on page-nav
// or stream-open takes a View once and reads it freely.
//
// Concurrency guarantee: a View is a deep value copy of the live component
// columns, taken by View() while the caller holds exclusive store access (on the
// loop goroutine). Once built it shares no memory with the Store, so the loop may
// mutate the Store concurrently while any number of goroutines read the View. A
// View never changes after construction; it reflects the store exactly as of the
// tick at which it was taken. Build a fresh one to observe later state.
//
// A View is read-only: it exposes ViewQuery / ViewGet / ViewChanged / ViewHas,
// the read mirrors of the Store helpers, plus Tick and Len. It deliberately has
// no mutators and no relation reads — it is the rendered-state read model, not a
// second store.
type View struct {
	tick Tick
	// rows holds one entry per registered component id that had any live row at
	// capture time. Each entry is a parallel set of entities, a copied typed
	// column of values, and the per-row last-write ticks. The column carries its
	// own ticks slice, so changed-since queries work off-loop too.
	rows map[componentID]*viewColumn
}

// viewColumn is one component type's copied rows inside a View: the entities that
// held it (parallel to the column's data) and the copied column itself (values +
// ticks). The column is a fresh, owned copy — never aliased to the live Store.
type viewColumn struct {
	entities []Entity
	col      column
}

// View captures an immutable copy of every live entity's components at the
// current tick. It must be called with exclusive store access — on the loop
// goroutine (e.g. inside a Command or via sim.Sim.Snapshot) or before Run
// starts. The returned View shares no memory with the Store and is safe to read
// from other goroutines while the loop keeps mutating.
func (s *Store) View() *View {
	v := &View{tick: s.tick, rows: make(map[componentID]*viewColumn)}
	for _, a := range s.archetypes {
		if len(a.entities) == 0 {
			continue
		}
		for _, id := range a.ids {
			src := a.columns[id]
			vc := v.rows[id]
			if vc == nil {
				vc = &viewColumn{col: registry.byTypeOf(id).newColumn()}
				v.rows[id] = vc
			}
			for row, e := range a.entities {
				vc.entities = append(vc.entities, e)
				vc.col.appendFrom(src, row) // deep-copies value + preserves tick
			}
		}
	}
	return v
}

// Tick reports the store tick the View was captured at. Pair it with a later
// ViewChanged(sinceTick) on a newer View to diff, though the common use is a
// one-shot full read.
func (v *View) Tick() Tick { return v.tick }

// Len reports the number of distinct live entities captured in the View.
func (v *View) Len() int {
	seen := make(map[Entity]struct{})
	for _, vc := range v.rows {
		for _, e := range vc.entities {
			seen[e] = struct{}{}
		}
	}
	return len(seen)
}

// ViewHas reports whether entity e held component T at capture time.
func ViewHas[T any](v *View, e Entity) bool {
	info := idFor[T]()
	vc := v.rows[info.id]
	if vc == nil {
		return false
	}
	for _, ve := range vc.entities {
		if ve == e {
			return true
		}
	}
	return false
}

// ViewGet returns entity e's component T as captured, and true, or the zero
// value and false if e did not hold T at capture time. It is the off-loop read
// mirror of Get.
func ViewGet[T any](v *View, e Entity) (T, bool) {
	var zero T
	info := idFor[T]()
	vc := v.rows[info.id]
	if vc == nil {
		return zero, false
	}
	col := vc.col.(*typedColumn[T])
	for row, ve := range vc.entities {
		if ve == e {
			return col.data[row], true
		}
	}
	return zero, false
}

// ViewQuery iterates every entity that held component T at capture time,
// yielding the entity and a copy of its value. It is the off-loop read mirror of
// Query: iterate it from any goroutine, as many times as you like.
func ViewQuery[T any](v *View) iter.Seq2[Entity, T] {
	info := idFor[T]()
	return func(yield func(Entity, T) bool) {
		vc := v.rows[info.id]
		if vc == nil {
			return
		}
		col := vc.col.(*typedColumn[T])
		for row, e := range vc.entities {
			if !yield(e, col.data[row]) {
				return
			}
		}
	}
}

// ViewChanged iterates every entity whose T column was stamped strictly after
// sinceTick as of capture — the dirty set frozen into the View. It is the
// off-loop read mirror of Changed, useful to render only what moved since a
// known tick from a catch-up handler. Pass 0 for every entity holding T.
func ViewChanged[T any](v *View, sinceTick Tick) iter.Seq2[Entity, T] {
	info := idFor[T]()
	return func(yield func(Entity, T) bool) {
		vc := v.rows[info.id]
		if vc == nil {
			return
		}
		col := vc.col.(*typedColumn[T])
		for row, e := range vc.entities {
			if col.stamp[row] > sinceTick {
				if !yield(e, col.data[row]) {
					return
				}
			}
		}
	}
}
