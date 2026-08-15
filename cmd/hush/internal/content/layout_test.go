package content

import (
	"math"
	"testing"

	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/store"
)

func TestLayoutValidate(t *testing.T) {
	ok := Layout{Pieces: []Piece{{CFID: "abc", X: 0.1, Y: 0.1, W: 0.4, H: 0.5, Rot: -8}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid layout: %v", err)
	}
	if err := (Layout{}).Validate(); err == nil {
		t.Fatal("empty layout should fail")
	}
	tooBig := Layout{Pieces: []Piece{{CFID: "abc", X: 0, Y: 0, W: 0.99, H: 0.99}}}
	if err := tooBig.Validate(); err == nil {
		t.Fatal("oversized piece should fail")
	}
	noID := Layout{Pieces: []Piece{{X: 0.2, Y: 0.2, W: 0.3, H: 0.3}}}
	if err := noID.Validate(); err == nil {
		t.Fatal("missing cf_id should fail")
	}
}

func TestDefaultPinStableAndClamped(t *testing.T) {
	a, b := DefaultPin(7), DefaultPin(7)
	if a != b {
		t.Fatalf("same id must pin the same: %+v vs %+v", a, b)
	}
	for id := int64(1); id <= 40; id++ {
		p := DefaultPin(id)
		if p.W < MinPinW || p.W > MaxPinW {
			t.Fatalf("id %d width %.3f out of budget", id, p.W)
		}
		if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
			t.Fatalf("id %d off the board: %+v", id, p)
		}
		if math.Abs(p.Rot) > MaxPinRot {
			t.Fatalf("id %d rot %.1f", id, p.Rot)
		}
	}
}

func TestPinOfUsesSaved(t *testing.T) {
	p := store.Post{ID: 3, HasPin: true, PinX: 0.4, PinY: 0.5, PinRot: -6, PinW: 0.22}
	got := PinOf(p)
	if math.Abs(got.X-0.4) > 1e-9 || math.Abs(got.W-0.22) > 1e-9 {
		t.Fatalf("saved pin lost: %+v", got)
	}
	// unpinned falls back to the id scatter, not zeros
	scattered := PinOf(store.Post{ID: 3})
	if scattered.W == 0 {
		t.Fatal("unpinned post should scatter")
	}
}
