package chart

import (
	"fmt"
	"math"
)

// --- Input types ------------------------------------------------------------

type StreamData struct {
	Labels []string      `json:"labels"`
	Layers []StreamLayer `json:"layers"`
	Unit   string        `json:"unit,omitempty"`
}

type StreamLayer struct {
	Label  string    `json:"label"`
	Values []float64 `json:"values"`
}

// --- Output geometry --------------------------------------------------------

// Stream uses a 1000x600 viewBox.
type Stream struct {
	Layers []StreamChartLayer
	XTicks []StreamTick
	Legend []LegendItem
}

type StreamChartLayer struct {
	PathD string
	Color string
	Tip   string
}

type StreamTick struct {
	X     float64
	Label string
}

// --- Layout -----------------------------------------------------------------

func LayoutStream(data StreamData) Stream {
	nPoints := len(data.Labels)
	nLayers := len(data.Layers)
	if nPoints == 0 || nLayers == 0 {
		return Stream{}
	}

	const (
		vw   = 1000.0
		vh   = 600.0
		padL = 30.0
		padR = 10.0
		padT = 10.0
		padB = 50.0
	)
	plotW := vw - padL - padR
	plotH := vh - padT - padB

	colTotals := make([]float64, nPoints)
	var maxTotal float64
	for j := 0; j < nPoints; j++ {
		for i := 0; i < nLayers; i++ {
			if j < len(data.Layers[i].Values) {
				colTotals[j] += data.Layers[i].Values[j]
			}
		}
		maxTotal = math.Max(maxTotal, colTotals[j])
	}
	if maxTotal == 0 {
		maxTotal = 1
	}

	baselines := make([]float64, nPoints)
	for j := 0; j < nPoints; j++ {
		baselines[j] = (maxTotal - colTotals[j]) / 2
	}

	out := Stream{}

	for i := 0; i < nLayers; i++ {
		color := ColorVar(i)

		d := ""
		for j := 0; j < nPoints; j++ {
			x := padL + (float64(j)/float64(nPoints-1))*plotW

			var below float64
			for k := 0; k < i; k++ {
				if j < len(data.Layers[k].Values) {
					below += data.Layers[k].Values[j]
				}
			}

			v := 0.0
			if j < len(data.Layers[i].Values) {
				v = data.Layers[i].Values[j]
			}

			yTop := padT + plotH - ((baselines[j]+below+v)/maxTotal)*plotH

			if j == 0 {
				d = fmt.Sprintf("M %.1f %.1f", x, yTop)
			} else {
				d += fmt.Sprintf(" L %.1f %.1f", x, yTop)
			}
		}

		for j := nPoints - 1; j >= 0; j-- {
			x := padL + (float64(j)/float64(nPoints-1))*plotW

			var below float64
			for k := 0; k < i; k++ {
				if j < len(data.Layers[k].Values) {
					below += data.Layers[k].Values[j]
				}
			}

			yBottom := padT + plotH - ((baselines[j]+below)/maxTotal)*plotH
			d += fmt.Sprintf(" L %.1f %.1f", x, yBottom)
		}
		d += " Z"

		var layerTotal float64
		for _, v := range data.Layers[i].Values {
			layerTotal += v
		}
		tip := BuildTipHTML(color, data.Layers[i].Label,
			[]TipRow{{Label: "total", Value: fmt.Sprintf("%.0f", layerTotal) + " " + data.Unit}})

		out.Layers = append(out.Layers, StreamChartLayer{PathD: d, Color: color, Tip: tip})
		out.Legend = append(out.Legend, LegendItem{Label: data.Layers[i].Label, Color: color})
	}

	maxTicks := 8
	step := 1
	if nPoints > maxTicks {
		step = nPoints / maxTicks
	}
	for j := 0; j < nPoints; j += step {
		x := padL + (float64(j)/float64(nPoints-1))*plotW
		out.XTicks = append(out.XTicks, StreamTick{X: x, Label: data.Labels[j]})
	}

	return out
}
