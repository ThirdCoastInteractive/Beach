package chart

import "io"

// ScatterInput configures a scatter plot. It reuses XYSeries; each series gets
// its own qualitative color.
type ScatterInput struct {
	Series []XYSeries
	Title  string
	Width  float64 // defaults to 480
	Height float64 // defaults to 360
	Radius float64 // point radius; defaults to 3
}

// ScatterLayout is the geometry for a scatter plot.
type ScatterLayout struct {
	W, H   float64
	Title  string
	Plot   Box
	Radius float64
	Series []scatterSeries
	XTicks []axisTick
	YTicks []axisTick
}

type scatterSeries struct {
	Name  string
	Color string
	Pts   [][2]float64
}

// LayoutScatter computes geometry for a scatter plot.
func LayoutScatter(in ScatterInput) ScatterLayout {
	w := in.Width
	if w == 0 {
		w = 480
	}
	h := in.Height
	if h == 0 {
		h = 360
	}
	rad := in.Radius
	if rad == 0 {
		rad = 3
	}
	plot := Box{X: 48, Y: 16, W: w - 64, H: h - 40}
	xmin, xmax, ymin, ymax := dataBounds(in.Series)
	if xmax <= xmin {
		xmax = xmin + 1
	}
	if ymax <= ymin {
		ymax = ymin + 1
	}
	sx := func(x float64) float64 { return plot.X + (x-xmin)/(xmax-xmin)*plot.W }
	sy := func(y float64) float64 { return plot.Bottom() - (y-ymin)/(ymax-ymin)*plot.H }

	lay := ScatterLayout{W: w, H: h, Title: in.Title, Plot: plot, Radius: rad}
	for i, ser := range in.Series {
		g := scatterSeries{Name: ser.Name, Color: QualitativeVar(i)}
		for _, p := range ser.Points {
			g.Pts = append(g.Pts, [2]float64{sx(p.X), sy(p.Y)})
		}
		lay.Series = append(lay.Series, g)
	}
	for _, t := range niceTicks(ymin, ymax, 5) {
		lay.YTicks = append(lay.YTicks, axisTick{Pos: sy(t), Label: fnum(t)})
	}
	for _, t := range niceTicks(xmin, xmax, 6) {
		lay.XTicks = append(lay.XTicks, axisTick{Pos: sx(t), Label: fnum(t)})
	}
	return lay
}

// ScatterSVG renders the scatter plot described by lay.
func ScatterSVG(w io.Writer, lay ScatterLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	drawGrid(s, lay.Plot, lay.XTicks, lay.YTicks)
	for _, g := range lay.Series {
		s.p(`<g fill="`, g.Color, `">`)
		if g.Name != "" {
			s.title(g.Name)
		}
		for _, p := range g.Pts {
			s.circle(p[0], p[1], lay.Radius, g.Color, `fill-opacity="0.7"`)
		}
		s.p(`</g>`)
	}
	s.close()
	return s.Err()
}
