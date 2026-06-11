package chart

import (
	"io"
)

// TrendDir is the direction of a billboard KPI's change.
type TrendDir int

const (
	TrendFlat TrendDir = iota
	TrendUp
	TrendDown
)

// BillboardInput configures a single-KPI "billboard" tile.
type BillboardInput struct {
	Title  string
	Label  string // metric name, e.g. "Active users"
	Value  string // pre-formatted big number, e.g. "12,402"
	Delta  string // pre-formatted change, e.g. "+4.2%"
	Trend  TrendDir
	Width  float64 // defaults to 220
	Height float64 // defaults to 120
}

// BillboardLayout is the geometry for a billboard KPI tile.
type BillboardLayout struct {
	W, H     float64
	Title    string
	Label    string
	Value    string
	Delta    string
	Trend    TrendDir
	Arrow    string // glyph reinforcing trend (not color-only)
	DeltaVar string // token color name reflecting good/bad/neutral
}

// LayoutBillboard computes geometry for a billboard KPI tile. Trend is encoded
// with both an arrow glyph and a semantic color token, never color alone.
func LayoutBillboard(in BillboardInput) BillboardLayout {
	w := in.Width
	if w == 0 {
		w = 220
	}
	h := in.Height
	if h == 0 {
		h = 120
	}
	lay := BillboardLayout{
		W: w, H: h, Title: in.Title, Label: in.Label,
		Value: in.Value, Delta: in.Delta, Trend: in.Trend,
	}
	switch in.Trend {
	case TrendUp:
		lay.Arrow = "↑" // up arrow
		lay.DeltaVar = "var(--success, currentColor)"
	case TrendDown:
		lay.Arrow = "↓" // down arrow
		lay.DeltaVar = "var(--error, currentColor)"
	default:
		lay.Arrow = "→" // right arrow
		lay.DeltaVar = "currentColor"
	}
	return lay
}

// BillboardSVG renders the billboard KPI described by lay.
func BillboardSVG(w io.Writer, lay BillboardLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	if lay.Label != "" {
		s.text(16, 28, "start", "chart-kpi-label", lay.Label)
	}
	s.text(16, 72, "start", "chart-kpi-value", lay.Value)
	if lay.Delta != "" {
		s.p(`<text x="16" y="100" text-anchor="start" class="chart-kpi-delta" fill="`,
			lay.DeltaVar, `">`, esc(lay.Arrow+" "+lay.Delta), `</text>`)
	}
	s.close()
	return s.Err()
}
