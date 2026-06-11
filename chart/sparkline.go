package chart

import (
	"io"
	"math"
)

// SparkInput configures a compact, axis-free sparkline.
type SparkInput struct {
	Values []float64
	Title  string
	Width  float64 // defaults to 120
	Height float64 // defaults to 32
	Area   bool    // fill under the line
}

// SparkLayout is the computed geometry for a sparkline.
type SparkLayout struct {
	W, H   float64
	Title  string
	Area   bool
	Color  string
	Pts    [][2]float64
	LastPt [2]float64
	HasPts bool
}

// LayoutSparkline computes geometry for a sparkline.
func LayoutSparkline(in SparkInput) SparkLayout {
	w := in.Width
	if w == 0 {
		w = 120
	}
	h := in.Height
	if h == 0 {
		h = 32
	}
	lay := SparkLayout{W: w, H: h, Title: in.Title, Area: in.Area, Color: QualitativeVar(0)}
	if len(in.Values) == 0 {
		return lay
	}
	pad := 2.0
	min, max := math.Inf(1), math.Inf(-1)
	for _, v := range in.Values {
		min = math.Min(min, v)
		max = math.Max(max, v)
	}
	if max <= min {
		max = min + 1
	}
	n := len(in.Values)
	for i, v := range in.Values {
		var x float64
		if n == 1 {
			x = w / 2
		} else {
			x = pad + float64(i)/float64(n-1)*(w-2*pad)
		}
		y := (h - pad) - (v-min)/(max-min)*(h-2*pad)
		lay.Pts = append(lay.Pts, [2]float64{x, y})
	}
	lay.LastPt = lay.Pts[len(lay.Pts)-1]
	lay.HasPts = true
	return lay
}

// SparklineSVG renders the sparkline described by lay.
func SparklineSVG(w io.Writer, lay SparkLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	if lay.HasPts {
		if lay.Area {
			area := make([][2]float64, 0, len(lay.Pts)+2)
			area = append(area, [2]float64{lay.Pts[0][0], lay.H})
			area = append(area, lay.Pts...)
			area = append(area, [2]float64{lay.Pts[len(lay.Pts)-1][0], lay.H})
			s.p(`<path d="`, polyPath(area, true), `" fill="`, lay.Color,
				`" fill-opacity="0.15" stroke="none"/>`)
		}
		s.p(`<path d="`, polyPath(lay.Pts, false),
			`" fill="none" stroke="`, lay.Color,
			`" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke"/>`)
		s.circle(lay.LastPt[0], lay.LastPt[1], 2, lay.Color, "")
	}
	s.close()
	return s.Err()
}
