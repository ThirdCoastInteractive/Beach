package game

import (
	"testing"

	"github.com/ThirdCoastInteractive/Beach/pkg/ecs"
	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
	"github.com/ThirdCoastInteractive/Beach/pkg/sim"
)

func TestSnapshotRestoreKeepsSeatsAndCash(t *testing.T) {
	RegisterComponents()
	s := sim.New(sim.Config{Hub: hub.New(), Store: ecs.New()})
	g := NewGame(s, nil, Renderers{})
	st := s.Store()
	if len(g.seats) != MaxSeats {
		t.Fatalf("seats = %d, want %d", len(g.seats), MaxSeats)
	}
	cash, ok := ecs.Get[Cash](st, g.seats[0])
	if !ok || cash.Amount != StartingCash {
		t.Fatalf("seat 0 cash = %+v ok=%v, want %d", cash, ok, StartingCash)
	}

	blob, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ecs.Load(blob)
	if err != nil {
		t.Fatal(err)
	}
	g2 := NewGame(sim.New(sim.Config{Hub: hub.New(), Store: restored}), nil, Renderers{})
	if len(g2.seats) != MaxSeats {
		t.Fatalf("restored seats = %d, want %d", len(g2.seats), MaxSeats)
	}
	cash2, ok := ecs.Get[Cash](restored, g2.seats[0])
	if !ok || cash2.Amount != StartingCash {
		t.Fatalf("restored seat 0 cash = %+v ok=%v, want %d", cash2, ok, StartingCash)
	}
	if g2.seats[0] != g.seats[0] {
		t.Fatalf("seat 0 handle changed across restore: %v vs %v", g2.seats[0], g.seats[0])
	}
}
