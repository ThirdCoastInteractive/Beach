package chart

import (
	"fmt"
	"math"
)

// --- Input types ------------------------------------------------------------

type BollingerData struct {
	Points []BollingerPoint `json:"points"`
	Window int              `json:"window,omitempty"`
	K      float64          `json:"k,omitempty"`
	YLabel string           `json:"yLabel,omitempty"`
	Unit   string           `json:"unit,omitempty"`
}

type BollingerPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// --- Output geometry --------------------------------------------------------

// Bollinger uses a 1000x600 viewBox.
type Bollinger struct {
	LinePath  string
	UpperPath string
	LowerPath string
	BandPath  string
	MidPath   string
	XTicks    []StreamTick
	YTicks    []ScatterTick
	GridYs    []float64
	PlotL     float64
	PlotR     float64
	PlotT     float64
	PlotB     float64
	Color     string

	// HoverID names the JSON payload the client crosshair reads, and Hover is
	// the payload: one entry per sample, each carrying the sentence to announce
	// when a keyboard lands on it (WCAG 2.1.1).
	HoverID string
	Hover   VBHover
}

// --- Layout -----------------------------------------------------------------

func LayoutBollinger(data BollingerData) Bollinger {
	n := len(data.Points)
	if n == 0 {
		return Bollinger{}
	}

	window := data.Window
	if window <= 0 {
		window = 20
	}
	k := data.K
	if k <= 0 {
		k = 2
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

	// Compute SMA and standard deviation.
	sma := make([]float64, n)
	stddev := make([]float64, n)
	for i := 0; i < n; i++ {
		start := i - window + 1
		if start < 0 {
			start = 0
		}
		count := float64(i - start + 1)
		var sum float64
		for j := start; j <= i; j++ {
			sum += data.Points[j].Value
		}
		mean := sum / count
		sma[i] = mean
		var variance float64
		for j := start; j <= i; j++ {
			d := data.Points[j].Value - mean
			variance += d * d
		}
		stddev[i] = math.Sqrt(variance / count)
	}

	upper := make([]float64, n)
	lower := make([]float64, n)
	for i := 0; i < n; i++ {
		upper[i] = sma[i] + k*stddev[i]
		lower[i] = sma[i] - k*stddev[i]
	}

	// Find Y extents.
	minY := lower[0]
	maxY := upper[0]
	for i := 0; i < n; i++ {
		minY = math.Min(minY, math.Min(lower[i], data.Points[i].Value))
		maxY = math.Max(maxY, math.Max(upper[i], data.Points[i].Value))
	}
	spanY := maxY - minY
	if spanY == 0 {
		spanY = 1
	}
	padFrac := spanY * 0.05
	minY -= padFrac
	maxY += padFrac
	spanY = maxY - minY

	xMap := func(i int) float64 {
		return padL + (float64(i)/float64(n-1))*plotW
	}
	yMap := func(v float64) float64 {
		return (vh - padB) - ((v-minY)/spanY)*plotH
	}

	color := ColorVar(0)

	// Build paths.
	var linePath, upperPath, lowerPath, midPath, bandPath string
	for i := 0; i < n; i++ {
		x := xMap(i)
		if i == 0 {
			linePath = fmt.Sprintf("M %.1f %.1f", x, yMap(data.Points[i].Value))
			upperPath = fmt.Sprintf("M %.1f %.1f", x, yMap(upper[i]))
			lowerPath = fmt.Sprintf("M %.1f %.1f", x, yMap(lower[i]))
			midPath = fmt.Sprintf("M %.1f %.1f", x, yMap(sma[i]))
			bandPath = fmt.Sprintf("M %.1f %.1f", x, yMap(upper[i]))
		} else {
			linePath += fmt.Sprintf(" L %.1f %.1f", x, yMap(data.Points[i].Value))
			upperPath += fmt.Sprintf(" L %.1f %.1f", x, yMap(upper[i]))
			lowerPath += fmt.Sprintf(" L %.1f %.1f", x, yMap(lower[i]))
			midPath += fmt.Sprintf(" L %.1f %.1f", x, yMap(sma[i]))
			bandPath += fmt.Sprintf(" L %.1f %.1f", x, yMap(upper[i]))
		}
	}
	for i := n - 1; i >= 0; i-- {
		bandPath += fmt.Sprintf(" L %.1f %.1f", xMap(i), yMap(lower[i]))
	}
	bandPath += " Z"

	out := Bollinger{
		LinePath:  linePath,
		UpperPath: upperPath,
		LowerPath: lowerPath,
		BandPath:  bandPath,
		MidPath:   midPath,
		PlotL:     padL,
		PlotR:     vw - padR,
		PlotT:     padT,
		PlotB:     vh - padB,
		Color:     color,
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
		out.XTicks = append(out.XTicks, StreamTick{X: xMap(i), Label: data.Points[i].Label})
	}

	// The crosshair shows a position; this is what it can say. Built here
	// because the unit and the number formatting are here, and the client has
	// neither.
	unit := ""
	if data.Unit != "" {
		unit = " " + data.Unit
	}
	labels := make([]string, n)
	for i, pt := range data.Points {
		labels[i] = pt.Label
	}
	out.Hover = vbHover(labels, func(i int) string {
		return fmt.Sprintf("%s: %.2f%s", data.Points[i].Label, data.Points[i].Value, unit)
	})

	return out
}
