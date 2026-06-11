package chart

import (
	"io"
	"math"
)

// BollingerInput configures a Bollinger-band chart over a single value series.
type BollingerInput struct {
	Title  string
	Values []float64
	Window int     // moving-average window; defaults to 20
	K      float64 // band width in standard deviations; defaults to 2
	Width  float64 // defaults to 640
	Height float64 // defaults to 320
}

// BollingerLayout is the geometry for a Bollinger chart.
type BollingerLayout struct {
	W, H       float64
	Title      string
	Plot       Box
	BandD      string // closed upper+lower band area
	MidPts     [][2]float64
	PricePts   [][2]float64
	BandColor  string
	MidColor   string
	PriceColor string
	YTicks     []axisTick
}

// LayoutBollinger computes geometry for a Bollinger-band chart.
func LayoutBollinger(in BollingerInput) BollingerLayout {
	w := in.Width
	if w == 0 {
		w = 640
	}
	h := in.Height
	if h == 0 {
		h = 320
	}
	win := in.Window
	if win <= 0 {
		win = 20
	}
	k := in.K
	if k == 0 {
		k = 2
	}
	plot := Box{X: 48, Y: 16, W: w - 64, H: h - 40}
	lay := BollingerLayout{
		W: w, H: h, Title: in.Title, Plot: plot,
		BandColor: QualitativeVar(0), MidColor: QualitativeVar(1), PriceColor: QualitativeVar(2),
	}
	n := len(in.Values)
	if n == 0 {
		return lay
	}
	mid := make([]float64, n)
	upper := make([]float64, n)
	lower := make([]float64, n)
	for i := 0; i < n; i++ {
		lo := i - win + 1
		if lo < 0 {
			lo = 0
		}
		var sum float64
		cnt := 0
		for j := lo; j <= i; j++ {
			sum += in.Values[j]
			cnt++
		}
		m := sum / float64(cnt)
		var ss float64
		for j := lo; j <= i; j++ {
			d := in.Values[j] - m
			ss += d * d
		}
		sd := math.Sqrt(ss / float64(cnt))
		mid[i] = m
		upper[i] = m + k*sd
		lower[i] = m - k*sd
	}
	min, max := math.Inf(1), math.Inf(-1)
	for i := 0; i < n; i++ {
		min = math.Min(min, math.Min(lower[i], in.Values[i]))
		max = math.Max(max, math.Max(upper[i], in.Values[i]))
	}
	if max <= min {
		max = min + 1
	}
	sx := func(i int) float64 {
		if n == 1 {
			return plot.X + plot.W/2
		}
		return plot.X + float64(i)/float64(n-1)*plot.W
	}
	sy := func(v float64) float64 { return plot.Bottom() - (v-min)/(max-min)*plot.H }

	var up, lo [][2]float64
	for i := 0; i < n; i++ {
		up = append(up, [2]float64{sx(i), sy(upper[i])})
		lo = append(lo, [2]float64{sx(i), sy(lower[i])})
		lay.MidPts = append(lay.MidPts, [2]float64{sx(i), sy(mid[i])})
		lay.PricePts = append(lay.PricePts, [2]float64{sx(i), sy(in.Values[i])})
	}
	band := make([][2]float64, 0, 2*n)
	band = append(band, up...)
	for i := len(lo) - 1; i >= 0; i-- {
		band = append(band, lo[i])
	}
	lay.BandD = polyPath(band, true)
	for _, t := range niceTicks(min, max, 5) {
		lay.YTicks = append(lay.YTicks, axisTick{Pos: sy(t), Label: fnum(t)})
	}
	return lay
}

// BollingerSVG renders the Bollinger chart described by lay.
func BollingerSVG(w io.Writer, lay BollingerLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, t := range lay.YTicks {
		s.line(lay.Plot.X, t.Pos, lay.Plot.Right(), t.Pos,
			"var(--chart-grid, currentColor)",
			`stroke-width="1" stroke-opacity="0.15" vector-effect="non-scaling-stroke"`)
		s.text(lay.Plot.X-6, t.Pos+3, "end", "chart-axis", t.Label)
	}
	if lay.BandD != "" {
		s.path(lay.BandD, lay.BandColor, "", `fill-opacity="0.15"`)
	}
	s.p(`<g>`)
	s.title("moving average")
	s.p(`<path d="`, polyPath(lay.MidPts, false), `" fill="none" stroke="`, lay.MidColor,
		`" stroke-width="1.5" stroke-dasharray="4 3" vector-effect="non-scaling-stroke"/>`)
	s.p(`</g>`)
	s.p(`<g>`)
	s.title("price")
	s.p(`<path d="`, polyPath(lay.PricePts, false), `" fill="none" stroke="`, lay.PriceColor,
		`" stroke-width="2" stroke-linejoin="round" vector-effect="non-scaling-stroke"/>`)
	s.p(`</g>`)
	s.close()
	return s.Err()
}
