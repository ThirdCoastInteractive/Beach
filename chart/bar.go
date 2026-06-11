package chart

import (
	"io"
	"math"
)

// Datum is one labeled value, the unit of bar/gauge/billboard input.
type Datum struct {
	Label string
	Value float64
}

// HBarInput configures a horizontal bar chart.
type HBarInput struct {
	Series []Datum
	Title  string
	Unit   string  // appended to value labels, e.g. " kW"
	Max    float64 // 0 = auto from data
	Width  float64 // defaults to 480
	RowH   float64 // per-row height; defaults to 28
}

// HBarLayout is the computed geometry for a horizontal bar chart.
type HBarLayout struct {
	W, H  float64
	Title string
	Bars  []hbar
}

type hbar struct {
	Box   Box
	Color string
	Label string
	Value string // formatted value+unit
	ValX  float64
}

// LayoutHBar computes geometry for a horizontal bar chart.
func LayoutHBar(in HBarInput) HBarLayout {
	w := in.Width
	if w == 0 {
		w = 480
	}
	rowH := in.RowH
	if rowH == 0 {
		rowH = 28
	}
	max := in.Max
	if max <= 0 {
		for _, d := range in.Series {
			max = math.Max(max, d.Value)
		}
	}
	if max <= 0 {
		max = 1
	}
	labelW := 120.0
	trackX := labelW
	trackW := w - labelW - 56 // room for value text on the right
	h := rowH*float64(len(in.Series)) + 8

	lay := HBarLayout{W: w, H: h, Title: in.Title}
	for i, d := range in.Series {
		y := 4 + float64(i)*rowH
		bw := d.Value / max * trackW
		if bw < 0 {
			bw = 0
		}
		lay.Bars = append(lay.Bars, hbar{
			Box:   Box{X: trackX, Y: y + 4, W: bw, H: rowH - 12},
			Color: QualitativeVar(i),
			Label: d.Label,
			Value: fnum(d.Value) + in.Unit,
			ValX:  trackX + bw + 6,
		})
	}
	return lay
}

// HBarSVG renders the horizontal bar chart described by lay.
func HBarSVG(w io.Writer, lay HBarLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, b := range lay.Bars {
		cy := b.Box.Y + b.Box.H/2 + 4
		s.text(b.Box.X-8, cy, "end", "chart-label", b.Label)
		s.p(`<g>`)
		s.title(b.Label + ": " + b.Value)
		s.rect(b.Box, b.Color, `rx="2"`)
		s.p(`</g>`)
		s.text(b.ValX, cy, "start", "chart-value", b.Value)
	}
	s.close()
	return s.Err()
}

// StackedBarInput configures a stacked or grouped vertical bar chart.
type StackedBarInput struct {
	Categories []string // x-axis groups
	Series     []Datum2 // each series spans all categories
	Title      string
	Grouped    bool    // true = side-by-side; false = stacked
	Width      float64 // defaults to 560
	Height     float64 // defaults to 320
}

// Datum2 is a named series of values aligned to a category axis.
type Datum2 struct {
	Name   string
	Values []float64
}

// StackedBarLayout is the geometry for a stacked/grouped vertical bar chart.
type StackedBarLayout struct {
	W, H    float64
	Title   string
	Plot    Box
	Rects   []barRect
	YTicks  []axisTick
	XLabels []axisTick
}

type barRect struct {
	Box    Box
	Color  string
	Series string
	Cat    string
	Value  float64
}

// LayoutStackedBar computes geometry for a stacked or grouped vertical bar chart.
func LayoutStackedBar(in StackedBarInput) StackedBarLayout {
	w := in.Width
	if w == 0 {
		w = 560
	}
	h := in.Height
	if h == 0 {
		h = 320
	}
	plot := Box{X: 48, Y: 16, W: w - 64, H: h - 40}
	nCat := len(in.Categories)
	if nCat == 0 {
		return StackedBarLayout{W: w, H: h, Title: in.Title, Plot: plot}
	}

	// Determine y-axis max.
	max := 0.0
	if in.Grouped {
		for _, s := range in.Series {
			for _, v := range s.Values {
				max = math.Max(max, v)
			}
		}
	} else {
		for c := 0; c < nCat; c++ {
			sum := 0.0
			for _, s := range in.Series {
				if c < len(s.Values) {
					sum += s.Values[c]
				}
			}
			max = math.Max(max, sum)
		}
	}
	if max <= 0 {
		max = 1
	}

	lay := StackedBarLayout{W: w, H: h, Title: in.Title, Plot: plot}
	sy := func(v float64) float64 { return plot.Bottom() - v/max*plot.H }
	catW := plot.W / float64(nCat)
	for c := 0; c < nCat; c++ {
		cx := plot.X + float64(c)*catW
		lay.XLabels = append(lay.XLabels, axisTick{Pos: cx + catW/2, Label: in.Categories[c]})
		if in.Grouped {
			nS := len(in.Series)
			if nS == 0 {
				continue
			}
			slot := catW * 0.8 / float64(nS)
			off := catW * 0.1
			for si, s := range in.Series {
				v := 0.0
				if c < len(s.Values) {
					v = s.Values[c]
				}
				bx := cx + off + float64(si)*slot
				top := sy(v)
				lay.Rects = append(lay.Rects, barRect{
					Box:    Box{X: bx, Y: top, W: slot * 0.9, H: plot.Bottom() - top},
					Color:  QualitativeVar(si),
					Series: s.Name, Cat: in.Categories[c], Value: v,
				})
			}
		} else {
			bw := catW * 0.7
			bx := cx + (catW-bw)/2
			cursor := plot.Bottom()
			for si, s := range in.Series {
				v := 0.0
				if c < len(s.Values) {
					v = s.Values[c]
				}
				hgt := v / max * plot.H
				top := cursor - hgt
				lay.Rects = append(lay.Rects, barRect{
					Box:    Box{X: bx, Y: top, W: bw, H: hgt},
					Color:  QualitativeVar(si),
					Series: s.Name, Cat: in.Categories[c], Value: v,
				})
				cursor = top
			}
		}
	}
	for _, t := range niceTicks(0, max, 5) {
		lay.YTicks = append(lay.YTicks, axisTick{Pos: sy(t), Label: fnum(t)})
	}
	return lay
}

// StackedBarSVG renders the stacked/grouped bar chart described by lay.
func StackedBarSVG(w io.Writer, lay StackedBarLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, t := range lay.YTicks {
		s.line(lay.Plot.X, t.Pos, lay.Plot.Right(), t.Pos,
			"var(--chart-grid, currentColor)",
			`stroke-width="1" stroke-opacity="0.15" vector-effect="non-scaling-stroke"`)
		s.text(lay.Plot.X-6, t.Pos+3, "end", "chart-axis", t.Label)
	}
	for _, r := range lay.Rects {
		s.p(`<g>`)
		s.title(r.Series + " / " + r.Cat + ": " + fnum(r.Value))
		s.rect(r.Box, r.Color, `rx="1"`)
		s.p(`</g>`)
	}
	for _, t := range lay.XLabels {
		s.text(t.Pos, lay.Plot.Bottom()+16, "middle", "chart-axis", t.Label)
	}
	s.close()
	return s.Err()
}

// BulletInput configures a bullet chart: a measure bar against a target and
// qualitative ranges.
type BulletInput struct {
	Title   string
	Label   string
	Measure float64
	Target  float64
	Ranges  []float64 // ascending range thresholds (qualitative bands)
	Max     float64   // 0 = auto
	Unit    string
	Width   float64 // defaults to 360
	Height  float64 // defaults to 48
}

// BulletLayout is the geometry for a bullet chart.
type BulletLayout struct {
	W, H    float64
	Title   string
	Label   string
	Bands   []barRect
	Measure Box
	TargetX float64
	HasTgt  bool
	ValTxt  string
}

// LayoutBullet computes geometry for a bullet chart.
func LayoutBullet(in BulletInput) BulletLayout {
	w := in.Width
	if w == 0 {
		w = 360
	}
	h := in.Height
	if h == 0 {
		h = 48
	}
	max := in.Max
	if max <= 0 {
		max = math.Max(in.Measure, in.Target)
		for _, r := range in.Ranges {
			max = math.Max(max, r)
		}
	}
	if max <= 0 {
		max = 1
	}
	trackX := 0.0
	if in.Label != "" {
		trackX = 96
	}
	trackW := w - trackX
	sx := func(v float64) float64 { return trackX + v/max*trackW }

	lay := BulletLayout{W: w, H: h, Title: in.Title, Label: in.Label,
		ValTxt: fnum(in.Measure) + in.Unit}
	// Qualitative bands, lightest (diverging) first.
	prev := 0.0
	for i, r := range in.Ranges {
		lay.Bands = append(lay.Bands, barRect{
			Box:   Box{X: sx(prev), Y: 8, W: sx(r) - sx(prev), H: h - 16},
			Color: DivergingVar(i, len(in.Ranges)),
		})
		prev = r
	}
	mh := (h - 16) * 0.4
	lay.Measure = Box{X: trackX, Y: (h-mh)/2 + 0, W: sx(in.Measure) - trackX, H: mh}
	if in.Target > 0 {
		lay.TargetX = sx(in.Target)
		lay.HasTgt = true
	}
	return lay
}

// BulletSVG renders the bullet chart described by lay.
func BulletSVG(w io.Writer, lay BulletLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	if lay.Label != "" {
		s.text(88, lay.H/2+4, "end", "chart-label", lay.Label)
	}
	for _, b := range lay.Bands {
		s.rect(b.Box, b.Color, `fill-opacity="0.35"`)
	}
	s.p(`<g>`)
	s.title(lay.Label + ": " + lay.ValTxt)
	s.rect(lay.Measure, QualitativeVar(0), `rx="1"`)
	s.p(`</g>`)
	if lay.HasTgt {
		s.line(lay.TargetX, 6, lay.TargetX, lay.H-6,
			"var(--chart-emphasis, currentColor)",
			`stroke-width="2" vector-effect="non-scaling-stroke"`)
	}
	s.close()
	return s.Err()
}
