package chart

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

// golden compares rendered output against testdata/<name>.svg, writing it when
// -update is passed.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".svg")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	if !bytes.Equal(normalize(want), normalize(got)) {
		t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
			name, got, want)
	}
}

func normalize(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// render runs an SVG func and returns the bytes, failing on error.
func render(t *testing.T, fn func(*bytes.Buffer) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := fn(&buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.Bytes()
}

func TestLine(t *testing.T) {
	lay := LayoutLine(LineInput{
		Title: "Throughput",
		Series: []XYSeries{
			{Name: "A", Points: []Point{{0, 1}, {1, 4}, {2, 2}, {3, 5}}},
			{Name: "B", Points: []Point{{0, 2}, {1, 1}, {2, 3}, {3, 2}}},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return LineSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "line", out)
}

func TestArea(t *testing.T) {
	lay := LayoutLine(LineInput{
		Title:  "Area",
		Series: []XYSeries{{Name: "A", Points: []Point{{0, 1}, {1, 3}, {2, 2}, {3, 4}}}},
	})
	out := render(t, func(b *bytes.Buffer) error { return AreaSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "area", out)
}

func TestSparkline(t *testing.T) {
	lay := LayoutSparkline(SparkInput{Values: []float64{3, 1, 4, 1, 5, 9, 2, 6}, Title: "spark"})
	out := render(t, func(b *bytes.Buffer) error { return SparklineSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "sparkline", out)
}

func TestHBar(t *testing.T) {
	lay := LayoutHBar(HBarInput{
		Title: "Power",
		Unit:  " kW",
		Series: []Datum{
			{"Rack A", 4.2}, {"Rack B", 7.8}, {"Rack C", 2.1},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return HBarSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "hbar", out)
}

func TestStackedBar(t *testing.T) {
	lay := LayoutStackedBar(StackedBarInput{
		Title:      "Stacked",
		Categories: []string{"Q1", "Q2", "Q3"},
		Series: []Datum2{
			{Name: "X", Values: []float64{3, 5, 2}},
			{Name: "Y", Values: []float64{1, 2, 4}},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return StackedBarSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "stackedbar", out)
}

func TestGroupedBar(t *testing.T) {
	lay := LayoutStackedBar(StackedBarInput{
		Title:      "Grouped",
		Grouped:    true,
		Categories: []string{"Q1", "Q2"},
		Series: []Datum2{
			{Name: "X", Values: []float64{3, 5}},
			{Name: "Y", Values: []float64{1, 2}},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return StackedBarSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "groupedbar", out)
}

func TestBullet(t *testing.T) {
	lay := LayoutBullet(BulletInput{
		Title: "SLA", Label: "Uptime", Measure: 92, Target: 95,
		Ranges: []float64{60, 80, 100}, Unit: "%",
	})
	out := render(t, func(b *bytes.Buffer) error { return BulletSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "bullet", out)
}

func TestGauge(t *testing.T) {
	lay := LayoutGauge(GaugeInput{Title: "CPU", Label: "CPU", Value: 72, Max: 100, Unit: "%"})
	out := render(t, func(b *bytes.Buffer) error { return GaugeSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "gauge", out)
}

func TestStackedGauge(t *testing.T) {
	lay := LayoutStackedGauge(StackedGaugeInput{
		Title: "Rings",
		Series: []Datum{
			{"CPU", 72}, {"Mem", 55}, {"Disk", 30},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return StackedGaugeSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "stackedgauge", out)
}

func TestBillboard(t *testing.T) {
	lay := LayoutBillboard(BillboardInput{
		Title: "Users", Label: "Active users", Value: "12,402",
		Delta: "4.2%", Trend: TrendUp,
	})
	out := render(t, func(b *bytes.Buffer) error { return BillboardSVG(b, lay) })
	assertValidSVG(t, out)
	if !strings.Contains(string(out), "↑") {
		t.Error("billboard up trend should include an arrow glyph, not color alone")
	}
	golden(t, "billboard", out)
}

func TestScatter(t *testing.T) {
	lay := LayoutScatter(ScatterInput{
		Title: "Scatter",
		Series: []XYSeries{
			{Name: "A", Points: []Point{{1, 2}, {3, 4}, {5, 1}, {2, 5}}},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return ScatterSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "scatter", out)
}

func TestBox(t *testing.T) {
	lay := LayoutBoxPlot(BoxPlotInput{
		Title: "Latency",
		Boxes: []BoxStats{
			{Label: "p50", Min: 1, Q1: 3, Median: 5, Q3: 7, Max: 9},
			{Label: "p99", Min: 2, Q1: 4, Median: 8, Q3: 12, Max: 20},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return BoxPlotSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "boxplot", out)
}

func TestHeatmap(t *testing.T) {
	lay := LayoutHeatmap(HeatmapInput{
		Title: "Heat",
		Cols:  []string{"Mon", "Tue", "Wed"},
		Rows:  []string{"AM", "PM"},
		Values: [][]float64{
			{1, 5, 3},
			{8, 2, 6},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return HeatmapSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "heatmap", out)
}

func TestCalendar(t *testing.T) {
	lay := LayoutCalendarHeatmap(CalendarInput{
		Title: "Activity",
		Year:  2025,
		Days: map[string]float64{
			"2025-01-01": 3, "2025-01-02": 5, "2025-06-15": 9, "2025-12-31": 1,
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return CalendarHeatmapSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "calendar", out)
}

func TestStreamgraph(t *testing.T) {
	lay := LayoutStreamgraph(StreamgraphInput{
		Title: "Stream",
		Series: []Datum2{
			{Name: "A", Values: []float64{1, 3, 2, 4}},
			{Name: "B", Values: []float64{2, 1, 3, 2}},
			{Name: "C", Values: []float64{1, 2, 1, 3}},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return StreamgraphSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "streamgraph", out)
}

func TestSankey(t *testing.T) {
	lay := LayoutSankey(SankeyInput{
		Title: "Flow",
		Nodes: []string{"A", "B", "C", "D"},
		Links: []SankeyLink{
			{Source: 0, Target: 2, Value: 5},
			{Source: 0, Target: 3, Value: 3},
			{Source: 1, Target: 2, Value: 2},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return SankeySVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "sankey", out)
}

func TestChord(t *testing.T) {
	lay := LayoutChord(ChordInput{
		Title:  "Chord",
		Labels: []string{"A", "B", "C"},
		Matrix: [][]float64{
			{0, 3, 2},
			{3, 0, 1},
			{2, 1, 0},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return ChordSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "chord", out)
}

func TestBollinger(t *testing.T) {
	lay := LayoutBollinger(BollingerInput{
		Title:  "BB",
		Values: []float64{10, 11, 12, 11, 13, 14, 13, 15, 16, 15, 17, 18, 16, 19, 20, 18, 21, 22, 20, 23},
		Window: 5,
	})
	out := render(t, func(b *bytes.Buffer) error { return BollingerSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "bollinger", out)
}

func TestTimeline(t *testing.T) {
	lay := LayoutTimeline(TimelineInput{
		Title: "Roadmap",
		Spans: []TimelineSpan{
			{Label: "Design", Start: 0, End: 3, Row: 0},
			{Label: "Build", Start: 2, End: 8, Row: 1},
			{Label: "Ship", Start: 8, End: 10, Row: 0},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return TimelineSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "timeline", out)
}

func TestBarRace(t *testing.T) {
	lay := LayoutBarRace(BarRaceInput{
		Title: "Race",
		Frame: 0,
		Bars: []Datum{
			{"Alpha", 80}, {"Bravo", 65}, {"Charlie", 95}, {"Delta", 40},
		},
	})
	out := render(t, func(b *bytes.Buffer) error { return BarRaceSVG(b, lay) })
	assertValidSVG(t, out)
	golden(t, "barrace", out)
}

// assertValidSVG performs lightweight structural checks every chart must pass.
func assertValidSVG(t *testing.T, out []byte) {
	t.Helper()
	s := string(out)
	if !strings.HasPrefix(s, "<svg") {
		t.Error("output must start with <svg")
	}
	if !strings.HasSuffix(strings.TrimSpace(s), "</svg>") {
		t.Error("output must end with </svg>")
	}
	if !strings.Contains(s, `role="img"`) {
		t.Error("root must have role=\"img\"")
	}
	if strings.Contains(s, "#") && strings.Contains(s, `fill="#`) {
		t.Error("no hardcoded hex colors allowed; use CSS variables")
	}
	// balance angle brackets crudely: every < should have a matching >
	if strings.Count(s, "<") != strings.Count(s, ">") {
		t.Error("unbalanced angle brackets in SVG")
	}
}
