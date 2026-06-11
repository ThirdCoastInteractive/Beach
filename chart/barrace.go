package chart

import (
	"io"
	"sort"
)

// BarRaceInput configures one frame of a bar race. Each frame is a normal SSR
// render; animation comes from the server patching successive frames over SSE
// (the bars carry stable ids so the client CSS-transitions between states).
type BarRaceInput struct {
	Title string
	Bars  []Datum
	Frame int     // frame index, used only to namespace nothing — informational
	Max   float64 // 0 = auto from this frame
	Top   int     // keep only the top N bars; 0 = all
	Width float64 // defaults to 480
	RowH  float64 // defaults to 32
}

// BarRaceLayout is the geometry for one bar-race frame.
type BarRaceLayout struct {
	W, H  float64
	Title string
	Bars  []raceBar
}

type raceBar struct {
	ID    string // stable per-label id for SSE patching
	Box   Box
	Color string
	Label string
	Value string
	ValX  float64
	LblY  float64
}

// LayoutBarRace computes geometry for one frame of a bar race. Bars are sorted
// descending by value and assigned a stable, label-derived element id so a
// client can transition rectangles as the server patches new frames.
func LayoutBarRace(in BarRaceInput) BarRaceLayout {
	w := in.Width
	if w == 0 {
		w = 480
	}
	rowH := in.RowH
	if rowH == 0 {
		rowH = 32
	}
	bars := make([]Datum, len(in.Bars))
	copy(bars, in.Bars)
	sort.SliceStable(bars, func(i, j int) bool { return bars[i].Value > bars[j].Value })
	if in.Top > 0 && len(bars) > in.Top {
		bars = bars[:in.Top]
	}
	max := in.Max
	if max <= 0 {
		for _, d := range bars {
			if d.Value > max {
				max = d.Value
			}
		}
	}
	if max <= 0 {
		max = 1
	}
	labelW := 96.0
	trackX := labelW
	trackW := w - labelW - 56
	h := rowH*float64(len(bars)) + 8

	lay := BarRaceLayout{W: w, H: h, Title: in.Title}
	for rank, d := range bars {
		y := 4 + float64(rank)*rowH
		bw := d.Value / max * trackW
		if bw < 0 {
			bw = 0
		}
		lay.Bars = append(lay.Bars, raceBar{
			ID:    "race-" + slug(d.Label),
			Box:   Box{X: trackX, Y: y + 4, W: bw, H: rowH - 12},
			Color: QualitativeVar(rank),
			Label: d.Label,
			Value: fnum(d.Value),
			ValX:  trackX + bw + 6,
			LblY:  y + rowH/2 + 0,
		})
	}
	return lay
}

// slug produces a stable id fragment from a label.
func slug(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r == ' ' || r == '-' || r == '_':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "x"
	}
	return string(out)
}

// BarRaceSVG renders one frame of the bar race described by lay. Each bar group
// carries a stable id so the server can patch it over SSE; CSS transitions on
// the client tween the geometry between frames.
func BarRaceSVG(w io.Writer, lay BarRaceLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, b := range lay.Bars {
		s.p(`<g id="`, b.ID, `">`)
		s.title(b.Label + ": " + b.Value)
		s.text(b.Box.X-8, b.LblY+4, "end", "chart-label", b.Label)
		s.rect(b.Box, b.Color, `rx="2"`)
		s.text(b.ValX, b.LblY+4, "start", "chart-value", b.Value)
		s.p(`</g>`)
	}
	s.close()
	return s.Err()
}
