package ecs

// Relationships are entity-valued edges with a storage-maintained reverse
// index. The motivating queries are owner->items and item->trade: given a
// target entity, find every source that points at it, in O(1)+O(k).
//
// Rather than encode relationships inside component structs (which would force
// reflection to find Entity-typed fields), the Store exposes them as named
// directed edges. A "relation" is just a string label (e.g. "owns",
// "listed_in"). Each source entity has at most one target per relation — the
// common cardinality (an item has one owner, one trade) — which keeps the
// forward map a plain entity->entity lookup. owner->items is the reverse.
//
// Edges are not tick-stamped (they are structural, not rendered component
// state); Changed[T] does not see them. When either endpoint is destroyed, the
// Store removes every edge touching it so the reverse index never dangles.

// relationKey identifies one relation label's index.
type relationIndex struct {
	forward map[Entity]Entity              // source -> target (one per source)
	reverse map[Entity]map[Entity]struct{} // target -> set of sources
}

func newRelationIndex() *relationIndex {
	return &relationIndex{
		forward: make(map[Entity]Entity),
		reverse: make(map[Entity]map[Entity]struct{}),
	}
}

// relations lazily holds one index per relation label. nil until first Relate.
func (s *Store) relationFor(relation string) *relationIndex {
	if s.relations == nil {
		s.relations = make(map[string]*relationIndex)
	}
	idx, ok := s.relations[relation]
	if !ok {
		idx = newRelationIndex()
		s.relations[relation] = idx
	}
	return idx
}

// Relate sets the edge (src -[relation]-> dst), replacing any existing target
// for src under this relation and updating the reverse index. Both endpoints
// must be live. Returns false if either is dead/stale.
func (s *Store) Relate(relation string, src, dst Entity) bool {
	if !s.Alive(src) || !s.Alive(dst) {
		return false
	}
	idx := s.relationFor(relation)
	if old, ok := idx.forward[src]; ok {
		idx.removeReverse(old, src)
	}
	idx.forward[src] = dst
	rs := idx.reverse[dst]
	if rs == nil {
		rs = make(map[Entity]struct{})
		idx.reverse[dst] = rs
	}
	rs[src] = struct{}{}
	return true
}

// Unrelate removes src's edge under relation, if any. Returns true if an edge
// was removed.
func (s *Store) Unrelate(relation string, src Entity) bool {
	if s.relations == nil {
		return false
	}
	idx, ok := s.relations[relation]
	if !ok {
		return false
	}
	old, ok := idx.forward[src]
	if !ok {
		return false
	}
	delete(idx.forward, src)
	idx.removeReverse(old, src)
	return true
}

// Target returns the entity src points at under relation, and true, or the zero
// Entity and false if there is no such edge.
func (s *Store) Target(relation string, src Entity) (Entity, bool) {
	if s.relations == nil {
		return 0, false
	}
	idx, ok := s.relations[relation]
	if !ok {
		return 0, false
	}
	t, ok := idx.forward[src]
	return t, ok
}

// Sources returns every source pointing at dst under relation (the reverse
// query: owner->items). The returned slice is freshly allocated and the
// caller owns it; order is unspecified.
func (s *Store) Sources(relation string, dst Entity) []Entity {
	if s.relations == nil {
		return nil
	}
	idx, ok := s.relations[relation]
	if !ok {
		return nil
	}
	rs := idx.reverse[dst]
	if len(rs) == 0 {
		return nil
	}
	out := make([]Entity, 0, len(rs))
	for src := range rs {
		out = append(out, src)
	}
	return out
}

func (idx *relationIndex) removeReverse(dst, src Entity) {
	rs := idx.reverse[dst]
	if rs == nil {
		return
	}
	delete(rs, src)
	if len(rs) == 0 {
		delete(idx.reverse, dst)
	}
}

// relationsOnDestroy purges every edge touching e (as source or as target)
// across all relations. Called by Destroy so the reverse index never points at
// a dead entity.
func (s *Store) relationsOnDestroy(e Entity) {
	if s.relations == nil {
		return
	}
	for _, idx := range s.relations {
		// e as source
		if dst, ok := idx.forward[e]; ok {
			delete(idx.forward, e)
			idx.removeReverse(dst, e)
		}
		// e as target: drop every source that pointed at e
		if rs, ok := idx.reverse[e]; ok {
			for src := range rs {
				delete(idx.forward, src)
			}
			delete(idx.reverse, e)
		}
	}
}
