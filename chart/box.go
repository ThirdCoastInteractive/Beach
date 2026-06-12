package chart

import "fmt"

// --- Input types ------------------------------------------------------------

type BoxPlotData struct {
	Groups []BoxGroup `json:"groups"`
	YLabel string     `json:"yLabel,omitempty"`
	Unit   string     `json:"unit,omitempty"`
}

type BoxGroup struct {
	Label string  `json:"label"`
	Min   float64 `json:"min"`
	Q1    float64 `json:"q1"`
	Med   float64 `json:"med"`
	Q3    float64 `json:"q3"`
	Max   float64 `json:"max"`
}

// --- Output geometry --------------------------------------------------------

// BoxPlot uses a 1000x700 viewBox.
type BoxPlot struct {
	Boxes  []Box
	GridYs []float64
	YTicks []ScatterTick
	PlotL  float64
	PlotR  float64
	PlotT  float64
	PlotB  float64
}

type Box struct {
	Label  string
	CX     float64
	BoxW   float64
	MinY   float64
	Q1Y    float64
	MedY   float64
	Q3Y    float64
	MaxY   float64
	Color  string
	Tip    string
	LabelY float64
}

// --- Layout -----------------------------------------------------------------

func LayoutBoxPlot(data BoxPlotData) BoxPlot {
	n := len(data.Groups)
	if n == 0 {
		return BoxPlot{}
	}

	const (
		vw   = 1000.0
		vh   = 700.0
		padL = 80.0
		padR = 30.0
		padT = 20.0
		padB = 70.0
	)
	plotW := vw - padL - padR
	plotH := vh - padT - padB

	var minV, maxV float64
	minV = data.Groups[0].Min
	maxV = data.Groups[0].Max
	for _, g := range data.Groups {
		if g.Min < minV {
			minV = g.Min
		}
		if g.Max > maxV {
			maxV = g.Max
		}
	}
	span := maxV - minV
	if span == 0 {
		span = 1
	}
	padFrac := span * 0.1
	minV -= padFrac
	maxV += padFrac
	span = maxV - minV

	out := BoxPlot{
		PlotL: padL, PlotR: vw - padR, PlotT: padT, PlotB: vh - padB,
	}

	yMap := func(v float64) float64 {
		return (vh - padB) - ((v-minV)/span)*plotH
	}

	nTicks := 6
	for i := 0; i <= nTicks; i++ {
		t := float64(i) / float64(nTicks)
		yVal := minV + t*span
		yPos := yMap(yVal)
		out.GridYs = append(out.GridYs, yPos)
		out.YTicks = append(out.YTicks, ScatterTick{Pos: yPos, Label: fmt.Sprintf("%.0f", yVal)})
	}

	band := plotW / float64(n)
	boxW := band * 0.5

	for i, g := range data.Groups {
		cx := padL + (float64(i)+0.5)*band
		color := ColorVar(i)

		tip := BuildTipHTML(color, g.Label, []TipRow{
			{Label: "max", Value: fmt.Sprintf("%.0f", g.Max) + " " + data.Unit},
			{Label: "Q3", Value: fmt.Sprintf("%.0f", g.Q3) + " " + data.Unit},
			{Label: "median", Value: fmt.Sprintf("%.0f", g.Med) + " " + data.Unit},
			{Label: "Q1", Value: fmt.Sprintf("%.0f", g.Q1) + " " + data.Unit},
			{Label: "min", Value: fmt.Sprintf("%.0f", g.Min) + " " + data.Unit},
		})

		out.Boxes = append(out.Boxes, Box{
			Label:  g.Label,
			CX:     cx,
			BoxW:   boxW,
			MinY:   yMap(g.Min),
			Q1Y:    yMap(g.Q1),
			MedY:   yMap(g.Med),
			Q3Y:    yMap(g.Q3),
			MaxY:   yMap(g.Max),
			Color:  color,
			Tip:    tip,
			LabelY: vh - padB + 30,
		})
	}

	return out
}
