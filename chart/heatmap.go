package chart

import "fmt"

// --- Input types ------------------------------------------------------------

type HeatmapData struct {
	Rows  []string      `json:"rows"`
	Cols  []string      `json:"cols"`
	Cells []HeatmapCell `json:"cells"`
	Unit  string        `json:"unit,omitempty"`
}

type HeatmapCell struct {
	Row   int     `json:"row"`
	Col   int     `json:"col"`
	Value float64 `json:"value"`
}

// --- Output geometry --------------------------------------------------------

type Heatmap struct {
	Cells    []HeatmapGridCell
	RowCount int
	ColCount int
	Rows     []HeatmapLabel
	Cols     []HeatmapLabel
	Tip      string
}

type HeatmapGridCell struct {
	X     string
	Y     string
	W     string
	H     string
	Color string
	Tip   string
}

type HeatmapLabel struct {
	Text string
	X    string
	Y    string
}

// --- Layout -----------------------------------------------------------------

func LayoutHeatmap(data HeatmapData) Heatmap {
	nRows := len(data.Rows)
	nCols := len(data.Cols)
	if nRows == 0 || nCols == 0 {
		return Heatmap{}
	}

	const (
		labelLeft   = 16.0
		labelTop    = 8.0
		plotLeft    = 17.0
		plotTop     = 9.0
		plotRight   = 99.0
		plotBottom  = 95.0
		cellPadding = 0.3
	)
	plotW := plotRight - plotLeft
	plotH := plotBottom - plotTop
	cellW := plotW / float64(nCols)
	cellH := plotH / float64(nRows)

	var minV, maxV float64
	first := true
	for _, c := range data.Cells {
		if first || c.Value < minV {
			minV = c.Value
		}
		if first || c.Value > maxV {
			maxV = c.Value
		}
		first = false
	}
	span := maxV - minV
	if span == 0 {
		span = 1
	}

	out := Heatmap{RowCount: nRows, ColCount: nCols}

	for i, label := range data.Rows {
		y := plotTop + (float64(i)+0.5)*cellH
		out.Rows = append(out.Rows, HeatmapLabel{
			Text: label,
			X:    Pct(labelLeft),
			Y:    Pct(y),
		})
	}
	for j, label := range data.Cols {
		x := plotLeft + (float64(j)+0.5)*cellW
		out.Cols = append(out.Cols, HeatmapLabel{
			Text: label,
			X:    Pct(x),
			Y:    Pct(labelTop),
		})
	}

	for _, c := range data.Cells {
		t := (c.Value - minV) / span
		l := 0.25 + t*0.45
		ch := 0.02 + t*0.16
		color := fmt.Sprintf("oklch(%.2f %.2f 250)", l, ch)

		x := plotLeft + float64(c.Col)*cellW + cellPadding
		y := plotTop + float64(c.Row)*cellH + cellPadding
		w := cellW - 2*cellPadding
		h := cellH - 2*cellPadding

		tip := BuildTipHTML(color, data.Rows[c.Row]+" / "+data.Cols[c.Col],
			[]TipRow{{Label: "value", Value: fmt.Sprintf("%.0f", c.Value) + " " + data.Unit}})

		out.Cells = append(out.Cells, HeatmapGridCell{
			X:     Pct(x),
			Y:     Pct(y),
			W:     Pct(w),
			H:     Pct(h),
			Color: color,
			Tip:   tip,
		})
	}
	return out
}
