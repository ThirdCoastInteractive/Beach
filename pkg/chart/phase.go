package chart

import (
	"fmt"
	"math"
)

// A phase diagram plots observations on a 2-D field that is itself divided into
// colored zones by a scalar function of the two axes — the canonical example is
// a psychrometric / VPD chart (temperature × humidity, banded by vapor-pressure
// deficit), but it suits any "where do my points sit in this regime?" view:
// pressure × temperature phase boundaries, pace × heart-rate training zones,
// price × volume regimes, and so on. The caller supplies the field function and
// the zone thresholds; the chart samples the field, paints the bands, and
// overlays the labeled points.

// --- Input types ------------------------------------------------------------

// PhaseBand is one zone: every field value strictly below Max (and at or above
// the previous band's Max) is painted Fill and named Label. The final band is
// the catch-all — give it Max = math.Inf(1). Fills should be light so the dark
// point markers and labels stay legible on top.
type PhaseBand struct {
	Max   float64 `json:"max"`
	Label string  `json:"label"`
	Fill  string  `json:"fill"` // any CSS color (hex or var())
}

// PhasePoint is one plotted observation.
type PhasePoint struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Label string  `json:"label,omitempty"`
}

// PhaseData drives a phase diagram. Field returns the banded scalar at a point;
// Bands map its ranges to fills (ascending by Max); Points are the overlay.
type PhaseData struct {
	XMin   float64 `json:"xMin,omitempty"`
	XMax   float64 `json:"xMax,omitempty"`
	YMin   float64 `json:"yMin,omitempty"`
	YMax   float64 `json:"yMax,omitempty"`
	XLabel string  `json:"xLabel,omitempty"`
	YLabel string  `json:"yLabel,omitempty"`
	Unit   string  `json:"unit,omitempty"` // unit of the field scalar, for tooltips

	Bands  []PhaseBand
	Points []PhasePoint

	// Field is the scalar evaluated across the plane to choose each cell's band.
	Field func(x, y float64) float64
	// Cells is the sampling resolution per axis (default 40). Adjacent same-band
	// cells in a row are merged, so the rendered rect count stays small.
	Cells int
	// Fmt formats the field scalar in tooltips (default 2 decimals).
	Fmt func(v float64) string
}

// --- Output geometry (1000x700 viewBox) -------------------------------------

// Phase is the rendered geometry. It shares the 1000x700 viewBox and tick type
// with the scatter chart.
type Phase struct {
	Cells  []PhaseCell
	Dots   []PhaseDot
	Legend []PhaseLegendItem
	YTicks []ScatterTick
	XTicks []ScatterTick
	PlotL  float64
	PlotR  float64
	PlotT  float64
	PlotB  float64
}

// PhaseCell is one painted run of the field (one or more merged grid cells).
type PhaseCell struct {
	X, Y, W, H float64
	Fill       string
}

// PhaseDot is a plotted point with its label and tooltip.
type PhaseDot struct {
	CX, CY, R      float64
	Label          string
	LabelX, LabelY float64
	LabelAnchor    string // "start" or "end"
	Tip            string
}

// PhaseLegendItem is one zone in the legend.
type PhaseLegendItem struct {
	Label string
	Fill  string
}

// --- Layout -----------------------------------------------------------------

// LayoutPhase samples the field into banded cells and maps the points onto the
// plot. It returns an empty Phase when no field or bands are given.
func LayoutPhase(data PhaseData) Phase {
	if data.Field == nil || len(data.Bands) == 0 {
		return Phase{}
	}
	cells := data.Cells
	if cells <= 0 {
		cells = 40
	}
	fmtV := data.Fmt
	if fmtV == nil {
		fmtV = func(v float64) string { return fmt.Sprintf("%.2f", v) }
	}

	const (
		vw   = 1000.0
		vh   = 700.0
		padL = 80.0
		padR = 30.0
		padT = 30.0
		padB = 60.0
	)
	plotW := vw - padL - padR
	plotH := vh - padT - padB

	xMin, xMax := data.XMin, data.XMax
	yMin, yMax := data.YMin, data.YMax
	spanX := xMax - xMin
	spanY := yMax - yMin
	if spanX == 0 {
		spanX = 1
	}
	if spanY == 0 {
		spanY = 1
	}

	out := Phase{PlotL: padL, PlotR: vw - padR, PlotT: padT, PlotB: vh - padB}

	// Paint the field, row by row, merging adjacent same-band cells into one rect
	// (run-length encoding) so a smooth field stays a few hundred rects, not
	// thousands. Each row's cells share a y; a run spans a contiguous column range.
	cw := plotW / float64(cells)
	ch := plotH / float64(cells)
	fillAt := func(i, j int) string {
		// Cell center in data coords; row j=0 is the top (yMax).
		x := xMin + (float64(i)+0.5)/float64(cells)*spanX
		y := yMax - (float64(j)+0.5)/float64(cells)*spanY
		return bandFill(data.Field(x, y), data.Bands)
	}
	for j := 0; j < cells; j++ {
		py := padT + float64(j)*ch
		start := 0
		cur := fillAt(0, j)
		for i := 1; i <= cells; i++ {
			var f string
			if i < cells {
				f = fillAt(i, j)
			}
			if i == cells || f != cur {
				out.Cells = append(out.Cells, PhaseCell{
					X:    padL + float64(start)*cw,
					Y:    py,
					W:    float64(i-start)*cw + 0.6, // slight overlap hides seams
					H:    ch + 0.6,
					Fill: cur,
				})
				start = i
				cur = f
			}
		}
	}

	for _, b := range data.Bands {
		out.Legend = append(out.Legend, PhaseLegendItem{Label: b.Label, Fill: b.Fill})
	}

	// Ticks use the exact ranges (no padding) so the field fills the plot.
	const nTicks = 5
	for i := 0; i <= nTicks; i++ {
		t := float64(i) / nTicks
		yPos := (vh - padB) - t*plotH
		out.YTicks = append(out.YTicks, ScatterTick{Pos: yPos, Label: fmt.Sprintf("%.0f", yMin+t*spanY)})
		xPos := padL + t*plotW
		out.XTicks = append(out.XTicks, ScatterTick{Pos: xPos, Label: fmt.Sprintf("%.0f", xMin+t*spanX)})
	}

	for _, p := range data.Points {
		tx := (p.X - xMin) / spanX
		ty := (p.Y - yMin) / spanY
		cx := padL + clamp01(tx)*plotW
		cy := (vh - padB) - clamp01(ty)*plotH

		v := data.Field(p.X, p.Y)
		title := p.Label
		if title == "" {
			title = fmt.Sprintf("(%.1f, %.1f)", p.X, p.Y)
		}
		tip := BuildTipHTML("var(--color-series-a)", title, []TipRow{
			{Label: data.XLabel, Value: fmt.Sprintf("%.1f", p.X)},
			{Label: data.YLabel, Value: fmt.Sprintf("%.1f", p.Y)},
			{Label: bandLabel(v, data.Bands), Value: fmtV(v) + " " + data.Unit},
		})

		// Label to the right normally; flip left near the right edge.
		anchor, lx := "start", cx+16
		if tx > 0.85 {
			anchor, lx = "end", cx-16
		}
		out.Dots = append(out.Dots, PhaseDot{
			CX: cx, CY: cy, R: 7,
			Label: p.Label, LabelX: lx, LabelY: cy, LabelAnchor: anchor, Tip: tip,
		})
	}

	return out
}

// bandFill returns the fill of the first band whose Max exceeds v, or the last
// band's fill (the catch-all).
func bandFill(v float64, bands []PhaseBand) string {
	for _, b := range bands {
		if v < b.Max {
			return b.Fill
		}
	}
	return bands[len(bands)-1].Fill
}

// bandLabel mirrors bandFill for the band name.
func bandLabel(v float64, bands []PhaseBand) string {
	for _, b := range bands {
		if v < b.Max {
			return b.Label
		}
	}
	return bands[len(bands)-1].Label
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
