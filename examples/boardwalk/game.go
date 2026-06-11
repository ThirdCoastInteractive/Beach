package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/ThirdCoastInteractive/the-beach/ecs"
	"github.com/ThirdCoastInteractive/the-beach/sim"
)

// Topics. The board everyone watches (spectators included) is one hub topic;
// each player's private hand/cash is their own user:<seat> topic.
const boardTopic = "board"

func userTopic(seat int) string { return fmt.Sprintf("user:%d", seat) }

// maxSeats is the fixed table size. Seats fill as players join; unclaimed seats
// render as open. Keeping it small keeps the board legible.
const maxSeats = 4

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

// Game holds the sim plus the fixed entity handles the handlers need. It is the
// app's single game instance. Players, the property deeds, and the singleton
// Board entity are all created up front so seats can be claimed without any
// structural mutation racing the loop.
type Game struct {
	sim *sim.Sim

	mu      sync.Mutex
	gameEnt ecs.Entity         // carries the Board singleton
	seats   []ecs.Entity       // seat -> player entity (always present)
	seatOf  map[ecs.Entity]int // player entity -> seat index
	deeds   map[int]ecs.Entity // tile index -> deed entity (properties only)
}

// NewGame builds the sim, seeds the board entities, registers the projections,
// and returns the Game. It does not start the loop; call Run.
func NewGame(s *sim.Sim) *Game {
	g := &Game{sim: s, deeds: make(map[int]ecs.Entity), seatOf: make(map[ecs.Entity]int)}
	store := s.Store()

	// The singleton Board entity.
	g.gameEnt = store.Create()
	ecs.Add(store, g.gameEnt, Board{
		Turn: 0,
		Log:  []string{"Welcome to Boardwalk. Claim a seat to play."},
	})

	// Fixed player seats. Each is a full player entity (Position/Cash/TurnOrder)
	// flagged as an open Spec seat until claimed. Pre-creating avoids structural
	// mutation on the hot path.
	tokens := []string{"🦀", "🐚", "⛵", "🏖️"}
	g.seats = make([]ecs.Entity, maxSeats)
	for i := 0; i < maxSeats; i++ {
		e := store.Create()
		ecs.Add(store, e, Player{Name: fmt.Sprintf("Seat %d", i+1), Token: tokens[i], Spec: true})
		ecs.Add(store, e, Position{Square: 0})
		ecs.Add(store, e, Cash{Amount: startingCash})
		ecs.Add(store, e, TurnOrder{Seat: i})
		g.seats[i] = e
		g.seatOf[e] = i
	}

	// Property deeds, one entity per buyable tile, owned by the bank initially.
	for idx, t := range board {
		if t.Kind != kindProperty {
			continue
		}
		e := store.Create()
		ecs.Add(store, e, Ownership{Tile: idx})
		g.deeds[idx] = e
	}

	g.registerProjections()
	return g
}

// registerProjections wires change detection to hub topics. The Board singleton
// drives the shared board surface; each player's Cash change drives their private
// hand on their user:<seat> topic. Position/Ownership changes always coincide
// with a Board touch in the same command, so the board re-render covers them.
func (g *Game) registerProjections() {
	store := g.sim.Store()

	// Shared board: any Board stamp re-renders the whole surface to "board".
	sim.Project(g.sim, sim.Projection[Board]{
		Topic: func(e ecs.Entity, _ Board) string { return boardTopic },
		View: func(e ecs.Entity, _ Board) []byte {
			snap := snapshotBoard(store, g.seats, g.deeds, g.gameEnt)
			return renderToBytes(boardFragment(snap))
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
			return userTopic(seat)
		},
		View: func(e ecs.Entity, _ Cash) []byte {
			seat, ok := g.seatOf[e]
			if !ok {
				return nil
			}
			snap := snapshotBoard(store, g.seats, g.deeds, g.gameEnt)
			return renderToBytes(handFragment(snap, seat))
		},
	})
}

// Snapshot builds a read-only view of the board off the loop for catch-up
// rendering. It Asks the loop so the read is consistent and race-free.
func (g *Game) Snapshot(ctx context.Context) (boardSnapshot, error) {
	return sim.AskFunc(ctx, g.sim, func(w *sim.World) boardSnapshot {
		return snapshotBoard(w.Store, g.seats, g.deeds, g.gameEnt)
	})
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
	}
	c.reply(seat)
}

// rollResult is what an Ask roll returns to the handler.
type rollResult struct {
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
	reply func(rollResult)
}

func (c rollCmd) Apply(w *sim.World) {
	store := w.Store
	g := c.game
	ent := g.seats[c.seat]

	pl, _ := ecs.Get[Player](store, ent)
	if pl.Spec {
		c.reply(rollResult{OK: false, Msg: "That seat is open — claim it first."})
		return
	}
	board0, _ := ecs.Get[Board](store, g.gameEnt)
	if board0.Turn != c.seat {
		c.reply(rollResult{OK: false, Msg: "Not your turn."})
		return
	}

	d1 := w.Intn(6) + 1
	d2 := w.Intn(6) + 1
	steps := d1 + d2

	pos, _ := ecs.Get[Position](store, ent)
	from := pos.Square
	to := (from + steps) % len(board)
	ecs.Set(store, ent, Position{Square: to})

	lines := []string{fmt.Sprintf("%s rolled %d+%d=%d → %s", pl.Token, d1, d2, steps, board[to].Name)}

	// Passing GO (wrap-around) pays salary.
	if to < from {
		addCash(store, ent, salary)
		lines = append(lines, fmt.Sprintf("%s passed GO, collected $%d.", pl.Token, salary))
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

	c.reply(rollResult{OK: true, Msg: lines[len(lines)-1], D1: d1, D2: d2})
}

// resolveTile applies the effect of landing on tile `to` and returns log lines.
func (c rollCmd) resolveTile(w *sim.World, ent ecs.Entity, pl Player, to int) []string {
	store := w.Store
	g := c.game
	t := board[to]
	var out []string

	switch t.Kind {
	case kindGo:
		addCash(store, ent, salary)
		out = append(out, fmt.Sprintf("%s landed on GO, collected $%d.", pl.Token, salary))

	case kindTax:
		addCash(store, ent, -t.Price)
		out = append(out, fmt.Sprintf("%s paid $%d %s.", pl.Token, t.Price, t.Name))

	case kindChance:
		// Deterministic swing off the sim PRNG: -$60..+$80 in $20 steps.
		delta := (w.Intn(8) - 3) * 20
		addCash(store, ent, delta)
		if delta >= 0 {
			out = append(out, fmt.Sprintf("%s drew Chance: +$%d.", pl.Token, delta))
		} else {
			out = append(out, fmt.Sprintf("%s drew Chance: -$%d.", pl.Token, -delta))
		}

	case kindProperty:
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
		}
	}
	return out
}

// nextSeat returns the next claimed (non-Spec) seat after `seat`, wrapping. If
// no other player has joined, it returns the same seat (solo play).
func (g *Game) nextSeat(store *ecs.Store, seat int) int {
	for i := 1; i <= maxSeats; i++ {
		cand := (seat + i) % maxSeats
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
func (g *Game) Roll(ctx context.Context, seat int) (rollResult, error) {
	return sim.Ask(ctx, g.sim, func(reply func(rollResult)) sim.Command {
		return rollCmd{game: g, seat: seat, reply: reply}
	})
}

// ---- helpers ----

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
