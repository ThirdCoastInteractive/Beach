package chart

import (
	"fmt"
	"math"
)

// --- Input types ------------------------------------------------------------

type ScatterData struct {
	Series []ScatterSeries `json:"series"`
	XLabel string          `json:"xLabel,omitempty"`
	YLabel string          `json:"yLabel,omitempty"`
}

type ScatterSeries struct {
	Label  string         `json:"label"`
	Points []ScatterPoint `json:"points"`
}

type ScatterPoint struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Label string  `json:"label,omitempty"`
}

// --- Output geometry --------------------------------------------------------

// Scatter uses a 1000x700 viewBox.
type Scatter struct {
	Dots       []ScatterDot
	TrendLines []ScatterTrend
	GridYs     []float64
	GridXs     []float64
	YTicks     []ScatterTick
	XTicks     []ScatterTick
	Legend     []LegendItem
	PlotL      float64
	PlotR      float64
	PlotT      float64
	PlotB      float64
}

type ScatterTrend struct {
	X1, Y1, X2, Y2 float64
	Color          string
}

type ScatterDot struct {
	CX    float64
	CY    float64
	R     float64
	Color string
	Tip   string
}

type ScatterTick struct {
	Pos   float64
	Label string
}

// --- Layout -----------------------------------------------------------------

func LayoutScatter(data ScatterData) Scatter {
	if len(data.Series) == 0 {
		return Scatter{}
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

	var minX, maxX, minY, maxY float64
	first := true
	for _, s := range data.Series {
		for _, p := range s.Points {
			if first {
				minX, maxX = p.X, p.X
				minY, maxY = p.Y, p.Y
				first = false
			}
			minX = math.Min(minX, p.X)
			maxX = math.Max(maxX, p.X)
			minY = math.Min(minY, p.Y)
			maxY = math.Max(maxY, p.Y)
		}
	}
	spanX := maxX - minX
	spanY := maxY - minY
	if spanX == 0 {
		spanX = 1
	}
	if spanY == 0 {
		spanY = 1
	}
	padFracX := spanX * 0.05
	padFracY := spanY * 0.05
	minX -= padFracX
	maxX += padFracX
	minY -= padFracY
	maxY += padFracY
	spanX = maxX - minX
	spanY = maxY - minY

	out := Scatter{
		PlotL: padL, PlotR: vw - padR, PlotT: padT, PlotB: vh - padB,
	}

	nYTicks := 5
	for i := 0; i <= nYTicks; i++ {
		t := float64(i) / float64(nYTicks)
		yVal := minY + t*spanY
		yPos := (vh - padB) - t*plotH
		out.GridYs = append(out.GridYs, yPos)
		out.YTicks = append(out.YTicks, ScatterTick{Pos: yPos, Label: fmt.Sprintf("%.0f", yVal)})
	}

	nXTicks := 5
	for i := 0; i <= nXTicks; i++ {
		t := float64(i) / float64(nXTicks)
		xVal := minX + t*spanX
		xPos := padL + t*plotW
		out.GridXs = append(out.GridXs, xPos)
		out.XTicks = append(out.XTicks, ScatterTick{Pos: xPos, Label: fmt.Sprintf("%.0f", xVal)})
	}

	for si, s := range data.Series {
		color := ColorVar(si)
		out.Legend = append(out.Legend, LegendItem{Label: s.Label, Color: color})

		var sumX, sumY, sumXX, sumXY float64
		n := float64(len(s.Points))

		for _, p := range s.Points {
			tx := (p.X - minX) / spanX
			ty := (p.Y - minY) / spanY
			cx := padL + tx*plotW
			cy := (vh - padB) - ty*plotH

			label := p.Label
			if label == "" {
				label = fmt.Sprintf("(%.1f, %.1f)", p.X, p.Y)
			}
			tip := BuildTipHTML(color, label, []TipRow{
				{Label: data.XLabel, Value: fmt.Sprintf("%.1f", p.X)},
				{Label: data.YLabel, Value: fmt.Sprintf("%.1f", p.Y)},
			})

			out.Dots = append(out.Dots, ScatterDot{
				CX: cx, CY: cy, R: 6, Color: color, Tip: tip,
			})

			sumX += p.X
			sumY += p.Y
			sumXX += p.X * p.X
			sumXY += p.X * p.Y
		}

		if n >= 2 {
			denom := n*sumXX - sumX*sumX
			if denom != 0 {
				slope := (n*sumXY - sumX*sumY) / denom
				intercept := (sumY - slope*sumX) / n

				trendY0 := slope*minX + intercept
				trendY1 := slope*maxX + intercept

				x1 := padL
				y1 := (vh - padB) - ((trendY0-minY)/spanY)*plotH
				x2 := padL + plotW
				y2 := (vh - padB) - ((trendY1-minY)/spanY)*plotH

				out.TrendLines = append(out.TrendLines, ScatterTrend{
					X1: x1, Y1: y1, X2: x2, Y2: y2, Color: color,
				})
			}
		}
	}

	return out
}
