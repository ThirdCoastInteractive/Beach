package ecs

// archetype is one table: the set of entities that hold exactly the component
// set named by mask. Storage is columnar (SoA) — one column per component,
// indexed in lockstep with the entities slice. entities[row] is the Entity
// living at row r; columns[id][row] is its component value for component id.
type archetype struct {
	mask     componentMask
	ids      []componentID // sorted component ids present, for iteration
	columns  map[componentID]column
	entities []Entity // row -> entity
}

func newArchetype(mask componentMask) *archetype {
	a := &archetype{mask: mask, columns: make(map[componentID]column)}
	for id := componentID(0); int(id) < maxComponents; id++ {
		if mask.has(id) {
			a.ids = append(a.ids, id)
		}
	}
	return a
}

// location records where an entity's data lives: which archetype and which row.
type location struct {
	arch *archetype
	row  int
}

// slot is the entity table entry for one index. generation increments on each
// destroy so stale handles are detected. alive distinguishes a live slot from a
// freed one awaiting reuse.
type slot struct {
	generation uint32
	alive      bool
	loc        location
}

// Store is the container: an entity table plus a set of archetype tables. It
// does not tick — the caller owns the clock and passes it in via SetTick. Not
// safe for concurrent use; the sim drives it from a single goroutine.
type Store struct {
	slots      []slot
	freelist   []uint32 // freed slot indices available for reuse
	archetypes map[componentMask]*archetype
	empty      *archetype // archetype with no components (newly created entities)
	tick       Tick
	relations  map[string]*relationIndex // lazily created; nil until first Relate
}

// New returns an empty Store at tick 0.
func New() *Store {
	s := &Store{archetypes: make(map[componentMask]*archetype)}
	s.empty = s.archetypeFor(componentMask{})
	return s
}

// Tick returns the Store's current logical tick.
func (s *Store) Tick() Tick { return s.tick }

// SetTick advances the Store's clock. Subsequent mutations stamp columns with
// this value. The caller (sim) sets it once at the top of each tick. Going
// backwards is allowed but pointless; Changed[T] compares with >.
func (s *Store) SetTick(t Tick) { s.tick = t }

// Len reports the number of live entities.
func (s *Store) Len() int { return len(s.slots) - len(s.freelist) }

// archetypeFor returns the archetype for an exact component mask, creating it
// on first request.
func (s *Store) archetypeFor(mask componentMask) *archetype {
	if a, ok := s.archetypes[mask]; ok {
		return a
	}
	a := newArchetype(mask)
	for _, id := range a.ids {
		a.columns[id] = registry.byTypeOf(id).newColumn()
	}
	s.archetypes[mask] = a
	return a
}

// Create makes a new entity with no components and returns its handle. The
// entity lives in the empty archetype until components are added.
func (s *Store) Create() Entity {
	var index uint32
	if n := len(s.freelist); n > 0 {
		index = s.freelist[n-1]
		s.freelist = s.freelist[:n-1]
	} else {
		index = uint32(len(s.slots))
		s.slots = append(s.slots, slot{})
	}
	sl := &s.slots[index]
	sl.alive = true
	row := s.empty.push(makeEntity(index, sl.generation))
	sl.loc = location{arch: s.empty, row: row}
	return makeEntity(index, sl.generation)
}

// Alive reports whether e refers to a currently-live entity. A handle whose
// generation no longer matches its slot (the slot was destroyed and reused) is
// not alive — this is the stale-handle detection.
func (s *Store) Alive(e Entity) bool {
	i := e.index()
	if int(i) >= len(s.slots) {
		return false
	}
	sl := &s.slots[i]
	return sl.alive && sl.generation == e.generation()
}

// Destroy removes an entity and all its components. The slot's generation is
// bumped so any surviving handle becomes stale. Destroying an already-dead or
// stale handle is a no-op returning false.
func (s *Store) Destroy(e Entity) bool {
	if !s.Alive(e) {
		return false
	}
	i := e.index()
	sl := &s.slots[i]
	s.relationsOnDestroy(e)
	s.removeFromArchetype(sl.loc)
	sl.alive = false
	sl.generation++ // stale-handle detection: old handle no longer matches
	sl.loc = location{}
	s.freelist = append(s.freelist, i)
	return true
}

// push appends an entity to an archetype, zeroing its columns at the Store's
// current tick. Returns the new row. The caller stamps real values after.
func (a *archetype) push(e Entity) int {
	row := len(a.entities)
	a.entities = append(a.entities, e)
	return row
}

// removeFromArchetype removes the entity at loc via swap-remove, fixing up the
// swapped entity's recorded row. Columns swap-remove in lockstep.
func (s *Store) removeFromArchetype(loc location) {
	a := loc.arch
	last := len(a.entities) - 1
	moved := a.entities[last]
	a.entities[loc.row] = moved
	a.entities = a.entities[:last]
	for _, id := range a.ids {
		a.columns[id].swapRemove(loc.row)
	}
	if loc.row != last {
		// the entity that was at the tail now lives at loc.row
		ms := &s.slots[moved.index()]
		ms.loc.row = loc.row
	}
}

// migrate moves entity e from its current archetype into dst (which differs by
// exactly one component), copying every shared component column row-for-row and
// preserving stored ticks. Returns the new row in dst. The added/removed
// component's column in dst is left zero-valued (caller stamps it) or simply
// absent (on remove).
func (s *Store) migrate(e Entity, dst *archetype) int {
	sl := &s.slots[e.index()]
	src := sl.loc.arch
	srcRow := sl.loc.row

	newRow := dst.push(e)
	for _, id := range dst.ids {
		col := dst.columns[id]
		if srcCol, ok := src.columns[id]; ok {
			col.appendFrom(srcCol, srcRow)
		} else {
			col.appendZero(s.tick) // newly added component
		}
	}
	s.removeFromArchetype(sl.loc)
	sl.loc = location{arch: dst, row: newRow}
	return newRow
}
