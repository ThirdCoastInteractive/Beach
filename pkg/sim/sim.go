// Package sim runs a simulation over an ecs.Store: a single goroutine owns the
// store, drives a fixed tick loop, drains a typed command channel at the top of
// each tick, runs systems in registration order, then projects the tick's dirty
// component set onto hub topics as pre-rendered patches.
//
// One writer, no locks. Determinism is the feature: construct a Sim, Send
// commands, Tick N times, assert. See docs/architecture/08-sim.md.
//
// The web layer never touches the store. Handlers Send fire-and-forget commands
// or Ask request/reply commands; everything mutating the store runs on the loop
// goroutine inside a Command's Apply.
package sim

import (
	"context"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/ecs"
	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
)

// Tick is the simulation's logical clock value, mirroring ecs.Tick.
type Tick = ecs.Tick

// Command is a unit of work applied to the store on the loop goroutine. Apply
// runs single-threaded at the top of a tick, before systems, with exclusive
// store access — it may freely Create/Add/Set/Remove. Commands are the only way
// the outside world mutates the store.
//
// Most commands are plain structs implementing Apply. Request/reply commands
// (see Ask) carry a reply channel in their own fields and send on it from
// within Apply.
type Command interface {
	Apply(w *World)
}

// World is the mutation surface handed to Commands and Systems on the loop
// goroutine. It wraps the ecs.Store (use the ecs.* generic helpers with
// w.Store) and exposes the current tick plus deterministic randomness.
//
// World is not safe for concurrent use; it is only ever touched from the single
// loop goroutine.
type World struct {
	Store *ecs.Store
	tick  Tick
	rng   *rng
}

// Tick returns the tick currently being processed.
func (w *World) Tick() Tick { return w.tick }

// Rand returns a deterministic uint64 from the sim's seeded PRNG. Two runs with
// the same seed and the same command/tick sequence produce identical streams —
// this is what keeps dice rolls and drops reproducible. Never use math/rand or
// crypto/rand inside a Command or System; use this.
func (w *World) Rand() uint64 { return w.rng.next() }

// Intn returns a deterministic int in [0, n) from the sim PRNG. Panics if n<=0,
// matching math/rand semantics.
func (w *World) Intn(n int) int { return w.rng.intn(n) }

// System is a plain function over the store, run every tick in registration
// order after commands drain. Systems read and Mutate components; they must not
// block, spawn goroutines that touch the store, or retain the World past the
// call. Structural mutation (Create/Destroy/Add/Remove) is allowed but must not
// happen while iterating the same store via ecs.Query — query the rarer
// component and Get the rest, or collect-then-mutate.
type System func(w *World)

// Config configures a Sim. Zero values get sensible defaults in New.
type Config struct {
	// TickRate is the simulation step frequency in Hz (steps per second).
	// Defaults to 20.
	TickRate int
	// ProjectRate is the projection/fan-out frequency in Hz. It should divide
	// into TickRate; projections run every TickRate/ProjectRate ticks. Defaults
	// to 8 (clamped so it never exceeds TickRate).
	ProjectRate int
	// Seed seeds the deterministic PRNG. Defaults to 1.
	Seed uint64
	// CommandBuffer bounds the inbound command channel. Defaults to 1024.
	CommandBuffer int
	// Hub receives projection patches. May be nil (projections then render but
	// publish nowhere — useful in tests).
	Hub *hub.Hub
	// Store lets a caller supply a pre-populated store (e.g. from a snapshot
	// restore). Nil means a fresh ecs.New().
	Store *ecs.Store
	// Now returns the current wall time; injectable for tests. Defaults to
	// time.Now. Only used by the wall-clock Run loop, never by manual Tick.
	Now func() time.Time
}

func (c Config) withDefaults() Config {
	if c.TickRate <= 0 {
		c.TickRate = 20
	}
	if c.ProjectRate <= 0 {
		c.ProjectRate = 8
	}
	if c.ProjectRate > c.TickRate {
		c.ProjectRate = c.TickRate
	}
	if c.Seed == 0 {
		c.Seed = 1
	}
	if c.CommandBuffer <= 0 {
		c.CommandBuffer = 1024
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Sim is the single-writer simulation. Construct with New, register systems and
// projections, then either drive it manually with Tick (tests) or hand it to
// Run (production wall-clock loop). All exported methods except the loop-side
// internals are safe to call from any goroutine *before* Run starts; once Run
// owns the loop, mutate only via Send/Ask.
type Sim struct {
	cfg     Config
	world   *World
	systems []System
	projs   []projection
	// projectEvery is how many ticks pass between projection passes.
	projectEvery int

	commands chan Command

	// lastProjectTick is the tick the previous projection pass ran at; the next
	// pass collects ecs.Changed since this value. Owned by the loop goroutine.
	lastProjectTick Tick

	// behind holds the write-behind lane (may be nil).
	behind *writeBehind
	// journal holds the append-only journal lane (may be nil).
	journal Journal
}

// New constructs a Sim from cfg. It does not start ticking; call Tick manually
// or Run. Systems and projections must be registered before Run (or before the
// first Tick) — registration is not safe once the loop owns the Sim.
func New(cfg Config) *Sim {
	cfg = cfg.withDefaults()
	store := cfg.Store
	if store == nil {
		store = ecs.New()
	}
	s := &Sim{
		cfg: cfg,
		world: &World{
			Store: store,
			tick:  store.Tick(),
			rng:   newRNG(cfg.Seed),
		},
		commands:        make(chan Command, cfg.CommandBuffer),
		projectEvery:    cfg.TickRate / cfg.ProjectRate,
		lastProjectTick: store.Tick(),
	}
	if s.projectEvery < 1 {
		s.projectEvery = 1
	}
	return s
}

// Store returns the underlying ecs.Store. Use it only before Run starts (e.g.
// to seed initial entities) or from inside a Command/System via World — touching
// it concurrently with the loop is a data race.
func (s *Sim) Store() *ecs.Store { return s.world.Store }

// TickRate reports the configured simulation frequency in Hz.
func (s *Sim) TickRate() int { return s.cfg.TickRate }

// AddSystems appends systems to run every tick, in the order given. Call before
// Run. Later calls append after earlier ones; order is fixed and meaningful.
func (s *Sim) AddSystems(systems ...System) {
	s.systems = append(s.systems, systems...)
}

// UseWriteBehind installs the write-behind persistence lane: the given Flusher
// receives the batch of dirty component rows accumulated since the last flush,
// once per projection pass. Pass nil to disable. See persist.go.
func (s *Sim) UseWriteBehind(wb *writeBehind) { s.behind = wb }

// UseJournal installs the append-only journal lane. Commands that implement
// Journaled have their entries appended (on the loop goroutine, before Apply
// mutates) so the journal is the authoritative ordered log. See persist.go.
func (s *Sim) UseJournal(j Journal) { s.journal = j }

// Send enqueues a fire-and-forget command. It blocks only if the command buffer
// is full (back-pressure on a saturated sim); it never blocks on the loop. Safe
// from any goroutine. Returns ctx.Err() if ctx is cancelled while waiting for
// buffer space.
func (s *Sim) Send(ctx context.Context, cmd Command) error {
	select {
	case s.commands <- cmd:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendNonBlocking enqueues cmd if the buffer has room and reports whether it was
// accepted. It never blocks — useful on hot paths that prefer to drop rather
// than stall (presence pings, say).
func (s *Sim) SendNonBlocking(cmd Command) bool {
	select {
	case s.commands <- cmd:
		return true
	default:
		return false
	}
}

// Tick advances the simulation by exactly one step and returns the new tick.
// This is the deterministic test entrypoint and the unit Run calls on each
// wall-clock beat. It:
//
//  1. advances the store clock,
//  2. drains all currently-queued commands and Applies them in FIFO order,
//  3. runs every system in registration order,
//  4. if this tick is a projection tick, collects the dirty set and projects.
//
// Tick must only be called from one goroutine (the loop, or a test). It drains
// only commands already enqueued when it starts, so a test can Send N commands
// then Tick once and know exactly what was processed.
func (s *Sim) Tick() Tick {
	w := s.world
	w.tick++
	w.Store.SetTick(w.tick)

	s.drainCommands(w)

	for _, sys := range s.systems {
		sys(w)
	}

	if int(w.tick)%s.projectEvery == 0 {
		s.project(w)
	}
	return w.tick
}

// drainCommands applies every command currently buffered, without blocking on
// new arrivals. It snapshots the queue length first so commands enqueued by an
// Apply (re-entrancy) land on the next tick, preserving determinism.
func (s *Sim) drainCommands(w *World) {
	n := len(s.commands)
	for i := 0; i < n; i++ {
		select {
		case cmd := <-s.commands:
			if s.journal != nil {
				if j, ok := cmd.(Journaled); ok {
					s.journal.Append(w.tick, j.JournalEntry())
				}
			}
			cmd.Apply(w)
		default:
			return
		}
	}
}

// Run drives the wall-clock tick loop until ctx is cancelled, ticking at the
// configured TickRate. It is the production entrypoint; it owns the loop
// goroutine and the store for its lifetime. On ctx cancellation it drains no
// further commands and returns. Run blocks; callers typically `go s.Run(ctx)`.
func (s *Sim) Run(ctx context.Context) {
	interval := time.Second / time.Duration(s.cfg.TickRate)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick()
		}
	}
}

// Save serializes the current store to a CBOR snapshot (see ecs.Save). Call it
// only on the loop goroutine (e.g. from a Command) or before Run starts; it
// reads the store and races a live loop otherwise.
func (s *Sim) Save() ([]byte, error) { return s.world.Store.Save() }

// Restore replaces a Sim's store from a snapshot. It is intended for boot: build
// the Sim with Config.Store set from RestoreStore instead. Provided here for
// symmetry where the caller already holds the Sim and has not started Run.
func RestoreStore(data []byte) (*ecs.Store, error) { return ecs.Load(data) }
