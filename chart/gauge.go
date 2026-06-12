package chart

import (
	"io"
	"math"
)

// GaugeInput configures a single-value radial gauge (180-degree arc).
type GaugeInput struct {
	Title string
	Label string
	Value float64
	Min   float64
	Max   float64 // must be > Min; defaults to 100 if zero
	Unit  string
	Size  float64 // diameter; defaults to 200
}

// GaugeLayout is the geometry for a single gauge.
type GaugeLayout struct {
	W, H      float64
	Title     string
	Cx, Cy, R float64
	TrackD    string
	ValueD    string
	Color     string
	ValTxt    string
	Label     string
}

// LayoutGauge computes geometry for a single-value semicircular gauge.
func LayoutGauge(in GaugeInput) GaugeLayout {
	size := in.Size
	if size == 0 {
		size = 200
	}
	max := in.Max
	if max == 0 {
		max = 100
	}
	if max <= in.Min {
		max = in.Min + 1
	}
	r := size/2 - 12
	cx := size / 2
	cy := size/2 + r/3 // shift down so the half-arc is centered
	frac := (in.Value - in.Min) / (max - in.Min)
	frac = math.Max(0, math.Min(1, frac))

	lay := GaugeLayout{
		W: size, H: cy + 16, Title: in.Title, Cx: cx, Cy: cy, R: r,
		Color: QualitativeVar(0), Label: in.Label,
		ValTxt: fnum(in.Value) + in.Unit,
		TrackD: arcPath(cx, cy, r, math.Pi, 0),
		ValueD: arcPath(cx, cy, r, math.Pi, math.Pi-frac*math.Pi),
	}
	return lay
}

// arcPath returns an SVG arc path from angle a0 to a1 (radians, standard math
// orientation) on a circle centered at (cx,cy) radius r. Angles increase
// counter-clockwise; SVG y grows downward, handled here.
func arcPath(cx, cy, r, a0, a1 float64) string {
	x0 := cx + r*math.Cos(a0)
	y0 := cy - r*math.Sin(a0)
	x1 := cx + r*math.Cos(a1)
	y1 := cy - r*math.Sin(a1)
	large := 0
	if math.Abs(a1-a0) > math.Pi {
		large = 1
	}
	// Points are placed with y = cy - r·sin(a), so increasing the math angle a
	// traces counter-clockwise on screen (SVG's y grows downward) — sweep-flag 0.
	// Decreasing a therefore needs sweep-flag 1 to bow the arc the intended way;
	// a gauge sweeps a0=π → a1=0, which must dome upward (∩), not smile (∪).
	sweep := 1
	if a1 > a0 {
		sweep = 0
	}
	return "M" + fnum(x0) + "," + fnum(y0) +
		" A" + fnum(r) + "," + fnum(r) + " 0 " +
		itoa(large) + " " + itoa(sweep) + " " +
		fnum(x1) + "," + fnum(y1)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	return "1"
}

// GaugeSVG renders the single gauge described by lay.
func GaugeSVG(w io.Writer, lay GaugeLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	s.p(`<g>`)
	if lay.Label != "" {
		s.title(lay.Label + ": " + lay.ValTxt)
	}
	s.path(lay.TrackD, "none", "var(--chart-grid, currentColor)",
		`stroke-width="12" stroke-opacity="0.2" stroke-linecap="round" vector-effect="non-scaling-stroke"`)
	s.path(lay.ValueD, "none", lay.Color,
		`stroke-width="12" stroke-linecap="round" vector-effect="non-scaling-stroke"`)
	s.p(`</g>`)
	s.text(lay.Cx, lay.Cy, "middle", "chart-gauge-value", lay.ValTxt)
	if lay.Label != "" {
		s.text(lay.Cx, lay.Cy+16, "middle", "chart-label", lay.Label)
	}
	s.close()
	return s.Err()
}

// StackedGaugeInput configures a concentric (stacked-ring) gauge.
type StackedGaugeInput struct {
	Title  string
	Series []Datum // each becomes a ring; Value is a fraction of Max
	Max    float64 // 0 = 100
	Size   float64 // defaults to 200
}

// StackedGaugeLayout is the geometry for a concentric ring gauge.
type StackedGaugeLayout struct {
	W, H  float64
	Title string
	Rings []gaugeRing
}

type gaugeRing struct {
	TrackD string
	ValueD string
	Color  string
	Label  string
	Value  string
}

// LayoutStackedGauge computes geometry for concentric ring gauges.
func LayoutStackedGauge(in StackedGaugeInput) StackedGaugeLayout {
	size := in.Size
	if size == 0 {
		size = 200
	}
	max := in.Max
	if max == 0 {
		max = 100
	}
	cx, cy := size/2, size/2
	outer := size/2 - 10
	gap := 16.0
	lay := StackedGaugeLayout{W: size, H: size, Title: in.Title}
	for i, d := range in.Series {
		r := outer - float64(i)*gap
		if r < 4 {
			r = 4
		}
		frac := d.Value / max
		frac = math.Max(0, math.Min(1, frac))
		// Full ring track (circle approximated by two arcs).
		lay.Rings = append(lay.Rings, gaugeRing{
			TrackD: arcPath(cx, cy, r, math.Pi/2, math.Pi/2-2*math.Pi+0.0001),
			ValueD: arcPath(cx, cy, r, math.Pi/2, math.Pi/2-frac*2*math.Pi),
			Color:  QualitativeVar(i),
			Label:  d.Label,
			Value:  fnum(d.Value),
		})
	}
	return lay
}

// StackedGaugeSVG renders the concentric ring gauge described by lay.
func StackedGaugeSVG(w io.Writer, lay StackedGaugeLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, ring := range lay.Rings {
		s.p(`<g>`)
		s.title(ring.Label + ": " + ring.Value)
		s.path(ring.TrackD, "none", "var(--chart-grid, currentColor)",
			`stroke-width="10" stroke-opacity="0.2" vector-effect="non-scaling-stroke"`)
		s.path(ring.ValueD, "none", ring.Color,
			`stroke-width="10" stroke-linecap="round" vector-effect="non-scaling-stroke"`)
		s.p(`</g>`)
	}
	s.close()
	return s.Err()
}
