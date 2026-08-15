// BYOL (bring-your-own-layout): a post's studio composition and its pin on
// the shared wall. Coords are normalized 0–1 so the same layout paints at
// any size. Geometry is the only thing the client authors; delivery URLs
// are recomputed from Cloudflare Images ids at render time.
package content

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"strconv"

	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/store"
	"github.com/a-h/templ"
)

// Studio budget. The canvas is one square; pieces must fit mostly inside it.
const (
	MaxPieces = 8
	MinPiece  = 0.12
	MaxPiece  = 0.85
	MaxAbsRot = 180.0
)

// Wall-print budget: each post occupies a slice of the board's square-footage.
const (
	MinPinW   = 0.16
	MaxPinW   = 0.36
	MaxPinRot = 26.0
)

// Piece is one image on a studio canvas. X/Y are the top-left, W/H the size,
// all as a fraction of the square; Rot is degrees clockwise.
type Piece struct {
	URL  string  `json:"url"`
	CFID string  `json:"cf_id"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Rot  float64 `json:"rot"`
	Z    int     `json:"z"`
}

// Layout is a studio composition. An empty Pieces slice is not a layout post.
type Layout struct {
	Pieces []Piece `json:"pieces"`
}

// Pin is a print's place on the shared wall. X/Y are the center, W the width,
// all as a fraction of the board; Rot is degrees clockwise.
type Pin struct {
	X, Y, Rot, W float64
	Z            int
}

// ParseLayout decodes a hush_posts.layout blob. Empty or "{}" is a valid
// empty layout (text/image posts carry that).
func ParseLayout(raw []byte) (Layout, error) {
	if len(raw) == 0 {
		return Layout{}, nil
	}
	var l Layout
	if err := json.Unmarshal(raw, &l); err != nil {
		return Layout{}, err
	}
	return l, nil
}

// JSON encodes a layout for storage.
func (l Layout) JSON() []byte {
	b, _ := json.Marshal(l)
	return b
}

const pieceHang = 0.15

// Clamp pulls each piece back onto the canvas budget: size, rotation, and
// position. Publish uses this so a dragged print still posts instead of
// bouncing the whole composition.
func (l Layout) Clamp() Layout {
	for i := range l.Pieces {
		l.Pieces[i] = l.Pieces[i].clamp()
	}
	return l
}

func (p Piece) clamp() Piece {
	if p.W <= 0 || math.IsNaN(p.W) {
		p.W = MinPiece
	}
	if p.H <= 0 || math.IsNaN(p.H) {
		p.H = MinPiece
	}
	if p.W < MinPiece {
		p.W = MinPiece
	}
	if p.W > MaxPiece {
		p.W = MaxPiece
	}
	if p.H < MinPiece {
		p.H = MinPiece
	}
	if p.H > MaxPiece {
		p.H = MaxPiece
	}
	p.Rot = wrapDeg(p.Rot)
	if p.X < -pieceHang {
		p.X = -pieceHang
	}
	if p.Y < -pieceHang {
		p.Y = -pieceHang
	}
	if p.X+p.W > 1+pieceHang {
		p.X = 1 + pieceHang - p.W
	}
	if p.Y+p.H > 1+pieceHang {
		p.Y = 1 + pieceHang - p.H
	}
	return p
}

func wrapDeg(r float64) float64 {
	if math.IsNaN(r) {
		return 0
	}
	for r > 180 {
		r -= 360
	}
	for r < -180 {
		r += 360
	}
	return r
}

// Validate checks a studio composition is postable: 1–MaxPieces pieces, each
// with an id and geometry inside the canvas budget. Client-supplied URLs are
// ignored — the server fills those from the ids. Callers should Clamp first.
func (l Layout) Validate() error {
	n := len(l.Pieces)
	if n == 0 {
		return fmt.Errorf("add at least one image")
	}
	if n > MaxPieces {
		return fmt.Errorf("too many images — %d max", MaxPieces)
	}
	for i, p := range l.Pieces {
		if p.CFID == "" {
			return fmt.Errorf("piece %d: missing image", i+1)
		}
		if p.W < MinPiece || p.W > MaxPiece || p.H < MinPiece || p.H > MaxPiece {
			return fmt.Errorf("piece %d: off the square-footage", i+1)
		}
		if p.X < -pieceHang || p.Y < -pieceHang || p.X+p.W > 1+pieceHang || p.Y+p.H > 1+pieceHang {
			return fmt.Errorf("piece %d: drifted off the canvas", i+1)
		}
		if math.Abs(p.Rot) > MaxAbsRot {
			return fmt.Errorf("piece %d: too much spin", i+1)
		}
	}
	return nil
}

// CSS is a piece's inline style inside a collage (top-left origin).
func (p Piece) CSS() templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf(
		"left: %.2f%%; top: %.2f%%; width: %.2f%%; height: %.2f%%; transform: rotate(%.1fdeg); z-index: %d",
		p.X*100, p.Y*100, p.W*100, p.H*100, p.Rot, p.Z,
	))
}

// LayoutOf reads a post's composition; a broken blob is an empty layout.
func LayoutOf(p store.Post) Layout {
	l, err := ParseLayout(p.Layout)
	if err != nil {
		return Layout{}
	}
	return l
}

// PinOf is a post's wall placement: the saved pin, or a deterministic scatter
// from its id so unpinned posts still sit like prints on a table.
func PinOf(p store.Post) Pin {
	if p.HasPin {
		z := p.PinZ
		if z == 0 {
			z = int(p.ID)
		}
		return ClampPin(Pin{X: p.PinX, Y: p.PinY, Rot: p.PinRot, W: p.PinW, Z: z})
	}
	pin := DefaultPin(p.ID)
	pin.Z = int(p.ID)
	if p.PinZ != 0 {
		pin.Z = p.PinZ
	}
	return pin
}

// DefaultPin scatters a post by id: a golden-angle spiral around the board's
// center, slight rotation, width in the print budget. Same id → same place.
func DefaultPin(id int64) Pin {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strconv.FormatInt(id, 10)))
	n := h.Sum32()

	// golden angle (radians) — the sunflower packing
	const ga = 2.399963229728653
	i := float64(id)
	// radius grows slowly so early posts sit in the middle and later ones
	// fill the table without marching off the edge
	r := 0.10 + 0.34*math.Sqrt(math.Mod(i*0.6180339887, 1))
	ang := i * ga
	x := 0.50 + r*math.Cos(ang)
	y := 0.50 + r*math.Sin(ang)*0.88
	w := MinPinW + (MaxPinW-MinPinW)*float64(n%7)/6
	rot := -18 + float64(n%37)
	return ClampPin(Pin{X: x, Y: y, Rot: rot, W: w})
}

// ClampPin keeps a wall pin inside the square-footage.
func ClampPin(p Pin) Pin {
	if p.W < MinPinW {
		p.W = MinPinW
	}
	if p.W > MaxPinW {
		p.W = MaxPinW
	}
	if p.Rot > MaxPinRot {
		p.Rot = MaxPinRot
	}
	if p.Rot < -MaxPinRot {
		p.Rot = -MaxPinRot
	}
	// keep the print's center on the board with a small margin
	m := p.W/2 + 0.02
	if p.X < m {
		p.X = m
	}
	if p.X > 1-m {
		p.X = 1 - m
	}
	if p.Y < m {
		p.Y = m
	}
	if p.Y > 1-m {
		p.Y = 1 - m
	}
	return p
}

// PrintStyle is a wall print's inline placement: center origin, rotation
// via --rot so the board script can rewrite it without fighting the cascade.
func PrintStyle(p store.Post, pin Pin) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf(
		"left: %.2f%%; top: %.2f%%; width: %.2f%%; --rot: %.1fdeg; z-index: %d; view-transition-class: hush-post",
		pin.X*100, pin.Y*100, pin.W*100, pin.Rot, pin.Z,
	))
}
