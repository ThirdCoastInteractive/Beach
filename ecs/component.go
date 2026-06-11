package ecs

import (
	"fmt"
	"reflect"
	"sync"
)

// componentID is a small dense integer assigned to each registered component
// type. It doubles as a bit position in an archetype's componentMask.
type componentID uint16

// maxComponents bounds the bitmask width. 256 distinct component types is far
// more than the live slice needs; raising it means widening componentMask.
const maxComponents = 256

// componentMask is a fixed-width bitset over componentIDs identifying exactly
// which components an archetype holds. Fixed-width keeps it comparable and
// usable as a map key for archetype lookup.
type componentMask [maxComponents / 64]uint64

func (m *componentMask) set(id componentID)     { m[id/64] |= 1 << (id % 64) }
func (m *componentMask) clear(id componentID)   { m[id/64] &^= 1 << (id % 64) }
func (m componentMask) has(id componentID) bool { return m[id/64]&(1<<(id%64)) != 0 }
func (m componentMask) hasAll(o componentMask) bool {
	for i := range m {
		if m[i]&o[i] != o[i] {
			return false
		}
	}
	return true
}

// componentInfo is the per-type registry entry. schemaID is a caller-supplied
// stable string used by snapshots so save/restore survive type renames and
// component reordering.
type componentInfo struct {
	id       componentID
	schemaID string
	typ      reflect.Type
	// newColumn builds a fresh typed column for this component. It is a
	// closure capturing the concrete generic type so the non-generic Store can
	// allocate columns without reflection at the hot path.
	newColumn func() column
}

// registry is a process-global map from Go type to componentInfo. Registration
// is idempotent: registering the same type twice returns the existing entry.
// It is package-global (not per-Store) so that the package-level generic
// helpers — Add[T], Get[T], Query[T] — can resolve a type to its id without a
// Store reference, and so two Stores share component identity (required for
// snapshot portability).
type componentRegistry struct {
	sync.Mutex
	byType   map[reflect.Type]*componentInfo
	bySchema map[string]*componentInfo
	byID     []*componentInfo // indexed by componentID, append-only
	next     componentID
}

var registry = componentRegistry{
	byType:   make(map[reflect.Type]*componentInfo),
	bySchema: make(map[string]*componentInfo),
}

// byTypeOf returns the componentInfo for a componentID. byID is append-only and
// never reordered, so this read needs no lock once the id is known to exist
// (ids are handed out only under the registry lock).
func (r *componentRegistry) byTypeOf(id componentID) *componentInfo {
	return r.byID[id]
}

// Register declares a component type with a stable schema id and returns its
// componentInfo. Most callers use the generic Register[T]; this lower form
// exists for the generic wrapper. Re-registering the same type with the same
// schema id is a no-op; a conflicting schema id panics, because a silent
// mismatch corrupts snapshots.
func registerType(typ reflect.Type, schemaID string, newColumn func() column) *componentInfo {
	registry.Lock()
	defer registry.Unlock()
	if info, ok := registry.byType[typ]; ok {
		if info.schemaID != schemaID {
			panic(fmt.Sprintf("ecs: component %s already registered with schema id %q, cannot re-register as %q", typ, info.schemaID, schemaID))
		}
		return info
	}
	if other, ok := registry.bySchema[schemaID]; ok {
		panic(fmt.Sprintf("ecs: schema id %q already used by %s, cannot assign to %s", schemaID, other.typ, typ))
	}
	if registry.next >= maxComponents {
		panic("ecs: too many component types registered (max 256)")
	}
	info := &componentInfo{
		id:        registry.next,
		schemaID:  schemaID,
		typ:       typ,
		newColumn: newColumn,
	}
	registry.next++
	registry.byType[typ] = info
	registry.bySchema[schemaID] = info
	registry.byID = append(registry.byID, info)
	return info
}

// Register declares component type T with a stable schema id (used by
// snapshots). Call it once per type before using T with a Store; it is safe to
// call repeatedly. Returns nothing useful — the side effect is the
// registration. If you never call Register, the first Add[T]/Get[T] auto-
// registers T using its Go type name as the schema id, which is convenient but
// fragile across renames; declare an explicit id for anything you snapshot.
func Register[T any](schemaID string) {
	infoFor[T](schemaID)
}

// infoFor returns the componentInfo for T, registering it on first use. The
// schemaID is honored only on the registering call; subsequent calls ignore it
// (the type is already registered). An empty schemaID defaults to the Go type
// name.
func infoFor[T any](schemaID string) *componentInfo {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	registry.Lock()
	if info, ok := registry.byType[typ]; ok {
		registry.Unlock()
		return info
	}
	registry.Unlock()
	if schemaID == "" {
		schemaID = typ.String()
	}
	return registerType(typ, schemaID, func() column { return newTypedColumn[T]() })
}

// idFor is the hot-path accessor used by the generic helpers.
func idFor[T any]() *componentInfo { return infoFor[T]("") }

// snapshotInfos returns a stable-ordered copy of all registered component
// infos (ordered by componentID, which is assignment order). Snapshots iterate
// this to emit one block per type.
func (r *componentRegistry) snapshotInfos() []*componentInfo {
	r.Lock()
	defer r.Unlock()
	out := make([]*componentInfo, len(r.byID))
	copy(out, r.byID)
	return out
}

// bySchemaLocked looks up a component by its stable schema id.
func (r *componentRegistry) bySchemaLocked(schemaID string) (*componentInfo, bool) {
	r.Lock()
	defer r.Unlock()
	info, ok := r.bySchema[schemaID]
	return info, ok
}
