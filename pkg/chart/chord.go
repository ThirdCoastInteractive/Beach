package chart

import (
	"fmt"
	"math"
)

// --- Input types ------------------------------------------------------------

type ChordData struct {
	Groups []string    `json:"groups"`
	Matrix [][]float64 `json:"matrix"`
	Unit   string      `json:"unit,omitempty"`
}

// --- Output geometry --------------------------------------------------------

// Chord uses a 200x200 viewBox centered at (100,100).
type Chord struct {
	Arcs    []ChordArc
	Ribbons []ChordRibbon
}

type ChordArc struct {
	PathD       string
	Color       string
	Label       string
	LabelX      string
	LabelY      string
	LabelAnchor string
	Tip         string
	GroupIdx    int
}

type ChordRibbon struct {
	PathD    string
	Color    string
	Opacity  string
	Tip      string
	SrcGroup int
	DstGroup int
}

// --- Layout -----------------------------------------------------------------

func LayoutChord(data ChordData) Chord {
	n := len(data.Groups)
	if n == 0 || len(data.Matrix) != n {
		return Chord{}
	}

	// Row totals for arc sizing.
	totals := make([]float64, n)
	var grandTotal float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			totals[i] += data.Matrix[i][j]
		}
		grandTotal += totals[i]
	}
	if grandTotal == 0 {
		return Chord{}
	}

	const (
		cx       = 100.0
		cy       = 100.0
		outerR   = 44.0
		innerR   = 40.0
		padAngle = 0.04
		twoPi    = 2 * math.Pi
	)

	totalPad := padAngle * float64(n)
	availAngle := twoPi - totalPad

	// Compute arc start/end angles.
	type arcRange struct {
		start, end float64
	}
	arcs := make([]arcRange, n)
	angle := 0.0
	for i := 0; i < n; i++ {
		sweep := (totals[i] / grandTotal) * availAngle
		arcs[i] = arcRange{start: angle, end: angle + sweep}
		angle += sweep + padAngle
	}

	polarXY := func(r, a float64) (float64, float64) {
		return cx + r*math.Cos(a-math.Pi/2), cy + r*math.Sin(a-math.Pi/2)
	}

	arcPath := func(r float64, a0, a1 float64) string {
		x0, y0 := polarXY(r, a0)
		x1, y1 := polarXY(r, a1)
		large := 0
		if a1-a0 > math.Pi {
			large = 1
		}
		return fmt.Sprintf("M %.2f %.2f A %.2f %.2f 0 %d 1 %.2f %.2f", x0, y0, r, r, large, x1, y1)
	}

	out := Chord{}

	for i := 0; i < n; i++ {
		color := ColorVar(i)
		midAngle := (arcs[i].start + arcs[i].end) / 2

		outerPath := arcPath(outerR, arcs[i].start, arcs[i].end)
		ix0, iy0 := polarXY(innerR, arcs[i].end)
		innerPath := arcPath(innerR, arcs[i].end, arcs[i].start)
		ox0, oy0 := polarXY(outerR, arcs[i].start)

		d := outerPath +
			fmt.Sprintf(" L %.2f %.2f ", ix0, iy0) +
			reverseArc(innerPath) +
			fmt.Sprintf(" L %.2f %.2f Z", ox0, oy0)

		lx, ly := polarXY(outerR+6, midAngle)
		anchor := "middle"
		if lx < cx-5 {
			anchor = "end"
		} else if lx > cx+5 {
			anchor = "start"
		}

		tip := BuildTipHTML(color, data.Groups[i],
			[]TipRow{{Label: "total", Value: fmt.Sprintf("%.0f", totals[i]) + " " + data.Unit}})

		out.Arcs = append(out.Arcs, ChordArc{
			PathD:       d,
			Color:       color,
			Label:       data.Groups[i],
			LabelX:      fmt.Sprintf("%.2f", lx),
			LabelY:      fmt.Sprintf("%.2f", ly),
			LabelAnchor: anchor,
			Tip:         tip,
			GroupIdx:    i,
		})
	}

	// Ribbons -- one per non-zero matrix cell.
	offsets := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			v := data.Matrix[i][j]
			if v <= 0 {
				continue
			}
			srcSweep := (v / grandTotal) * availAngle
			srcA0 := arcs[i].start + offsets[i]
			srcA1 := srcA0 + srcSweep
			offsets[i] += srcSweep

			// Find the target sub-arc.
			var tgtOff float64
			for k := 0; k < i; k++ {
				tgtOff += (data.Matrix[k][j] / grandTotal) * availAngle
			}
			tgtSweep := (v / grandTotal) * availAngle
			tgtA0 := arcs[j].start + tgtOff
			tgtA1 := tgtA0 + tgtSweep

			sx0, sy0 := polarXY(innerR, srcA0)
			sx1, sy1 := polarXY(innerR, srcA1)
			tx0, ty0 := polarXY(innerR, tgtA0)
			tx1, ty1 := polarXY(innerR, tgtA1)

			d := fmt.Sprintf("M %.2f %.2f", sx0, sy0)
			srcLarge := 0
			if srcA1-srcA0 > math.Pi {
				srcLarge = 1
			}
			d += fmt.Sprintf(" A %.2f %.2f 0 %d 1 %.2f %.2f", innerR, innerR, srcLarge, sx1, sy1)
			d += fmt.Sprintf(" Q %.2f %.2f %.2f %.2f", cx, cy, tx0, ty0)
			tgtLarge := 0
			if tgtA1-tgtA0 > math.Pi {
				tgtLarge = 1
			}
			d += fmt.Sprintf(" A %.2f %.2f 0 %d 1 %.2f %.2f", innerR, innerR, tgtLarge, tx1, ty1)
			d += fmt.Sprintf(" Q %.2f %.2f %.2f %.2f Z", cx, cy, sx0, sy0)

			tip := BuildTipHTML("", data.Groups[i]+" → "+data.Groups[j],
				[]TipRow{{Label: "flow", Value: fmt.Sprintf("%.0f", v) + " " + data.Unit}})

			out.Ribbons = append(out.Ribbons, ChordRibbon{
				PathD:    d,
				Color:    ColorVar(i),
				Opacity:  "0.45",
				Tip:      tip,
				SrcGroup: i,
				DstGroup: j,
			})
		}
	}

	return out
}

// reverseArc flips the sweep-direction flag in a single SVG arc command.
func reverseArc(d string) string {
	out := make([]byte, len(d))
	copy(out, d)
	inA := false
	spaces := 0
	for i := 0; i < len(out); i++ {
		if out[i] == 'A' {
			inA = true
			spaces = 0
			continue
		}
		if inA && out[i] == ' ' {
			spaces++
			if spaces == 5 {
				if i+1 < len(out) && out[i+1] == '1' {
					out[i+1] = '0'
				} else if i+1 < len(out) && out[i+1] == '0' {
					out[i+1] = '1'
				}
				inA = false
			}
		}
	}
	return string(out)
}
