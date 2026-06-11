package chart

import (
	"io"
	"math"
)

// ChordInput configures a chord diagram from a square relationship matrix.
// Matrix[i][j] is the weight from group i to group j.
type ChordInput struct {
	Title  string
	Labels []string
	Matrix [][]float64
	Size   float64 // diameter; defaults to 360
}

// ChordLayout is the geometry for a chord diagram.
type ChordLayout struct {
	W, H    float64
	Title   string
	Arcs    []chordArc
	Ribbons []chordRibbon
}

type chordArc struct {
	D      string
	Color  string
	Label  string
	LblX   float64
	LblY   float64
	Anchor string
}

type chordRibbon struct {
	D     string
	Color string
	Tip   string
}

// LayoutChord computes geometry for a chord diagram. Each group gets an arc on
// the circle sized by its total weight; ribbons connect related groups.
func LayoutChord(in ChordInput) ChordLayout {
	size := in.Size
	if size == 0 {
		size = 360
	}
	lay := ChordLayout{W: size, H: size, Title: in.Title}
	n := len(in.Labels)
	if n == 0 {
		return lay
	}
	cx, cy := size/2, size/2
	rOuter := size/2 - 40
	rInner := rOuter - 12

	// Row totals (out + in, symmetric-ish) for arc sizing.
	totals := make([]float64, n)
	grand := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i < len(in.Matrix) && j < len(in.Matrix[i]) {
				totals[i] += in.Matrix[i][j]
			}
		}
		grand += totals[i]
	}
	if grand <= 0 {
		grand = 1
	}
	gap := 0.03 * 2 * math.Pi
	avail := 2*math.Pi - gap*float64(n)
	// arc angle ranges
	start := make([]float64, n)
	end := make([]float64, n)
	ang := -math.Pi / 2
	for i := 0; i < n; i++ {
		start[i] = ang
		sweep := totals[i] / grand * avail
		end[i] = ang + sweep
		ang = end[i] + gap
	}
	pt := func(r, a float64) (float64, float64) {
		return cx + r*math.Cos(a), cy + r*math.Sin(a)
	}
	for i := 0; i < n; i++ {
		x0, y0 := pt(rOuter, start[i])
		x1, y1 := pt(rOuter, end[i])
		xi1, yi1 := pt(rInner, end[i])
		xi0, yi0 := pt(rInner, start[i])
		large := 0
		if end[i]-start[i] > math.Pi {
			large = 1
		}
		d := "M" + fnum(x0) + "," + fnum(y0) +
			" A" + fnum(rOuter) + "," + fnum(rOuter) + " 0 " + itoa(large) + " 1 " + fnum(x1) + "," + fnum(y1) +
			" L" + fnum(xi1) + "," + fnum(yi1) +
			" A" + fnum(rInner) + "," + fnum(rInner) + " 0 " + itoa(large) + " 0 " + fnum(xi0) + "," + fnum(yi0) + " Z"
		mid := (start[i] + end[i]) / 2
		lx, ly := pt(rOuter+12, mid)
		anchor := "start"
		if math.Cos(mid) < 0 {
			anchor = "end"
		}
		lay.Arcs = append(lay.Arcs, chordArc{
			D: d, Color: QualitativeVar(i), Label: in.Labels[i],
			LblX: lx, LblY: ly + 3, Anchor: anchor,
		})
	}
	// Ribbons: for each i<j with weight, a quadratic-ish band through center.
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			var wgt float64
			if i < len(in.Matrix) && j < len(in.Matrix[i]) {
				wgt += in.Matrix[i][j]
			}
			if i != j && j < len(in.Matrix) && i < len(in.Matrix[j]) {
				wgt += in.Matrix[j][i]
			}
			if wgt <= 0 {
				continue
			}
			ai := (start[i] + end[i]) / 2
			aj := (start[j] + end[j]) / 2
			xi, yi := pt(rInner, ai)
			xj, yj := pt(rInner, aj)
			d := "M" + fnum(xi) + "," + fnum(yi) +
				" Q" + fnum(cx) + "," + fnum(cy) + " " + fnum(xj) + "," + fnum(yj)
			lay.Ribbons = append(lay.Ribbons, chordRibbon{
				D: d, Color: QualitativeVar(i),
				Tip: in.Labels[i] + " ↔ " + in.Labels[j] + ": " + fnum(wgt),
			})
		}
	}
	return lay
}

// ChordSVG renders the chord diagram described by lay.
func ChordSVG(w io.Writer, lay ChordLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, r := range lay.Ribbons {
		s.p(`<g>`)
		s.title(r.Tip)
		s.path(r.D, "none", r.Color,
			`stroke-width="2" stroke-opacity="0.4" vector-effect="non-scaling-stroke"`)
		s.p(`</g>`)
	}
	for _, a := range lay.Arcs {
		s.p(`<g>`)
		s.title(a.Label)
		s.path(a.D, a.Color, "", "")
		s.p(`</g>`)
		s.text(a.LblX, a.LblY, a.Anchor, "chart-label", a.Label)
	}
	s.close()
	return s.Err()
}
