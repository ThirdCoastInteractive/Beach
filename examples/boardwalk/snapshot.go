package main

import (
	"bytes"
	"context"

	"github.com/ThirdCoastInteractive/the-beach/pkg/ecs"
	"github.com/a-h/templ"
)

// boardSnapshot is a read-only copy of everything the board surface renders,
// built on the sim loop so the renderer never touches the live store off-loop.
type boardSnapshot struct {
	board   Board
	players []playerView
	owners  map[int]string // tile index -> owner token ("" = bank)
}

type playerView struct {
	Seat   int
	Name   string
	Token  string
	Square int
	Cash   int
	Spec   bool
}

// snapshotBoard reads the whole live slice into a plain value. Loop-goroutine
// only (called from a projection View).
func snapshotBoard(store *ecs.Store, seats []ecs.Entity, deeds map[int]ecs.Entity, gameEnt ecs.Entity) boardSnapshot {
	b, _ := ecs.Get[Board](store, gameEnt)
	snap := boardSnapshot{board: b, owners: make(map[int]string)}
	for i, e := range seats {
		p, _ := ecs.Get[Player](store, e)
		pos, _ := ecs.Get[Position](store, e)
		cash, _ := ecs.Get[Cash](store, e)
		snap.players = append(snap.players, playerView{
			Seat: i, Name: p.Name, Token: p.Token,
			Square: pos.Square, Cash: cash.Amount, Spec: p.Spec,
		})
	}
	for tile, de := range deeds {
		own, _ := ecs.Get[Ownership](store, de)
		if own.Owner != 0 {
			op, _ := ecs.Get[Player](store, own.Owner)
			snap.owners[tile] = op.Token
		}
	}
	return snap
}

// renderToBytes renders a templ component to bytes for a hub.Event / projection.
func renderToBytes(c templ.Component) []byte {
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return nil
	}
	return buf.Bytes()
}
