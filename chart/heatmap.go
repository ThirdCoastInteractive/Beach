package chart

import (
	"io"
	"math"
)

// HeatmapInput configures a labeled grid heatmap.
type HeatmapInput struct {
	Title  string
	Cols   []string
	Rows   []string
	Values [][]float64 // [row][col]
	Width  float64     // defaults to 480
	Height float64     // defaults to 320
}

// HeatmapLayout is the geometry for a grid heatmap.
type HeatmapLayout struct {
	W, H    float64
	Title   string
	Cells   []heatCell
	ColLbls []axisTick
	RowLbls []axisTick
}

type heatCell struct {
	Box     Box
	Opacity float64
	Color   string
	Tip     string
}

// LayoutHeatmap computes geometry for a grid heatmap. Cell intensity is encoded
// as opacity over a single qualitative hue so the scale reads sequentially.
func LayoutHeatmap(in HeatmapInput) HeatmapLayout {
	w := in.Width
	if w == 0 {
		w = 480
	}
	h := in.Height
	if h == 0 {
		h = 320
	}
	lay := HeatmapLayout{W: w, H: h, Title: in.Title}
	nRow, nCol := len(in.Rows), len(in.Cols)
	if nRow == 0 || nCol == 0 {
		return lay
	}
	left, top := 64.0, 24.0
	gridW := w - left - 8
	gridH := h - top - 24
	cw := gridW / float64(nCol)
	ch := gridH / float64(nRow)

	min, max := math.Inf(1), math.Inf(-1)
	for _, row := range in.Values {
		for _, v := range row {
			min = math.Min(min, v)
			max = math.Max(max, v)
		}
	}
	if max <= min {
		max = min + 1
	}
	color := QualitativeVar(0)
	for r := 0; r < nRow; r++ {
		for c := 0; c < nCol; c++ {
			var v float64
			if r < len(in.Values) && c < len(in.Values[r]) {
				v = in.Values[r][c]
			}
			op := 0.1 + 0.9*(v-min)/(max-min)
			lay.Cells = append(lay.Cells, heatCell{
				Box:     Box{X: left + float64(c)*cw, Y: top + float64(r)*ch, W: cw - 1, H: ch - 1},
				Opacity: op, Color: color,
				Tip: in.Rows[r] + " / " + in.Cols[c] + ": " + fnum(v),
			})
		}
		lay.RowLbls = append(lay.RowLbls, axisTick{Pos: top + float64(r)*ch + ch/2 + 3, Label: in.Rows[r]})
	}
	for c := 0; c < nCol; c++ {
		lay.ColLbls = append(lay.ColLbls, axisTick{Pos: left + float64(c)*cw + cw/2, Label: in.Cols[c]})
	}
	return lay
}

// HeatmapSVG renders the heatmap described by lay.
func HeatmapSVG(w io.Writer, lay HeatmapLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, cell := range lay.Cells {
		s.p(`<g>`)
		s.title(cell.Tip)
		s.rect(cell.Box, cell.Color, `fill-opacity="`+fnum(cell.Opacity)+`" rx="1"`)
		s.p(`</g>`)
	}
	for _, t := range lay.ColLbls {
		s.text(t.Pos, 16, "middle", "chart-axis", t.Label)
	}
	for _, t := range lay.RowLbls {
		s.text(58, t.Pos, "end", "chart-axis", t.Label)
	}
	s.close()
	return s.Err()
}
