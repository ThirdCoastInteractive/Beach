package chart

import (
	"fmt"
	"math"
)

// ============================================================================
// Horizon chart
// ============================================================================

// --- Input types ------------------------------------------------------------

type HorizonData struct {
	Series []HorizonSeries `json:"series"`
	Bands  int             `json:"bands,omitempty"`
	Unit   string          `json:"unit,omitempty"`
}

type HorizonSeries struct {
	Label  string    `json:"label"`
	Values []float64 `json:"values"`
}

// --- Output geometry --------------------------------------------------------

// Horizon uses a 1000x(n*rowH) viewBox.
type Horizon struct {
	Rows []HorizonRow
	VH   float64
}

type HorizonRow struct {
	Label string
	Bands []HorizonBand
	Y     float64
	H     float64
}

type HorizonBand struct {
	PathD string
	Color string
}

// --- Helpers ----------------------------------------------------------------

func HorizonViewBox(h Horizon) string {
	vh := h.VH
	if vh < 50 {
		vh = 200
	}
	return "0 0 1000 " + F(vh)
}

// --- Layout -----------------------------------------------------------------

func LayoutHorizon(data HorizonData) Horizon {
	nSeries := len(data.Series)
	if nSeries == 0 {
		return Horizon{}
	}
	bands := data.Bands
	if bands <= 0 {
		bands = 4
	}

	const (
		vw     = 1000.0
		padL   = 100.0
		padR   = 10.0
		rowH   = 50.0
		rowGap = 4.0
	)
	plotW := vw - padL - padR
	totalH := float64(nSeries) * (rowH + rowGap)

	// Find global max across all series.
	var globalMax float64
	for _, s := range data.Series {
		for _, v := range s.Values {
			if math.Abs(v) > globalMax {
				globalMax = math.Abs(v)
			}
		}
	}
	if globalMax == 0 {
		globalMax = 1
	}
	bandSize := globalMax / float64(bands)

	out := Horizon{VH: totalH + 10}

	for si, s := range data.Series {
		n := len(s.Values)
		if n == 0 {
			continue
		}
		y := float64(si) * (rowH + rowGap)

		row := HorizonRow{
			Label: s.Label,
			Y:     y,
			H:     rowH,
		}

		for b := 0; b < bands; b++ {
			lo := float64(b) * bandSize
			t := float64(b+1) / float64(bands)
			l := 0.25 + t*0.40
			ch := 0.04 + t*0.14
			color := fmt.Sprintf("oklch(%.2f %.2f 250)", l, ch)

			d := ""
			for i := 0; i < n; i++ {
				x := padL + (float64(i)/float64(n-1))*plotW
				v := s.Values[i]
				if v < 0 {
					v = -v
				}
				clipped := v - lo
				if clipped < 0 {
					clipped = 0
				}
				if clipped > bandSize {
					clipped = bandSize
				}
				frac := clipped / bandSize
				cy := y + rowH - frac*rowH

				if i == 0 {
					d = fmt.Sprintf("M %.1f %.1f", x, y+rowH)
					d += fmt.Sprintf(" L %.1f %.1f", x, cy)
				} else {
					d += fmt.Sprintf(" L %.1f %.1f", x, cy)
				}
			}
			lastX := padL + plotW
			d += fmt.Sprintf(" L %.1f %.1f Z", lastX, y+rowH)

			row.Bands = append(row.Bands, HorizonBand{PathD: d, Color: color})
		}

		out.Rows = append(out.Rows, row)
	}

	return out
}

// ============================================================================
// Ridgeline chart
// ============================================================================

// --- Input types ------------------------------------------------------------

type RidgelineData struct {
	Series []RidgelineSeries `json:"series"`
	Unit   string            `json:"unit,omitempty"`
}

type RidgelineSeries struct {
	Label  string    `json:"label"`
	Values []float64 `json:"values"`
}

// --- Output geometry --------------------------------------------------------

// Ridgeline uses a 1000x(n*rowH) viewBox.
type Ridgeline struct {
	Rows []RidgelineRow
	VH   float64
}

type RidgelineRow struct {
	Label string
	PathD string
	Color string
	Y     float64
}

// --- Helpers ----------------------------------------------------------------

func RidgelineViewBox(r Ridgeline) string {
	vh := r.VH
	if vh < 50 {
		vh = 400
	}
	return "0 0 1000 " + F(vh)
}

// --- Layout -----------------------------------------------------------------

func LayoutRidgeline(data RidgelineData) Ridgeline {
	nSeries := len(data.Series)
	if nSeries == 0 {
		return Ridgeline{}
	}

	const (
		vw      = 1000.0
		padL    = 100.0
		padR    = 10.0
		rowH    = 80.0
		overlap = 30.0
	)
	plotW := vw - padL - padR
	totalH := float64(nSeries)*rowH - float64(nSeries-1)*overlap + 20

	var globalMax float64
	for _, s := range data.Series {
		for _, v := range s.Values {
			if v > globalMax {
				globalMax = v
			}
		}
	}
	if globalMax == 0 {
		globalMax = 1
	}

	out := Ridgeline{VH: totalH}

	for si, s := range data.Series {
		n := len(s.Values)
		if n == 0 {
			continue
		}
		baseY := float64(si) * (rowH - overlap)
		color := ColorVar(si)

		d := fmt.Sprintf("M %.1f %.1f", padL, baseY+rowH)
		for i := 0; i < n; i++ {
			x := padL + (float64(i)/float64(n-1))*plotW
			frac := s.Values[i] / globalMax
			y := baseY + rowH - frac*rowH*0.8
			d += fmt.Sprintf(" L %.1f %.1f", x, y)
		}
		d += fmt.Sprintf(" L %.1f %.1f Z", padL+plotW, baseY+rowH)

		out.Rows = append(out.Rows, RidgelineRow{
			Label: s.Label,
			PathD: d,
			Color: color,
			Y:     baseY + rowH,
		})
	}

	return out
}

// ============================================================================
// Difference chart
// ============================================================================

// --- Input types ------------------------------------------------------------

type DifferenceData struct {
	Labels  []string   `json:"labels"`
	SeriesA DiffSeries `json:"seriesA"`
	SeriesB DiffSeries `json:"seriesB"`
	YLabel  string     `json:"yLabel,omitempty"`
	Unit    string     `json:"unit,omitempty"`
}

type DiffSeries struct {
	Label  string    `json:"label"`
	Values []float64 `json:"values"`
}

// --- Output geometry --------------------------------------------------------

// Difference uses a 1000x600 viewBox.
type Difference struct {
	PathA   string
	PathB   string
	AboveD  string
	BelowD  string
	ColorA  string
	ColorB  string
	XTicks  []StreamTick
	YTicks  []ScatterTick
	GridYs  []float64
	PlotL   float64
	PlotR   float64
	PlotT   float64
	PlotB   float64
	LegendA string
	LegendB string

	// HoverID names the JSON payload the client crosshair reads, and Hover is
	// the payload: one entry per sample, each carrying the sentence to announce
	// when a keyboard lands on it (WCAG 2.1.1).
	HoverID string
	Hover   VBHover
}

// --- Layout -----------------------------------------------------------------

func LayoutDifference(data DifferenceData) Difference {
	n := len(data.Labels)
	if n == 0 || len(data.SeriesA.Values) != n || len(data.SeriesB.Values) != n {
		return Difference{}
	}

	const (
		vw   = 1000.0
		vh   = 600.0
		padL = 70.0
		padR = 20.0
		padT = 20.0
		padB = 50.0
	)
	plotW := vw - padL - padR
	plotH := vh - padT - padB

	var minY, maxY float64
	minY = data.SeriesA.Values[0]
	maxY = minY
	for i := 0; i < n; i++ {
		for _, v := range []float64{data.SeriesA.Values[i], data.SeriesB.Values[i]} {
			if v < minY {
				minY = v
			}
			if v > maxY {
				maxY = v
			}
		}
	}
	spanY := maxY - minY
	if spanY == 0 {
		spanY = 1
	}
	padFrac := spanY * 0.05
	minY -= padFrac
	maxY += padFrac
	spanY = maxY - minY

	xMap := func(i int) float64 { return padL + (float64(i)/float64(n-1))*plotW }
	yMap := func(v float64) float64 { return (vh - padB) - ((v-minY)/spanY)*plotH }

	colorA := ColorVar(0)
	colorB := ColorVar(1)

	var pathA, pathB string
	for i := 0; i < n; i++ {
		x := xMap(i)
		if i == 0 {
			pathA = fmt.Sprintf("M %.1f %.1f", x, yMap(data.SeriesA.Values[i]))
			pathB = fmt.Sprintf("M %.1f %.1f", x, yMap(data.SeriesB.Values[i]))
		} else {
			pathA += fmt.Sprintf(" L %.1f %.1f", x, yMap(data.SeriesA.Values[i]))
			pathB += fmt.Sprintf(" L %.1f %.1f", x, yMap(data.SeriesB.Values[i]))
		}
	}

	// Build above/below fill areas.
	aboveD := pathA
	for i := n - 1; i >= 0; i-- {
		aboveD += fmt.Sprintf(" L %.1f %.1f", xMap(i), yMap(data.SeriesB.Values[i]))
	}
	aboveD += " Z"

	out := Difference{
		PathA: pathA, PathB: pathB,
		AboveD: aboveD, BelowD: aboveD,
		ColorA: colorA, ColorB: colorB,
		PlotL: padL, PlotR: vw - padR, PlotT: padT, PlotB: vh - padB,
		LegendA: data.SeriesA.Label, LegendB: data.SeriesB.Label,
	}

	nYTicks := 5
	for i := 0; i <= nYTicks; i++ {
		t := float64(i) / float64(nYTicks)
		yVal := minY + t*spanY
		yPos := yMap(yVal)
		out.GridYs = append(out.GridYs, yPos)
		out.YTicks = append(out.YTicks, ScatterTick{Pos: yPos, Label: fmt.Sprintf("%.0f", yVal)})
	}

	maxTicks := 8
	step := 1
	if n > maxTicks {
		step = n / maxTicks
	}
	for i := 0; i < n; i += step {
		out.XTicks = append(out.XTicks, StreamTick{X: xMap(i), Label: data.Labels[i]})
	}

	// Both series in one sentence, because the chart's whole subject is the gap
	// between them: reading one value without the other says nothing about the
	// thing being shown.
	unit := ""
	if data.Unit != "" {
		unit = " " + data.Unit
	}
	out.Hover = vbHover(data.Labels, func(i int) string {
		return fmt.Sprintf("%s: %s %.2f%s, %s %.2f%s",
			data.Labels[i],
			data.SeriesA.Label, data.SeriesA.Values[i], unit,
			data.SeriesB.Label, data.SeriesB.Values[i], unit)
	})

	return out
}

// ============================================================================
// Timeline chart
// ============================================================================

// --- Input types ------------------------------------------------------------

type TimelineData struct {
	Rows []TimelineRow `json:"rows"`
}

type TimelineRow struct {
	Label string  `json:"label"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Group string  `json:"group"`
}

// --- Output geometry --------------------------------------------------------

// Timeline uses a 1000x(n*rowH) viewBox.
type Timeline struct {
	Bars   []TimelineBar
	XTicks []StreamTick
	GridXs []float64
	VH     float64
	PlotL  float64
	PlotR  float64
}

type TimelineBar struct {
	Label string
	X     float64
	Y     float64
	W     float64
	H     float64
	Color string
	Tip   string
}

// --- Helpers ----------------------------------------------------------------

func TimelineViewBox(t Timeline) string {
	vh := t.VH
	if vh < 50 {
		vh = 400
	}
	return "0 0 1000 " + F(vh)
}

// --- Layout -----------------------------------------------------------------

func LayoutTimeline(data TimelineData) Timeline {
	n := len(data.Rows)
	if n == 0 {
		return Timeline{}
	}

	const (
		vw     = 1000.0
		padL   = 120.0
		padR   = 20.0
		padT   = 30.0
		rowH   = 20.0
		rowGap = 3.0
	)
	plotW := vw - padL - padR
	totalH := padT + float64(n)*(rowH+rowGap) + 20

	var minT, maxT float64
	minT = data.Rows[0].Start
	maxT = data.Rows[0].End
	for _, r := range data.Rows {
		if r.Start < minT {
			minT = r.Start
		}
		if r.End > maxT {
			maxT = r.End
		}
	}
	span := maxT - minT
	if span == 0 {
		span = 1
	}

	groupColor := make(map[string]string)
	groupIdx := 0
	for _, r := range data.Rows {
		if _, ok := groupColor[r.Group]; !ok {
			groupColor[r.Group] = ColorVar(groupIdx)
			groupIdx++
		}
	}

	out := Timeline{VH: totalH, PlotL: padL, PlotR: vw - padR}

	for i, r := range data.Rows {
		y := padT + float64(i)*(rowH+rowGap)
		x := padL + ((r.Start-minT)/span)*plotW
		w := ((r.End - r.Start) / span) * plotW
		if w < 2 {
			w = 2
		}
		color := groupColor[r.Group]

		tip := BuildTipHTML(color, r.Label, []TipRow{
			{Label: "start", Value: fmt.Sprintf("%.0f", r.Start)},
			{Label: "end", Value: fmt.Sprintf("%.0f", r.End)},
			{Label: "group", Value: r.Group},
		})

		out.Bars = append(out.Bars, TimelineBar{
			Label: r.Label, X: x, Y: y, W: w, H: rowH, Color: color, Tip: tip,
		})
	}

	nTicks := 6
	for i := 0; i <= nTicks; i++ {
		t := float64(i) / float64(nTicks)
		xVal := minT + t*span
		xPos := padL + t*plotW
		out.GridXs = append(out.GridXs, xPos)
		out.XTicks = append(out.XTicks, StreamTick{X: xPos, Label: fmt.Sprintf("%.0f", xVal)})
	}

	return out
}
