// Package ecs is a standalone, zero-knowledge archetype ECS: entities and
// columnar (struct-of-arrays) component storage with tick-stamped change
// detection, typed queries, CBOR snapshots, and entity-valued relationships.
//
// It has no knowledge of Beach, hubs, or Postgres. The container is a
// Store; it does not tick, schedule, or know about time — that is the caller's
// job. See docs/architecture/07-ecs.md.
package ecs

// Entity is a 64-bit handle: a 32-bit index packed with a 32-bit generation.
// The generation lets the Store detect stale handles after a slot is reused,
// so a destroyed-then-recreated slot never silently aliases an old reference.
// Entity is a plain integer with no pointer to dangle, portable to any
// language.
type Entity uint64

// index returns the low 32 bits: the slot index into the Store's entity table.
func (e Entity) index() uint32 { return uint32(e) }

// generation returns the high 32 bits: the slot's reuse counter.
func (e Entity) generation() uint32 { return uint32(e >> 32) }

// makeEntity packs an index and generation into a handle.
func makeEntity(index, generation uint32) Entity {
	return Entity(index) | Entity(generation)<<32
}

// Index returns the entity's slot index. Exposed for snapshot/debug callers.
func (e Entity) Index() uint32 { return e.index() }

// Generation returns the entity's generation counter.
func (e Entity) Generation() uint32 { return e.generation() }
