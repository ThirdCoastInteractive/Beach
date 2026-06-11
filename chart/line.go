package chart

import (
	"io"
	"math"
)

// Point is one (x,y) datum on a continuous plane.
type Point struct {
	X, Y float64
}

// XYSeries is one named series of points for a line/area/scatter chart.
type XYSeries struct {
	Name   string
	Points []Point
}

// LineInput configures a line or area chart.
type LineInput struct {
	Series []XYSeries
	Title  string
	Width  float64 // defaults to 640
	Height float64 // defaults to 360
	Area   bool    // fill under each line
	// Optional fixed axis bounds; zero values mean auto from data.
	MinX, MaxX, MinY, MaxY float64
}

// LineLayout is the computed geometry for a line/area chart.
type LineLayout struct {
	W, H   float64
	Title  string
	Area   bool
	Plot   Box
	Series []lineSeriesGeom
	XTicks []axisTick
	YTicks []axisTick
	xmin   float64
	xmax   float64
	ymin   float64
	ymax   float64
}

type lineSeriesGeom struct {
	Name  string
	Color string
	Pts   [][2]float64 // pixel coordinates
}

type axisTick struct {
	Pos   float64 // pixel coordinate along the axis
	Label string
}

// LayoutLine computes geometry for a line (or area) chart.
func LayoutLine(in LineInput) LineLayout {
	w := in.Width
	if w == 0 {
		w = 640
	}
	h := in.Height
	if h == 0 {
		h = 360
	}
	root := Box{W: w, H: h}
	// Margins leave room for axis labels.
	plot := Box{X: 48, Y: 16, W: w - 64, H: h - 40}

	xmin, xmax := in.MinX, in.MaxX
	ymin, ymax := in.MinY, in.MaxY
	if xmin == 0 && xmax == 0 && ymin == 0 && ymax == 0 {
		xmin, xmax, ymin, ymax = dataBounds(in.Series)
	}
	if xmax <= xmin {
		xmax = xmin + 1
	}
	if ymax <= ymin {
		ymax = ymin + 1
	}

	lay := LineLayout{
		W: root.W, H: root.H, Title: in.Title, Area: in.Area, Plot: plot,
		xmin: xmin, xmax: xmax, ymin: ymin, ymax: ymax,
	}
	sx := func(x float64) float64 {
		return plot.X + (x-xmin)/(xmax-xmin)*plot.W
	}
	sy := func(y float64) float64 {
		return plot.Bottom() - (y-ymin)/(ymax-ymin)*plot.H
	}
	for i, s := range in.Series {
		g := lineSeriesGeom{Name: s.Name, Color: QualitativeVar(i)}
		for _, p := range s.Points {
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

func dataBounds(series []XYSeries) (xmin, xmax, ymin, ymax float64) {
	xmin, ymin = math.Inf(1), math.Inf(1)
	xmax, ymax = math.Inf(-1), math.Inf(-1)
	any := false
	for _, s := range series {
		for _, p := range s.Points {
			any = true
			xmin = math.Min(xmin, p.X)
			xmax = math.Max(xmax, p.X)
			ymin = math.Min(ymin, p.Y)
			ymax = math.Max(ymax, p.Y)
		}
	}
	if !any {
		return 0, 1, 0, 1
	}
	if ymin > 0 {
		ymin = 0 // baseline at zero reads more honestly for areas/lines
	}
	return xmin, xmax, ymin, ymax
}

// LineSVG renders the line/area chart described by lay.
func LineSVG(w io.Writer, lay LineLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	drawGrid(s, lay.Plot, lay.XTicks, lay.YTicks)
	for _, g := range lay.Series {
		if lay.Area && len(g.Pts) > 0 {
			area := make([][2]float64, 0, len(g.Pts)+2)
			area = append(area, [2]float64{g.Pts[0][0], lay.Plot.Bottom()})
			area = append(area, g.Pts...)
			area = append(area, [2]float64{g.Pts[len(g.Pts)-1][0], lay.Plot.Bottom()})
			s.p(`<path d="`, polyPath(area, true), `" fill="`, g.Color,
				`" fill-opacity="0.15" stroke="none"/>`)
		}
		s.p(`<g>`)
		if g.Name != "" {
			s.title(g.Name)
		}
		s.p(`<path d="`, polyPath(g.Pts, false),
			`" fill="none" stroke="`, g.Color,
			`" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke"/>`)
		s.p(`</g>`)
	}
	s.close()
	return s.Err()
}

// AreaSVG renders lay as a filled area chart regardless of lay.Area.
func AreaSVG(w io.Writer, lay LineLayout) error {
	lay.Area = true
	return LineSVG(w, lay)
}

func drawGrid(s *svg, plot Box, xticks, yticks []axisTick) {
	for _, t := range yticks {
		s.line(plot.X, t.Pos, plot.Right(), t.Pos,
			"var(--chart-grid, currentColor)",
			`stroke-width="1" stroke-opacity="0.15" vector-effect="non-scaling-stroke"`)
		s.text(plot.X-6, t.Pos+3, "end", "chart-axis", t.Label)
	}
	for _, t := range xticks {
		s.text(t.Pos, plot.Bottom()+16, "middle", "chart-axis", t.Label)
	}
}
