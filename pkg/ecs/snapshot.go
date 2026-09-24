package ecs

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// snapshotVersion is the on-disk format version. Bump it on any incompatible
// change to the wire layout below (not on component schema changes — those are
// handled by stable schema ids).
const snapshotVersion = 1

// Snapshots are versioned CBOR keyed by stable component schema ids, so a
// restore survives component reordering and Go type renames as long as the
// schema id is unchanged. A component present in the snapshot but no longer
// registered is an error (the caller dropped a type); a registered component
// absent from the snapshot simply restores as empty.

// entityRecord captures one live entity's identity and which components it
// holds (by schema id). Component values live in the per-component blobs below,
// addressed by the same (schemaID, ordinal) the writer assigned.
type snapshot struct {
	Version int  `cbor:"v"`
	Tick    Tick `cbor:"tick"`
	// Generations records every slot's generation counter, dead slots included,
	// so freelist reuse resumes from the right generation after restore.
	Generations []uint32        `cbor:"gens"`
	Entities    []snapEntity    `cbor:"entities"` // the live subset, by index
	Components  []snapComponent `cbor:"components"`
	Relations   []snapRelation  `cbor:"relations,omitempty"`
}

type snapEntity struct {
	Index      uint32 `cbor:"i"`
	Generation uint32 `cbor:"g"`
}

// snapComponent holds one component type's rows: the schema id, the entities
// that have it (parallel to Values), and the CBOR-encoded values + ticks.
type snapComponent struct {
	SchemaID string          `cbor:"id"`
	Entities []uint64        `cbor:"e"` // packed Entity handles, parallel to Values
	Values   cbor.RawMessage `cbor:"v"` // CBOR array of T, decoded by the column
	Ticks    []Tick          `cbor:"t"`
}

type snapRelation struct {
	Label   string   `cbor:"l"`
	Sources []uint64 `cbor:"s"`
	Targets []uint64 `cbor:"d"`
}

// Save serializes the Store to a versioned CBOR snapshot. Only live entities
// and their components are written. Component columns are addressed by stable
// schema id, so restore tolerates type reordering/renames.
func (s *Store) Save() ([]byte, error) {
	snap := snapshot{
		Version:     snapshotVersion,
		Tick:        s.tick,
		Generations: make([]uint32, len(s.slots)),
	}

	// Record every slot's generation (so dead slots restore with the correct
	// reuse counter) and the live subset separately.
	for i := range s.slots {
		snap.Generations[i] = s.slots[i].generation
		if s.slots[i].alive {
			snap.Entities = append(snap.Entities, snapEntity{
				Index:      uint32(i),
				Generation: s.slots[i].generation,
			})
		}
	}

	// One snapComponent per registered component type that has any rows.
	for _, info := range registry.snapshotInfos() {
		comp, err := s.encodeComponent(info)
		if err != nil {
			return nil, err
		}
		if comp != nil {
			snap.Components = append(snap.Components, *comp)
		}
	}

	// Relations.
	for label, idx := range s.relations {
		rel := snapRelation{Label: label}
		for src, dst := range idx.forward {
			rel.Sources = append(rel.Sources, uint64(src))
			rel.Targets = append(rel.Targets, uint64(dst))
		}
		if len(rel.Sources) > 0 {
			snap.Relations = append(snap.Relations, rel)
		}
	}

	return cbor.Marshal(snap)
}

// encodeComponent gathers every live entity holding the component and encodes
// its values column as a CBOR array. Returns nil if no entity holds it.
func (s *Store) encodeComponent(info *componentInfo) (*snapComponent, error) {
	out := &snapComponent{SchemaID: info.schemaID}
	enc := info.newColumn() // a fresh column to accumulate values in entity order
	for _, a := range s.archetypes {
		if !a.mask.has(info.id) {
			continue
		}
		col := a.columns[info.id]
		ticks := col.ticks()
		for row, e := range a.entities {
			out.Entities = append(out.Entities, uint64(e))
			enc.appendFrom(col, row)
			out.Ticks = append(out.Ticks, ticks[row])
		}
	}
	if len(out.Entities) == 0 {
		return nil, nil
	}
	raw, err := enc.marshalValues()
	if err != nil {
		return nil, fmt.Errorf("ecs: encode component %s: %w", info.schemaID, err)
	}
	out.Values = raw
	return out, nil
}

// Load restores a Store from a snapshot produced by Save. It returns a fresh
// Store; the receiver is ignored beyond providing a method handle. Every schema
// id in the snapshot must be currently registered.
func Load(data []byte) (*Store, error) {
	var snap snapshot
	if err := cbor.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("ecs: decode snapshot: %w", err)
	}
	if snap.Version != snapshotVersion {
		return nil, fmt.Errorf("ecs: snapshot version %d, want %d", snap.Version, snapshotVersion)
	}

	s := New()
	s.tick = snap.Tick

	// Rebuild the slot table with recorded generations. Slots not listed as a
	// live entity are dead and go on the freelist so future Create reuses them.
	slotCount := len(snap.Generations)
	s.slots = make([]slot, slotCount)
	liveIdx := make(map[uint32]struct{}, len(snap.Entities))
	for _, se := range snap.Entities {
		liveIdx[se.Index] = struct{}{}
	}
	for i := 0; i < slotCount; i++ {
		idx := uint32(i)
		gen := snap.Generations[i]
		if _, ok := liveIdx[idx]; ok {
			s.slots[i] = slot{generation: gen, alive: true, loc: location{arch: s.empty}}
			row := s.empty.push(makeEntity(idx, gen))
			s.slots[i].loc.row = row
		} else {
			// dead slot: preserve its generation so reuse continues correctly
			s.slots[i] = slot{generation: gen, alive: false}
			s.freelist = append(s.freelist, idx)
		}
	}

	// Replay components: decode each column and re-Add to each owning entity.
	// Re-using Add rebuilds archetypes and the per-row tick stamps below.
	for _, sc := range snap.Components {
		info, ok := registry.bySchemaLocked(sc.SchemaID)
		if !ok {
			return nil, fmt.Errorf("ecs: snapshot has unregistered component %q", sc.SchemaID)
		}
		col := info.newColumn()
		if err := col.unmarshalValues(sc.Values, len(sc.Entities)); err != nil {
			return nil, fmt.Errorf("ecs: decode component %s: %w", sc.SchemaID, err)
		}
		if err := s.restoreColumn(info, sc, col); err != nil {
			return nil, err
		}
	}

	// Relations.
	for _, rel := range snap.Relations {
		for i := range rel.Sources {
			src := Entity(rel.Sources[i])
			dst := Entity(rel.Targets[i])
			if s.Alive(src) && s.Alive(dst) {
				s.Relate(rel.Label, src, dst)
			}
		}
	}

	return s, nil
}

// restoreColumn migrates each listed entity into the archetype holding info and
// writes its decoded value + original tick. It bypasses the public Add stamping
// (which would overwrite ticks with the current tick) so change history
// survives the round trip.
func (s *Store) restoreColumn(info *componentInfo, sc snapComponent, decoded column) error {
	for i, packed := range sc.Entities {
		e := Entity(packed)
		if !s.Alive(e) {
			return fmt.Errorf("ecs: snapshot component %s references dead entity %d", sc.SchemaID, e)
		}
		sl := &s.slots[e.index()]
		mask := sl.loc.arch.mask
		if !mask.has(info.id) {
			mask.set(info.id)
			dst := s.archetypeFor(mask)
			s.migrate(e, dst)
		}
		loc := s.slots[e.index()].loc
		dstCol := loc.arch.columns[info.id]
		// copy decoded value at row i into the entity's row, with its tick
		if err := dstCol.copyRowFrom(loc.row, decoded, i); err != nil {
			return err
		}
		dstCol.ticks()[loc.row] = sc.Ticks[i]
	}
	return nil
}
