package chart

import (
	"io"
)

// StreamgraphInput configures a stacked, center-baselined area (streamgraph).
type StreamgraphInput struct {
	Title  string
	Series []Datum2 // each spans the same number of time steps
	Width  float64  // defaults to 560
	Height float64  // defaults to 280
}

// StreamgraphLayout is the geometry for a streamgraph.
type StreamgraphLayout struct {
	W, H  float64
	Title string
	Bands []streamBand
}

type streamBand struct {
	Name  string
	Color string
	D     string // closed path
}

// LayoutStreamgraph computes geometry for a streamgraph using a centered
// (wiggle-free, "silhouette") baseline: each step's stack is centered on H/2.
func LayoutStreamgraph(in StreamgraphInput) StreamgraphLayout {
	w := in.Width
	if w == 0 {
		w = 560
	}
	h := in.Height
	if h == 0 {
		h = 280
	}
	lay := StreamgraphLayout{W: w, H: h, Title: in.Title}
	nS := len(in.Series)
	if nS == 0 {
		return lay
	}
	steps := 0
	for _, s := range in.Series {
		if len(s.Values) > steps {
			steps = len(s.Values)
		}
	}
	if steps < 2 {
		return lay
	}
	val := func(si, t int) float64 {
		if t < len(in.Series[si].Values) {
			return in.Series[si].Values[t]
		}
		return 0
	}
	// Per-step total and max-total for vertical scaling.
	totals := make([]float64, steps)
	maxTotal := 0.0
	for t := 0; t < steps; t++ {
		for si := 0; si < nS; si++ {
			totals[t] += val(si, t)
		}
		if totals[t] > maxTotal {
			maxTotal = totals[t]
		}
	}
	if maxTotal <= 0 {
		maxTotal = 1
	}
	pad := 12.0
	plotH := h - 2*pad
	stepX := func(t int) float64 { return float64(t) / float64(steps-1) * w }
	sy := func(v float64) float64 { return pad + (1-v/maxTotal)*plotH }

	// baseline[t] = bottom (in value units) of current band, centered.
	baseline := make([]float64, steps)
	for t := 0; t < steps; t++ {
		baseline[t] = (maxTotal - totals[t]) / 2
	}
	for si := 0; si < nS; si++ {
		var top, bot [][2]float64
		for t := 0; t < steps; t++ {
			v := val(si, t)
			y0 := baseline[t]
			y1 := baseline[t] + v
			x := stepX(t)
			bot = append(bot, [2]float64{x, sy(y0)})
			top = append(top, [2]float64{x, sy(y1)})
			baseline[t] = y1
		}
		// path: top forward, bottom backward
		pts := make([][2]float64, 0, len(top)+len(bot))
		pts = append(pts, top...)
		for i := len(bot) - 1; i >= 0; i-- {
			pts = append(pts, bot[i])
		}
		lay.Bands = append(lay.Bands, streamBand{
			Name:  in.Series[si].Name,
			Color: QualitativeVar(si),
			D:     polyPath(pts, true),
		})
	}
	return lay
}

// StreamgraphSVG renders the streamgraph described by lay.
func StreamgraphSVG(w io.Writer, lay StreamgraphLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, b := range lay.Bands {
		s.p(`<g>`)
		if b.Name != "" {
			s.title(b.Name)
		}
		s.path(b.D, b.Color, "", `fill-opacity="0.85"`)
		s.p(`</g>`)
	}
	s.close()
	return s.Err()
}
