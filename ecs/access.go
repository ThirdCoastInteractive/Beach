package ecs

// This file holds the typed access layer. Because Go methods cannot be generic
// over a parameter the receiver does not mention, component access is exposed
// as package-level generic functions taking the Store explicitly:
//
//	ecs.Add(s, e, comp.XP{Level: 1})
//	xp, ok := ecs.Get[comp.XP](s, e)
//	ecs.Set(s, e, e2, func(xp *comp.XP){ xp.Level++ })  // not this; see below
//
// Mutation goes through Set / Mutate, which stamp the column row with the
// Store's current tick — this is the change-detection mechanism Changed[T]
// reads. Reading via Get never stamps.

// colFor returns the typed column for component T in archetype a, or nil if a
// does not hold T.
func colFor[T any](a *archetype, id componentID) *typedColumn[T] {
	c, ok := a.columns[id]
	if !ok {
		return nil
	}
	return c.(*typedColumn[T])
}

// Has reports whether entity e currently holds component T. A stale or dead
// handle reports false.
func Has[T any](s *Store, e Entity) bool {
	if !s.Alive(e) {
		return false
	}
	info := idFor[T]()
	return s.slots[e.index()].loc.arch.mask.has(info.id)
}

// Get returns entity e's component T and true, or the zero value and false if e
// is dead/stale or does not hold T. Get does not stamp the tick.
func Get[T any](s *Store, e Entity) (T, bool) {
	var zero T
	if !s.Alive(e) {
		return zero, false
	}
	info := idFor[T]()
	loc := s.slots[e.index()].loc
	col := colFor[T](loc.arch, info.id)
	if col == nil {
		return zero, false
	}
	return col.data[loc.row], true
}

// Add gives entity e component T with value v, migrating it to the archetype
// that includes T, and stamps the new column at the current tick. If e already
// holds T, Add overwrites the value (and re-stamps). Adding to a dead/stale
// handle panics — adding to a non-entity is a programmer error, not a runtime
// condition to tolerate.
func Add[T any](s *Store, e Entity, v T) {
	if !s.Alive(e) {
		panic("ecs: Add to dead or stale entity")
	}
	info := idFor[T]()
	sl := &s.slots[e.index()]
	if sl.loc.arch.mask.has(info.id) {
		// already present: overwrite in place and re-stamp
		col := colFor[T](sl.loc.arch, info.id)
		col.data[sl.loc.row] = v
		col.stamp[sl.loc.row] = s.tick
		return
	}
	mask := sl.loc.arch.mask
	mask.set(info.id)
	dst := s.archetypeFor(mask)
	row := s.migrate(e, dst)
	col := colFor[T](dst, info.id)
	col.data[row] = v
	col.stamp[row] = s.tick
}

// Set overwrites entity e's component T and stamps the current tick. It
// requires e to already hold T; use Add to attach it. Returns false if e is
// dead/stale or lacks T.
func Set[T any](s *Store, e Entity, v T) bool {
	if !s.Alive(e) {
		return false
	}
	info := idFor[T]()
	loc := s.slots[e.index()].loc
	col := colFor[T](loc.arch, info.id)
	if col == nil {
		return false
	}
	col.data[loc.row] = v
	col.stamp[loc.row] = s.tick
	return true
}

// Mutate applies fn to a pointer to entity e's component T in place and stamps
// the current tick. This is the cheap path for incremental updates (xp.Level++)
// that avoids a read-modify-write copy. Returns false if e is dead/stale or
// lacks T.
func Mutate[T any](s *Store, e Entity, fn func(*T)) bool {
	if !s.Alive(e) {
		return false
	}
	info := idFor[T]()
	loc := s.slots[e.index()].loc
	col := colFor[T](loc.arch, info.id)
	if col == nil {
		return false
	}
	fn(&col.data[loc.row])
	col.stamp[loc.row] = s.tick
	return true
}

// Remove detaches component T from entity e, migrating it to the archetype
// without T. Returns false if e is dead/stale or did not hold T.
func Remove[T any](s *Store, e Entity) bool {
	if !s.Alive(e) {
		return false
	}
	info := idFor[T]()
	sl := &s.slots[e.index()]
	if !sl.loc.arch.mask.has(info.id) {
		return false
	}
	mask := sl.loc.arch.mask
	mask.clear(info.id)
	dst := s.archetypeFor(mask)
	s.migrate(e, dst)
	return true
}
