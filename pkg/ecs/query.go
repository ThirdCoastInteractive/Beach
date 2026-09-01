package ecs

import "iter"

// Queries iterate matching archetypes' columns directly. They are exposed as
// range-over-func iterators (Go 1.23+) so callers write idiomatic for-range
// loops:
//
//	for e, xp := range ecs.Query[comp.XP](s) {
//	    ...
//	}
//
// The Store must not be structurally mutated (Create/Destroy/Add/Remove) during
// iteration of its own archetypes — doing so swap-removes rows out from under
// the loop. Reads and in-place Set/Mutate of the iterated component are safe.

// Query iterates every live entity holding component T, yielding the entity and
// a copy of its T value. Archetypes that include T (possibly alongside other
// components) all match.
func Query[T any](s *Store) iter.Seq2[Entity, T] {
	info := idFor[T]()
	return func(yield func(Entity, T) bool) {
		for _, a := range s.archetypes {
			if !a.mask.has(info.id) {
				continue
			}
			col := colFor[T](a, info.id)
			ents := a.entities
			for row := range ents {
				if !yield(ents[row], col.data[row]) {
					return
				}
			}
		}
	}
}

// Changed iterates every live entity holding component T whose T column was
// stamped strictly after sinceTick — the dirty set since that tick. This is the
// heart of the rendering pipeline: the sim collects each tick's changes with
// Changed[T](lastTick). Pass 0 to get every entity holding T (everything is
// "changed since before the beginning").
func Changed[T any](s *Store, sinceTick Tick) iter.Seq2[Entity, T] {
	info := idFor[T]()
	return func(yield func(Entity, T) bool) {
		for _, a := range s.archetypes {
			if !a.mask.has(info.id) {
				continue
			}
			col := colFor[T](a, info.id)
			ents := a.entities
			for row := range ents {
				if col.stamp[row] > sinceTick {
					if !yield(ents[row], col.data[row]) {
						return
					}
				}
			}
		}
	}
}

// Query2 iterates every live entity holding both A and B, yielding the entity
// and copies of both component values. This covers the common two-component
// join (the live slice rarely needs wider joins; build those by querying the
// rarer component and Get-ing the others).
func Query2[A any, B any](s *Store) iter.Seq2[Entity, AB[A, B]] {
	ia := idFor[A]()
	ib := idFor[B]()
	return func(yield func(Entity, AB[A, B]) bool) {
		for _, arch := range s.archetypes {
			if !arch.mask.has(ia.id) || !arch.mask.has(ib.id) {
				continue
			}
			ca := colFor[A](arch, ia.id)
			cb := colFor[B](arch, ib.id)
			ents := arch.entities
			for row := range ents {
				if !yield(ents[row], AB[A, B]{A: ca.data[row], B: cb.data[row]}) {
					return
				}
			}
		}
	}
}

// AB pairs two component values yielded by Query2.
type AB[A any, B any] struct {
	A A
	B B
}
