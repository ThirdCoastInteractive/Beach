package game

import "github.com/ThirdCoastInteractive/Beach/pkg/ecs"

// The ECS components that make up the live game slice. Everything the board
// renders from lives here in the sim's ecs.Store, mutated only on the sim loop
// goroutine inside a Command's Apply. None of it requires a database to play.

// Player marks an entity as a player and carries its identity for rendering and
// per-player (user:<id>) projections. Token is the board piece glyph.
type Player struct {
	Name  string
	Token string // emoji/glyph piece
	Spec  bool   // a seat that has not been claimed yet (open seat)
}

// Position is the player's square index on the board [0,len(Tiles)).
type Position struct {
	Square int
}

// Cash is the player's bank balance in dollars. Money is the audited lane: in a
// persisted deployment this would mirror to the append-only ledger; in the
// in-memory demo it lives only in the store.
type Cash struct {
	Amount int
}

// TurnOrder gives each player a fixed seat index; the player whose Seat equals
// the game's current turn pointer is the one allowed to roll.
type TurnOrder struct {
	Seat int
}

// Ownership lives on a *property tile* entity (not a player): which player owns
// it, or the zero Entity when the bank still holds it. Tile carries the static
// board metadata so a single Changed[Ownership] projection can render a deed.
type Ownership struct {
	Tile  int        // board square index this deed is for
	Owner ecs.Entity // zero value == owned by the bank
}
