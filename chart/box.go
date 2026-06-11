package chart

import (
	"io"
	"math"
)

// BoxStats is the five-number summary for one box in a box plot. Callers
// compute the quantiles (the package does not assume raw samples).
type BoxStats struct {
	Label                    string
	Min, Q1, Median, Q3, Max float64
}

// BoxPlotInput configures a box-and-whisker plot.
type BoxPlotInput struct {
	Title  string
	Boxes  []BoxStats
	Width  float64 // defaults to 480
	Height float64 // defaults to 320
}

// BoxPlotLayout is the geometry for a box plot.
type BoxPlotLayout struct {
	W, H   float64
	Title  string
	Plot   Box
	Items  []boxGeom
	YTicks []axisTick
}

type boxGeom struct {
	Label                      string
	Color                      string
	CX, BoxW                   float64
	YMin, YQ1, YMed, YQ3, YMax float64
	Stats                      BoxStats
}

// LayoutBoxPlot computes geometry for a box plot.
func LayoutBoxPlot(in BoxPlotInput) BoxPlotLayout {
	w := in.Width
	if w == 0 {
		w = 480
	}
	h := in.Height
	if h == 0 {
		h = 320
	}
	plot := Box{X: 48, Y: 16, W: w - 64, H: h - 40}
	lay := BoxPlotLayout{W: w, H: h, Title: in.Title, Plot: plot}
	n := len(in.Boxes)
	if n == 0 {
		return lay
	}
	min, max := math.Inf(1), math.Inf(-1)
	for _, b := range in.Boxes {
		min = math.Min(min, b.Min)
		max = math.Max(max, b.Max)
	}
	if max <= min {
		max = min + 1
	}
	sy := func(v float64) float64 { return plot.Bottom() - (v-min)/(max-min)*plot.H }
	slot := plot.W / float64(n)
	bw := math.Min(slot*0.5, 48)
	for i, b := range in.Boxes {
		cx := plot.X + slot*float64(i) + slot/2
		lay.Items = append(lay.Items, boxGeom{
			Label: b.Label, Color: QualitativeVar(i),
			CX: cx, BoxW: bw,
			YMin: sy(b.Min), YQ1: sy(b.Q1), YMed: sy(b.Median),
			YQ3: sy(b.Q3), YMax: sy(b.Max), Stats: b,
		})
	}
	for _, t := range niceTicks(min, max, 5) {
		lay.YTicks = append(lay.YTicks, axisTick{Pos: sy(t), Label: fnum(t)})
	}
	return lay
}

// BoxPlotSVG renders the box plot described by lay.
func BoxPlotSVG(w io.Writer, lay BoxPlotLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, t := range lay.YTicks {
		s.line(lay.Plot.X, t.Pos, lay.Plot.Right(), t.Pos,
			"var(--chart-grid, currentColor)",
			`stroke-width="1" stroke-opacity="0.15" vector-effect="non-scaling-stroke"`)
		s.text(lay.Plot.X-6, t.Pos+3, "end", "chart-axis", t.Label)
	}
	for _, g := range lay.Items {
		half := g.BoxW / 2
		s.p(`<g>`)
		s.title(g.Label)
		// whiskers
		s.line(g.CX, g.YMax, g.CX, g.YQ3, "currentColor",
			`stroke-width="1" vector-effect="non-scaling-stroke"`)
		s.line(g.CX, g.YQ1, g.CX, g.YMin, "currentColor",
			`stroke-width="1" vector-effect="non-scaling-stroke"`)
		s.line(g.CX-half/2, g.YMax, g.CX+half/2, g.YMax, "currentColor",
			`stroke-width="1" vector-effect="non-scaling-stroke"`)
		s.line(g.CX-half/2, g.YMin, g.CX+half/2, g.YMin, "currentColor",
			`stroke-width="1" vector-effect="non-scaling-stroke"`)
		// box (Q1..Q3)
		s.rect(Box{X: g.CX - half, Y: g.YQ3, W: g.BoxW, H: g.YQ1 - g.YQ3},
			g.Color, `fill-opacity="0.4" stroke="`+g.Color+`" stroke-width="1" vector-effect="non-scaling-stroke"`)
		// median
		s.line(g.CX-half, g.YMed, g.CX+half, g.YMed, g.Color,
			`stroke-width="2" vector-effect="non-scaling-stroke"`)
		s.p(`</g>`)
		s.text(g.CX, lay.Plot.Bottom()+16, "middle", "chart-axis", g.Label)
	}
	s.close()
	return s.Err()
}
