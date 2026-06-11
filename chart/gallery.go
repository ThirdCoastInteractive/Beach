package chart

import (
	"bytes"
	"html/template"
	"io"
)

// GalleryItem is one rendered chart in the gallery: a human title plus its
// final SVG markup, safe to embed directly in a page.
type GalleryItem struct {
	Title string
	SVG   template.HTML
}

// GallerySVGs renders one representative example of every chart type the
// package supports, each with sample data and the design system's
// --chart-qualitative-* series colors. The returned slice has one entry per
// type that renders; a type is omitted only if its layout/render pair returns
// an error or produces no SVG.
//
// All 18 supported types currently render: line, area, sparkline, horizontal
// bar, stacked bar, grouped bar, bullet, single gauge, stacked gauge,
// billboard, scatter, box plot, calendar heatmap, grid heatmap, streamgraph,
// sankey, chord, bollinger, timeline, and bar race.
func GallerySVGs() []GalleryItem {
	var out []GalleryItem
	add := func(title string, render func(io.Writer) error) {
		var buf bytes.Buffer
		if err := render(&buf); err != nil {
			return // skip cleanly: type produced an error
		}
		if buf.Len() == 0 {
			return
		}
		out = append(out, GalleryItem{Title: title, SVG: template.HTML(buf.String())})
	}

	// Sample data shared across a few line-shaped charts.
	wave := []float64{12, 15, 14, 18, 22, 21, 26, 24, 30, 28, 33, 31, 36, 40, 38, 44}

	// 1. Line.
	add("Line", func(w io.Writer) error {
		return LineSVG(w, LayoutLine(LineInput{
			Title: "Throughput",
			Series: []XYSeries{
				{Name: "us-east", Points: linePts([]float64{4, 8, 6, 12, 14, 11, 18, 22})}},
		}))
	})

	// 2. Area (multi-series, filled).
	add("Area", func(w io.Writer) error {
		return AreaSVG(w, LayoutLine(LineInput{
			Title: "Sessions",
			Area:  true,
			Series: []XYSeries{
				{Name: "web", Points: linePts([]float64{3, 5, 4, 9, 11, 10, 15, 19})},
				{Name: "mobile", Points: linePts([]float64{1, 2, 4, 3, 6, 8, 7, 12})},
			},
		}))
	})

	// 3. Sparkline.
	add("Sparkline", func(w io.Writer) error {
		return SparklineSVG(w, LayoutSparkline(SparkInput{
			Title: "Latency", Values: wave, Area: true,
		}))
	})

	// 4. Horizontal bar.
	add("Horizontal bar", func(w io.Writer) error {
		return HBarSVG(w, LayoutHBar(HBarInput{
			Title: "Rack draw", Unit: " kW",
			Series: []Datum{
				{Label: "Rack A1", Value: 6.4},
				{Label: "Rack A2", Value: 4.1},
				{Label: "Rack B1", Value: 7.8},
				{Label: "Rack B2", Value: 3.2},
			},
		}))
	})

	// 5. Stacked bar.
	add("Stacked bar", func(w io.Writer) error {
		return StackedBarSVG(w, LayoutStackedBar(StackedBarInput{
			Title:      "Energy mix",
			Categories: []string{"Q1", "Q2", "Q3", "Q4"},
			Series: []Datum2{
				{Name: "grid", Values: []float64{40, 38, 42, 45}},
				{Name: "solar", Values: []float64{12, 18, 24, 16}},
				{Name: "battery", Values: []float64{6, 8, 7, 10}},
			},
		}))
	})

	// 6. Grouped bar.
	add("Grouped bar", func(w io.Writer) error {
		return StackedBarSVG(w, LayoutStackedBar(StackedBarInput{
			Title:      "Reads vs writes",
			Grouped:    true,
			Categories: []string{"Mon", "Tue", "Wed", "Thu", "Fri"},
			Series: []Datum2{
				{Name: "reads", Values: []float64{120, 145, 138, 160, 150}},
				{Name: "writes", Values: []float64{60, 72, 55, 81, 64}},
			},
		}))
	})

	// 7. Bullet.
	add("Bullet", func(w io.Writer) error {
		return BulletSVG(w, LayoutBullet(BulletInput{
			Title: "SLA", Label: "Uptime", Unit: "%",
			Measure: 99.4, Target: 99.9, Max: 100,
			Ranges: []float64{95, 99, 100},
		}))
	})

	// 8. Single gauge.
	add("Gauge", func(w io.Writer) error {
		return GaugeSVG(w, LayoutGauge(GaugeInput{
			Title: "CPU", Label: "CPU", Value: 68, Min: 0, Max: 100, Unit: "%",
		}))
	})

	// 9. Stacked (concentric) gauge.
	add("Stacked gauge", func(w io.Writer) error {
		return StackedGaugeSVG(w, LayoutStackedGauge(StackedGaugeInput{
			Title: "Capacity", Max: 100,
			Series: []Datum{
				{Label: "CPU", Value: 68},
				{Label: "Memory", Value: 52},
				{Label: "Disk", Value: 81},
			},
		}))
	})

	// 10. Billboard.
	add("Billboard", func(w io.Writer) error {
		return BillboardSVG(w, LayoutBillboard(BillboardInput{
			Title: "Active users", Label: "Active users",
			Value: "12,402", Delta: "4.2%", Trend: TrendUp,
		}))
	})

	// 11. Scatter.
	add("Scatter", func(w io.Writer) error {
		return ScatterSVG(w, LayoutScatter(ScatterInput{
			Title: "Load vs latency",
			Series: []XYSeries{
				{Name: "node", Points: []Point{
					{X: 1, Y: 12}, {X: 2, Y: 18}, {X: 3, Y: 15}, {X: 4, Y: 26},
					{X: 5, Y: 24}, {X: 6, Y: 33}, {X: 7, Y: 30}, {X: 8, Y: 41},
				}},
			},
		}))
	})

	// 12. Box plot.
	add("Box plot", func(w io.Writer) error {
		return BoxPlotSVG(w, LayoutBoxPlot(BoxPlotInput{
			Title: "Response time by region",
			Boxes: []BoxStats{
				{Label: "us", Min: 8, Q1: 14, Median: 20, Q3: 28, Max: 40},
				{Label: "eu", Min: 12, Q1: 18, Median: 24, Q3: 33, Max: 48},
				{Label: "ap", Min: 20, Q1: 30, Median: 38, Q3: 50, Max: 70},
			},
		}))
	})

	// 13. Calendar heatmap.
	add("Calendar heatmap", func(w io.Writer) error {
		return CalendarHeatmapSVG(w, LayoutCalendarHeatmap(CalendarInput{
			Title: "Commits", Year: 2025, Days: calendarSample(2025),
		}))
	})

	// 14. Grid heatmap.
	add("Heatmap", func(w io.Writer) error {
		return HeatmapSVG(w, LayoutHeatmap(HeatmapInput{
			Title: "Hourly load",
			Cols:  []string{"00", "06", "12", "18"},
			Rows:  []string{"Mon", "Tue", "Wed", "Thu", "Fri"},
			Values: [][]float64{
				{2, 6, 9, 4},
				{3, 7, 8, 5},
				{1, 5, 10, 6},
				{4, 8, 7, 3},
				{2, 9, 6, 5},
			},
		}))
	})

	// 15. Streamgraph.
	add("Streamgraph", func(w io.Writer) error {
		return StreamgraphSVG(w, LayoutStreamgraph(StreamgraphInput{
			Title: "Traffic by source",
			Series: []Datum2{
				{Name: "search", Values: []float64{4, 6, 5, 9, 12, 10, 14, 13}},
				{Name: "social", Values: []float64{2, 3, 5, 4, 7, 9, 8, 11}},
				{Name: "direct", Values: []float64{6, 5, 7, 6, 8, 7, 9, 10}},
			},
		}))
	})

	// 16. Sankey.
	add("Sankey", func(w io.Writer) error {
		return SankeySVG(w, LayoutSankey(SankeyInput{
			Title: "Energy flow",
			Nodes: []string{"Grid", "Solar", "Bus", "Servers", "Cooling"},
			Links: []SankeyLink{
				{Source: 0, Target: 2, Value: 40},
				{Source: 1, Target: 2, Value: 20},
				{Source: 2, Target: 3, Value: 38},
				{Source: 2, Target: 4, Value: 22},
			},
		}))
	})

	// 17. Chord.
	add("Chord", func(w io.Writer) error {
		return ChordSVG(w, LayoutChord(ChordInput{
			Title:  "Cross traffic",
			Labels: []string{"A", "B", "C", "D"},
			Matrix: [][]float64{
				{0, 5, 3, 2},
				{5, 0, 4, 1},
				{3, 4, 0, 6},
				{2, 1, 6, 0},
			},
		}))
	})

	// 18. Bollinger.
	add("Bollinger", func(w io.Writer) error {
		return BollingerSVG(w, LayoutBollinger(BollingerInput{
			Title: "Price", Window: 5, K: 2, Values: bollingerSample(),
		}))
	})

	// 19. Timeline.
	add("Timeline", func(w io.Writer) error {
		return TimelineSVG(w, LayoutTimeline(TimelineInput{
			Title: "Release plan",
			Spans: []TimelineSpan{
				{Label: "Design", Start: 0, End: 10, Row: 0},
				{Label: "Build", Start: 8, End: 28, Row: 1},
				{Label: "Test", Start: 24, End: 36, Row: 2},
				{Label: "Ship", Start: 34, End: 40, Row: 3},
			},
		}))
	})

	// 20. Bar race (one frame).
	add("Bar race", func(w io.Writer) error {
		return BarRaceSVG(w, LayoutBarRace(BarRaceInput{
			Title: "Top services", Top: 5,
			Bars: []Datum{
				{Label: "api", Value: 820},
				{Label: "web", Value: 640},
				{Label: "auth", Value: 410},
				{Label: "cache", Value: 305},
				{Label: "queue", Value: 190},
			},
		}))
	})

	return out
}

// linePts converts a slice of y-values into evenly x-spaced Points.
func linePts(ys []float64) []Point {
	pts := make([]Point, len(ys))
	for i, y := range ys {
		pts[i] = Point{X: float64(i), Y: y}
	}
	return pts
}

// calendarSample produces a sparse, deterministic set of daily values for the
// given year so the calendar heatmap has something to shade.
func calendarSample(year int) map[string]float64 {
	days := map[string]float64{}
	// Deterministic pseudo-pattern by ordinal day; no randomness for stable output.
	for d := 0; d < 365; d++ {
		v := float64((d*7+3)%11) - 2
		if v <= 0 {
			continue
		}
		key := ordinalDate(year, d)
		days[key] = v
	}
	return days
}

// ordinalDate returns the YYYY-MM-DD key for the d-th day (0-based) of year.
func ordinalDate(year, d int) string {
	mdays := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		mdays[1] = 29
	}
	month := 1
	for d >= mdays[month-1] {
		d -= mdays[month-1]
		month++
	}
	day := d + 1
	return itoa2(year) + "-" + pad2(month) + "-" + pad2(day)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa2(n)
	}
	return itoa2(n)
}

func itoa2(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// bollingerSample is a deterministic oscillating price series.
func bollingerSample() []float64 {
	vals := make([]float64, 40)
	base := 100.0
	for i := range vals {
		// Smooth-ish deterministic walk: blend two cycles.
		osc := float64((i*13+7)%17) - 8
		base += osc * 0.4
		vals[i] = base
	}
	return vals
}
