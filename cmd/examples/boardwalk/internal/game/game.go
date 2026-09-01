package game

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/boardwalk/internal/analytics"
	"github.com/ThirdCoastInteractive/Beach/pkg/ch"
	"github.com/ThirdCoastInteractive/Beach/pkg/ecs"
	"github.com/ThirdCoastInteractive/Beach/pkg/sim"
)

// Topics. The board everyone watches (spectators included) is one hub topic;
// each player's private hand/cash is their own user:<seat> topic.
const BoardTopic = "board"

// UserTopic is the per-seat private hand topic.
func UserTopic(seat int) string { return fmt.Sprintf("user:%d", seat) }

// Board is a singleton component on the game entity. It is the projection
// trigger for the shared board surface: any command that changes the game
// stamps it (via touchBoard), and the board projection re-renders the whole
// board + log once per pass. The data it carries (log, turn pointer) is the
// derived view state the renderer needs that is not itself an entity component.
type Board struct {
	Turn   int // seat whose turn it is
	LastN  int // last seat that acted (for highlight)
	LastD1 int // last dice faces
	LastD2 int
	Log    []string // newest-last game log lines
	Joined int      // number of claimed seats
	Over   bool
}

// Renderers are the view hooks the sim projections render through. They live in
// the web package; injecting them keeps game free of any web import (game must
// not import web). Each returns the rendered fragment bytes for a hub.Event.
type Renderers struct {
	Board func(snap Snapshot) []byte
	Hand  func(snap Snapshot, seat int) []byte
}

// Game holds the sim plus the fixed entity handles the handlers need. It is the
// app's single game instance. Players, the property deeds, and the singleton
// Board entity are all created up front so seats can be claimed without any
// structural mutation racing the loop.
type Game struct {
	sim    *sim.Sim
	events *ch.Batcher[analytics.Event] // the ClickHouse action firehose
	render Renderers

	mu      sync.Mutex
	gameEnt ecs.Entity         // carries the Board singleton
	seats   []ecs.Entity       // seat -> player entity (always present)
	seatOf  map[ecs.Entity]int // player entity -> seat index
	deeds   map[int]ecs.Entity // tile index -> deed entity (properties only)
}

// RegisterComponents registers every component type with the ecs registry under
// its stable schema id. Restoring a snapshot (ecs.Load) maps each saved column
// back to a type by schema id, so the types MUST be registered before the load —
// otherwise restore fails with "unregistered component". Fresh boots register
// lazily on the first ecs.Add, but restore happens before any Add, so main calls
// this up front. Idempotent: re-registering a type is a no-op.
func RegisterComponents() {
	ecs.Register[Board]("boardwalk.Board")
	ecs.Register[Player]("boardwalk.Player")
	ecs.Register[Position]("boardwalk.Position")
	ecs.Register[Cash]("boardwalk.Cash")
	ecs.Register[TurnOrder]("boardwalk.TurnOrder")
	ecs.Register[Ownership]("boardwalk.Ownership")
}

// NewGame builds the Game over the sim's store: it either re-derives the entity
// handles from a restored snapshot (the store already holds a Board singleton)
// or seeds a fresh board, then registers the projections. It does not start the
// loop; call Run. events is the firehose every mutation emits to; render holds
// the web view hooks the projections render through.
func NewGame(s *sim.Sim, events *ch.Batcher[analytics.Event], render Renderers) *Game {
	g := &Game{
		sim:    s,
		events: events,
		render: render,
		deeds:  make(map[int]ecs.Entity),
		seatOf: make(map[ecs.Entity]int),
	}
	store := s.Store()

	// A restored store already carries the singleton Board entity; rebuild the
	// handle maps from it instead of creating fresh entities. A fresh store has
	// none, so seed.
	restored := false
	for range ecs.Query[Board](store) {
		restored = true
		break
	}
	if restored {
		g.rebuild(store)
	} else {
		g.seed(store)
	}

	g.registerProjections()
	return g
}

// seed creates the initial entities for a fresh game: the singleton Board, the
// fixed player seats, and one deed per buyable tile. Pre-creating everything up
// front avoids structural mutation on the hot path.
func (g *Game) seed(store *ecs.Store) {
	// The singleton Board entity.
	g.gameEnt = store.Create()
	ecs.Add(store, g.gameEnt, Board{
		Turn: 0,
		Log:  []string{"Welcome to Boardwalk. Claim a seat to play."},
	})

	// Fixed player seats. Each is a full player entity (Position/Cash/TurnOrder)
	// flagged as an open Spec seat until claimed.
	tokens := []string{"🦀", "🐚", "⛵", "🏖️"}
	g.seats = make([]ecs.Entity, MaxSeats)
	for i := 0; i < MaxSeats; i++ {
		e := store.Create()
		ecs.Add(store, e, Player{Name: fmt.Sprintf("Seat %d", i+1), Token: tokens[i], Spec: true})
		ecs.Add(store, e, Position{Square: 0})
		ecs.Add(store, e, Cash{Amount: StartingCash})
		ecs.Add(store, e, TurnOrder{Seat: i})
		g.seats[i] = e
		g.seatOf[e] = i
	}

	// Property deeds, one entity per buyable tile, owned by the bank initially.
	for idx, t := range Tiles {
		if t.Kind != KindProperty {
			continue
		}
		e := store.Create()
		ecs.Add(store, e, Ownership{Tile: idx})
		g.deeds[idx] = e
	}
}

// rebuild re-derives the Game's entity handles from a restored store. The
// snapshot preserved entity identity, so the restored Ownership.Owner handles
// still point at the right player entities; we just need to find each handle
// again. Seats are indexed by TurnOrder.Seat (not iteration order) so seat i is
// the same player it was before the restore.
func (g *Game) rebuild(store *ecs.Store) {
	// The Board singleton is the lone entity carrying a Board component.
	for e := range ecs.Query[Board](store) {
		g.gameEnt = e
		break
	}

	// Seats, indexed by their stable TurnOrder.Seat.
	g.seats = make([]ecs.Entity, MaxSeats)
	for e, to := range ecs.Query[TurnOrder](store) {
		if to.Seat < 0 || to.Seat >= MaxSeats {
			continue
		}
		g.seats[to.Seat] = e
		g.seatOf[e] = to.Seat
	}

	// Deeds, indexed by the tile they cover.
	for e, own := range ecs.Query[Ownership](store) {
		g.deeds[own.Tile] = e
	}
}

// registerProjections wires change detection to hub topics. The Board singleton
// drives the shared board surface; each player's Cash change drives their private
// hand on their user:<seat> topic. Position/Ownership changes always coincide
// with a Board touch in the same command, so the board re-render covers them.
func (g *Game) registerProjections() {
	store := g.sim.Store()

	// Shared board: any Board stamp re-renders the whole surface to "board".
	sim.Project(g.sim, sim.Projection[Board]{
		Topic: func(e ecs.Entity, _ Board) string { return BoardTopic },
		View: func(e ecs.Entity, _ Board) []byte {
			snap := snapshotBoard(store, g.seats, g.deeds, g.gameEnt)
			return g.render.Board(snap)
		},
	})

	// Per-player hand: a Cash change renders that seat's private hand to its
	// user:<seat> topic. Cash is the natural trigger (every money move stamps it).
	sim.Project(g.sim, sim.Projection[Cash]{
		Topic: func(e ecs.Entity, _ Cash) string {
			seat, ok := g.seatOf[e]
			if !ok {
				return ""
			}
			return UserTopic(seat)
		},
		View: func(e ecs.Entity, _ Cash) []byte {
			seat, ok := g.seatOf[e]
			if !ok {
				return nil
			}
			snap := snapshotBoard(store, g.seats, g.deeds, g.gameEnt)
			return g.render.Hand(snap, seat)
		},
	})
}

// Snapshot builds a read-only view of the board off the loop for catch-up
// rendering. It Asks the loop so the read is consistent and race-free.
func (g *Game) Snapshot(ctx context.Context) (Snapshot, error) {
	return sim.AskFunc(ctx, g.sim, func(w *sim.World) Snapshot {
		return snapshotBoard(w.Store, g.seats, g.deeds, g.gameEnt)
	})
}

// snapshotBlob pairs a CBOR store snapshot with the tick it was taken at.
type snapshotBlob struct {
	data []byte
	tick int64
}

// SaveSnapshot serializes the live store to a CBOR blob on the loop goroutine
// (via AskFunc, so it never races the loop) and returns it with the current
// tick. The save loop in main calls this periodically and persists the result.
func (g *Game) SaveSnapshot(ctx context.Context) ([]byte, int64, error) {
	res, err := sim.AskFunc(ctx, g.sim, func(w *sim.World) snapshotBlob {
		data, serr := w.Store.Save()
		if serr != nil {
			return snapshotBlob{}
		}
		return snapshotBlob{data: data, tick: int64(w.Tick())}
	})
	if err != nil {
		return nil, 0, err
	}
	if res.data == nil {
		return nil, 0, fmt.Errorf("boardwalk: store save failed")
	}
	return res.data, res.tick, nil
}

// Run drives the sim loop until ctx is cancelled.
func (g *Game) Run(ctx context.Context) { g.sim.Run(ctx) }

// touchBoard re-stamps the Board singleton so the next projection pass renders
// the shared surface. Call from inside a Command's Apply after any change.
func touchBoard(w *sim.World, ent ecs.Entity, fn func(b *Board)) {
	ecs.Mutate(w.Store, ent, fn)
}

// ---- Commands ----

// joinCmd claims the next open seat for a display name. Returns the seat index
// (or -1 if the table is full) via reply.
type joinCmd struct {
	game  *Game
	name  string
	reply func(int)
}

func (c joinCmd) Apply(w *sim.World) {
	store := w.Store
	seat := -1
	for i, e := range c.game.seats {
		p, _ := ecs.Get[Player](store, e)
		if p.Spec {
			p.Spec = false
			if c.name != "" {
				p.Name = c.name
			}
			ecs.Set(store, e, p)
			seat = i
			break
		}
	}
	if seat >= 0 {
		jp, _ := ecs.Get[Player](store, c.game.seats[seat])
		touchBoard(w, c.game.gameEnt, func(b *Board) {
			b.Joined++
			b.Log = appendLog(b.Log, fmt.Sprintf("%s joined as %s.", jp.Name, jp.Token))
		})
		c.game.emit(analytics.Event{Kind: "join", Seat: int32(seat), Token: jp.Token})
	}
	c.reply(seat)
}

// RollResult is what an Ask roll returns to the handler.
type RollResult struct {
	OK  bool
	Msg string
	D1  int
	D2  int
}

// rollCmd is the heart of the game: validate it is seat's turn, roll two dice
// off the deterministic sim PRNG, move, resolve the landed tile (GO salary,
// buy/rent, tax, chance), then advance the turn. All of it runs single-threaded
// inside Apply, so the result the handler renders is consistent.
type rollCmd struct {
	game  *Game
	seat  int
	reply func(RollResult)
}

func (c rollCmd) Apply(w *sim.World) {
	store := w.Store
	g := c.game
	ent := g.seats[c.seat]

	pl, _ := ecs.Get[Player](store, ent)
	if pl.Spec {
		c.reply(RollResult{OK: false, Msg: "That seat is open — claim it first."})
		return
	}
	board0, _ := ecs.Get[Board](store, g.gameEnt)
	if board0.Turn != c.seat {
		c.reply(RollResult{OK: false, Msg: "Not your turn."})
		return
	}

	d1 := w.Intn(6) + 1
	d2 := w.Intn(6) + 1
	steps := d1 + d2

	pos, _ := ecs.Get[Position](store, ent)
	from := pos.Square
	to := (from + steps) % len(Tiles)
	ecs.Set(store, ent, Position{Square: to})

	lines := []string{fmt.Sprintf("%s rolled %d+%d=%d → %s", pl.Token, d1, d2, steps, Tiles[to].Name)}
	g.emit(analytics.Event{Kind: "roll", Seat: int32(c.seat), Token: pl.Token, Tile: int32(to), Name: Tiles[to].Name})

	// Passing GO (wrap-around) pays salary.
	if to < from {
		addCash(store, ent, Salary)
		lines = append(lines, fmt.Sprintf("%s passed GO, collected $%d.", pl.Token, Salary))
		g.emit(analytics.Event{Kind: "pass_go", Seat: int32(c.seat), Token: pl.Token, Delta: int64(Salary)})
	}

	lines = append(lines, c.resolveTile(w, ent, pl, to)...)

	// Advance to the next claimed seat.
	next := g.nextSeat(store, c.seat)

	touchBoard(w, g.gameEnt, func(b *Board) {
		b.LastN = c.seat
		b.LastD1, b.LastD2 = d1, d2
		b.Turn = next
		for _, ln := range lines {
			b.Log = appendLog(b.Log, ln)
		}
	})

	c.reply(RollResult{OK: true, Msg: lines[len(lines)-1], D1: d1, D2: d2})
}

// resolveTile applies the effect of landing on tile `to` and returns log lines.
func (c rollCmd) resolveTile(w *sim.World, ent ecs.Entity, pl Player, to int) []string {
	store := w.Store
	g := c.game
	t := Tiles[to]
	var out []string

	switch t.Kind {
	case KindGo:
		addCash(store, ent, Salary)
		out = append(out, fmt.Sprintf("%s landed on GO, collected $%d.", pl.Token, Salary))

	case KindTax:
		addCash(store, ent, -t.Price)
		out = append(out, fmt.Sprintf("%s paid $%d %s.", pl.Token, t.Price, t.Name))
		g.emit(analytics.Event{Kind: "tax", Seat: int32(c.seat), Token: pl.Token, Tile: int32(to), Name: t.Name, Delta: int64(-t.Price)})

	case KindChance:
		// Deterministic swing off the sim PRNG: -$60..+$80 in $20 steps.
		delta := (w.Intn(8) - 3) * 20
		addCash(store, ent, delta)
		g.emit(analytics.Event{Kind: "chance", Seat: int32(c.seat), Token: pl.Token, Tile: int32(to), Name: t.Name, Delta: int64(delta)})
		if delta >= 0 {
			out = append(out, fmt.Sprintf("%s drew Chance: +$%d.", pl.Token, delta))
		} else {
			out = append(out, fmt.Sprintf("%s drew Chance: -$%d.", pl.Token, -delta))
		}

	case KindProperty:
		deed := g.deeds[to]
		own, _ := ecs.Get[Ownership](store, deed)
		switch {
		case own.Owner == 0:
			// Auto-buy if affordable (v1: landing on an unowned property buys it).
			cash, _ := ecs.Get[Cash](store, ent)
			if cash.Amount >= t.Price {
				addCash(store, ent, -t.Price)
				ecs.Set(store, deed, Ownership{Tile: to, Owner: ent})
				out = append(out, fmt.Sprintf("%s bought %s for $%d.", pl.Token, t.Name, t.Price))
				g.emit(analytics.Event{Kind: "buy", Seat: int32(c.seat), Token: pl.Token, Tile: int32(to), Name: t.Name, Delta: int64(-t.Price)})
			} else {
				out = append(out, fmt.Sprintf("%s can't afford %s ($%d).", pl.Token, t.Name, t.Price))
			}
		case own.Owner == ent:
			out = append(out, fmt.Sprintf("%s already owns %s.", pl.Token, t.Name))
		default:
			// Pay rent to the owner.
			addCash(store, ent, -t.Rent)
			addCash(store, own.Owner, t.Rent)
			op, _ := ecs.Get[Player](store, own.Owner)
			out = append(out, fmt.Sprintf("%s paid $%d rent to %s for %s.", pl.Token, t.Rent, op.Token, t.Name))
			g.emit(analytics.Event{Kind: "rent", Seat: int32(c.seat), Token: pl.Token, Tile: int32(to), Name: t.Name, Delta: int64(-t.Rent)})
		}
	}
	return out
}

// nextSeat returns the next claimed (non-Spec) seat after `seat`, wrapping. If
// no other player has joined, it returns the same seat (solo play).
func (g *Game) nextSeat(store *ecs.Store, seat int) int {
	for i := 1; i <= MaxSeats; i++ {
		cand := (seat + i) % MaxSeats
		p, _ := ecs.Get[Player](store, g.seats[cand])
		if !p.Spec {
			return cand
		}
	}
	return seat
}

// ---- Ask wrappers (handler -> sim) ----

// Join claims a seat, returning the seat index or -1 if full.
func (g *Game) Join(ctx context.Context, name string) (int, error) {
	return sim.Ask(ctx, g.sim, func(reply func(int)) sim.Command {
		return joinCmd{game: g, name: name, reply: reply}
	})
}

// Roll rolls for a seat, returning the outcome.
func (g *Game) Roll(ctx context.Context, seat int) (RollResult, error) {
	return sim.Ask(ctx, g.sim, func(reply func(RollResult)) sim.Command {
		return rollCmd{game: g, seat: seat, reply: reply}
	})
}

// ---- helpers ----

// emit stamps the event with the wall time and hands it to the firehose. Add is
// non-blocking, so this is safe to call from inside a command's Apply on the
// loop goroutine. The guard keeps it a no-op if the batcher is somehow unset.
func (g *Game) emit(e analytics.Event) {
	if g.events == nil {
		return
	}
	e.TS = time.Now()
	g.events.Add(e)
}

func addCash(store *ecs.Store, e ecs.Entity, delta int) {
	ecs.Mutate(store, e, func(c *Cash) { c.Amount += delta })
}

func appendLog(log []string, line string) []string {
	log = append(log, line)
	const maxLog = 14
	if len(log) > maxLog {
		log = log[len(log)-maxLog:]
	}
	return log
}
