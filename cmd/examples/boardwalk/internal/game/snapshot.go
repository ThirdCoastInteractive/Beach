package game

import (
	"github.com/ThirdCoastInteractive/Beach/pkg/ecs"
)

// Snapshot is a read-only copy of everything the board surface renders, built
// on the sim loop so the renderer never touches the live store off-loop. The
// web package shapes its views from this value.
type Snapshot struct {
	Board   Board
	Players []PlayerView
	Owners  map[int]string // tile index -> owner token ("" = bank)
}

// PlayerView is one seat's renderable state inside a Snapshot.
type PlayerView struct {
	Seat   int
	Name   string
	Token  string
	Square int
	Cash   int
	Spec   bool
}

// snapshotBoard reads the whole live slice into a plain value. Loop-goroutine
// only (called from a projection View or an Ask).
func snapshotBoard(store *ecs.Store, seats []ecs.Entity, deeds map[int]ecs.Entity, gameEnt ecs.Entity) Snapshot {
	b, _ := ecs.Get[Board](store, gameEnt)
	snap := Snapshot{Board: b, Owners: make(map[int]string)}
	for i, e := range seats {
		p, _ := ecs.Get[Player](store, e)
		pos, _ := ecs.Get[Position](store, e)
		cash, _ := ecs.Get[Cash](store, e)
		snap.Players = append(snap.Players, PlayerView{
			Seat: i, Name: p.Name, Token: p.Token,
			Square: pos.Square, Cash: cash.Amount, Spec: p.Spec,
		})
	}
	for tile, de := range deeds {
		own, _ := ecs.Get[Ownership](store, de)
		if own.Owner != 0 {
			op, _ := ecs.Get[Player](store, own.Owner)
			snap.Owners[tile] = op.Token
		}
	}
	return snap
}
