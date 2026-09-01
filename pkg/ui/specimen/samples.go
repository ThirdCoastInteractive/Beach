// Package specimen is the framework's living showcase: one (intentionally
// over-budget) page that renders the token sheet, the icon set, every
// driftwood component in its states, the rybitten gamut strips, and every
// chart type with representative data. It is the visual contract for the
// view layer — mount it at /specimen and look.
//
// This file holds the sample data: deterministic, beach-flavored inputs for
// every chart layout, plus the token/icon/gamut lists the page iterates.
// Nothing here is random — the specimen must render identically on every
// load so visual regressions are real regressions.
package specimen

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/beach/view"
	"github.com/ThirdCoastInteractive/Beach/pkg/chart"
	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/Beach/pkg/rybitten"
	"github.com/ThirdCoastInteractive/Beach/pkg/theme"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"
	"github.com/a-h/templ"
)

// --- token sheet --------------------------------------------------------------

// tokenSwatch is one named token with the role it plays.
type tokenSwatch struct {
	Var  string // CSS custom property, e.g. "--color-paper"
	Note string // what the token is for
}

// surfaceTokens are the surface/text/line tokens, in paint order: page, panel,
// then the inks and lines that sit on them.
var surfaceTokens = []tokenSwatch{
	{"--color-paper", "page background"},
	{"--color-panel", "card background"},
	{"--color-panel-hover", "card hover"},
	{"--color-fg-default", "default ink"},
	{"--color-fg-muted", "muted ink"},
	{"--color-line-soft", "soft line"},
	{"--color-line-strong", "strong line"},
	{"--color-accent", "accent"},
}

// semanticTokens are the four status colors shared by components and charts.
var semanticTokens = []tokenSwatch{
	{"--color-good", "good"},
	{"--color-warn", "warn"},
	{"--color-bad", "bad"},
	{"--color-info", "info"},
}

// fontTokens are the two type tokens; the specimen renders a line of text in
// each so the faces themselves are on the sheet.
var fontTokens = []tokenSwatch{
	{"--font-sans", "interface text"},
	{"--font-mono", "data, labels, code"},
}

// seriesTokens returns the 15 chart series tokens --color-series-a…o.
func seriesTokens() []string {
	out := make([]string, 0, 15)
	for c := 'a'; c <= 'o'; c++ {
		out = append(out, "--color-series-"+string(c))
	}
	return out
}

// varBG fills a swatch with a token. The token name is the per-instance datum
// being demonstrated, so an inline style is the honest encoding. Values come
// from the static lists above, never user input, hence SafeCSS.
func varBG(cssVar string) templ.SafeCSS {
	return templ.SafeCSS("background:var(" + cssVar + ")")
}

// hexBG fills a swatch with a literal color — used only for the rybitten gamut
// strips, where the computed hex IS the content on display.
func hexBG(hex string) templ.SafeCSS {
	return templ.SafeCSS("background:" + hex)
}

// fontStyle samples a font token on a line of text.
func fontStyle(cssVar string) templ.SafeCSS {
	return templ.SafeCSS("font-family:var(" + cssVar + ")")
}

// --- media ----------------------------------------------------------------------

// sampleImageSrc is a tiny inline SVG beach scene, so the media components show
// real pixels without the specimen depending on a static asset that may not be
// mounted. The colors are image content, not theme.
const sampleImageSrc = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 160 90'%3E%3Crect width='160' height='90' fill='%23355f6e'/%3E%3Ccircle cx='120' cy='22' r='12' fill='%23e5c151'/%3E%3Cpath d='M0 64 Q40 52 80 64 T160 64 V90 H0 Z' fill='%234091ac'/%3E%3Cpath d='M0 78 Q40 70 80 78 T160 78 V90 H0 Z' fill='%23c9a36a'/%3E%3C/svg%3E"

// samplePosterSrc is the video poster: the same inline-SVG trick, so the Video
// demo shows a real reserved frame with something in it and still fetches
// nothing. That is the component's own claim being demonstrated rather than
// asserted.
const samplePosterSrc = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 160 90'%3E%3Crect width='160' height='90' fill='%231c2b30'/%3E%3Cpath d='M0 70 Q40 58 80 70 T160 70 V90 H0 Z' fill='%233d7c82'/%3E%3Cpolygon points='72,38 72,58 92,48' fill='%23f4f4f5'/%3E%3C/svg%3E"

// sampleTrackSrc is a real two-cue caption file, served from the framework's own
// static tree.
//
// It is not inline like the poster beside it, for a reason worth writing down:
// the framework's CSP sets media-src 'self', so a data: URI is blocked — and a
// track marked default is the one thing on a <video preload="none"> the browser
// actually fetches, so a fictional path 404s on every page that mounts the
// specimen. A caption demo whose captions never load demonstrates the opposite
// of the point.
const sampleTrackSrc = "/static/media/specimen.en.vtt"

// --- icon set -----------------------------------------------------------------

// iconNames is every glyph the framework and examples currently reference.
// ui.Icon records each render for the font subsetter, so showing the set here
// also guarantees the specimen build ships every glyph.
var iconNames = []string{
	"alert-octagon", "bold", "box", "calendar", "chart", "check", "chevron-down",
	"chevron-up-down", "code", "gear", "heading", "home", "image", "inbox",
	"italic", "key", "link", "list", "list-ol", "mail", "pencil", "pin", "play",
	"plus", "quote", "refresh", "search", "send", "skip-forward", "spinner",
	"trash", "user", "video", "x",
}

// --- gamut preview -------------------------------------------------------------

// previewGamuts is the handful of rybitten gamuts the specimen strips: the live
// default first (munsell), then a spread from historical to synthetic.
var previewGamuts = []string{
	"munsell", "itten", "goethe", "harris", "albers", "apple90s", "pixelart", "cmy",
}

// gamutMeta returns the gamut and its 15-swatch qualitative palette as hexes —
// the same colors SeriesVars would emit as --color-series-* for that gamut.
func gamutMeta(key string) (rybitten.Gamut, []string) {
	g := rybitten.Cubes[key]
	return g, rybitten.Hexes(rybitten.Series(g, rybitten.SeriesCount))
}

// gamutProvenance formats a gamut's source line for the strip caption.
func gamutProvenance(g rybitten.Gamut) string {
	return fmt.Sprintf("%s — %s, %d", g.Title, g.Author, g.Year)
}

// seriesVarsCSS is the :root block the build would emit for the default gamut,
// shown verbatim so the token↔gamut pipeline is visible on the page.
func seriesVarsCSS() string {
	return rybitten.SeriesVars(rybitten.Cubes["munsell"])
}

// --- shared series helpers ------------------------------------------------------

// wave builds a deterministic synthetic series: base + two sine terms + drift.
// Every chart sample leans on it so the specimen never changes between loads.
func wave(n int, base, amp, freq, drift float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		x := float64(i)
		out[i] = base + amp*math.Sin(freq*x) + amp*0.35*math.Sin(freq*2.7*x+1.3) + drift*x
	}
	return out
}

// hours returns n hourly labels "00:00", "01:00", …
func hours(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%02d:00", i%24)
	}
	return out
}

var monthLabels = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// --- bar family -----------------------------------------------------------------

func sampleHBar() chart.BarChart {
	return chart.LayoutHBar(chart.HBarSeries{
		Unit: " visits",
		Series: []chart.HBarEntry{
			{Label: "North Beach", Value: 9420, Max: 10000},
			{Label: "The Cove", Value: 7180, Max: 9000},
			{Label: "Pier South", Value: 5660, Max: 8000},
			{Label: "Dunes", Value: 4110, Max: 8000},
			{Label: "Driftwood Flats", Value: 2870, Max: 6000},
			{Label: "Gull Point", Value: 1540, Max: 6000},
		},
	})
}

func sampleStackedBar() chart.StackedBar {
	seg := func(label string, v int) chart.StackedBarSegment {
		return chart.StackedBarSegment{Label: label, Value: v}
	}
	return chart.LayoutStackedBar(chart.StackedBarSeries{
		Unit: "",
		Series: []chart.StackedBarRow{
			{Label: "Boardwalk Grill", Segments: []chart.StackedBarSegment{seg("food", 640), seg("drinks", 410), seg("merch", 150)}},
			{Label: "Salt & Sugar", Segments: []chart.StackedBarSegment{seg("food", 520), seg("drinks", 280), seg("merch", 90)}},
			{Label: "Tide Rentals", Segments: []chart.StackedBarSegment{seg("food", 80), seg("drinks", 120), seg("merch", 610)}},
			{Label: "Gull Shack", Segments: []chart.StackedBarSegment{seg("food", 330), seg("drinks", 210), seg("merch", 40)}},
		},
	})
}

func sampleGroupedBar() chart.GroupedBar {
	return chart.LayoutGroupedBar(chart.GroupedBarData{
		Measures: []string{"2024", "2025"},
		Rows: []chart.GroupedBarRow{
			{Label: "Q1", Values: []int{310, 420}},
			{Label: "Q2", Values: []int{780, 905}},
			{Label: "Q3", Values: []int{1240, 1380}},
			{Label: "Q4", Values: []int{560, 610}},
		},
	})
}

func sampleBulletBar() chart.BulletBar {
	row := func(label string, capacity, target, actual int) chart.BulletRow {
		return chart.BulletRow{Label: label, Measures: []chart.BulletMeasure{
			{Label: "capacity", Value: capacity},
			{Label: "target", Value: target},
			{Label: "actual", Value: actual},
		}}
	}
	return chart.LayoutBullet(chart.BulletSeries{
		Series: []chart.BulletRow{
			row("Umbrellas", 400, 320, 348),
			row("Boards", 250, 200, 161),
			row("Cabanas", 120, 110, 117),
			row("Kayaks", 90, 70, 44),
		},
	}, 235)
}

// --- line family -----------------------------------------------------------------

func sampleLine() chart.LineChart {
	n := 24
	labels := hours(n)
	mk := func(label string, vals []float64) chart.LineSeries {
		pts := make([]chart.LinePoint, n)
		for i := range pts {
			v := vals[i]
			if v < 0 {
				v = 0
			}
			pts[i] = chart.LinePoint{X: labels[i], Y: math.Round(v)}
		}
		return chart.LineSeries{Label: label, Points: pts}
	}
	return chart.LayoutLine("spec-line", chart.LineSeriesData{
		YLabel: "visitors",
		Series: []chart.LineSeries{
			mk("North Beach", wave(n, 420, 260, 0.26, 6)),
			mk("The Cove", wave(n, 310, 150, 0.22, 4)),
			mk("Pier South", wave(n, 180, 90, 0.31, 3)),
		},
	}, chart.LineOpts{RefValue: 850, RefLabel: "capacity", Footnote: "synthetic specimen data"})
}

func sampleSparkline() chart.Sparkline {
	return chart.LayoutSparkline(chart.SparklineData{
		Values:     wave(14, 68, 6, 0.5, 0.3),
		TrendValue: "74°F",
		TrendDir:   "up",
	})
}

// --- gauges and billboards --------------------------------------------------------

func sampleGauge() chart.Gauge {
	return chart.Gauge{
		Value:    "68%",
		Pct:      0.68,
		Subtitle: "beach occupancy",
		Footer:   "target 80%",
		Tip: chart.BuildTipHTML("var(--color-series-a)", "Occupancy", []chart.TipRow{
			{Label: "current", Value: "5,440"},
			{Label: "capacity", Value: "8,000"},
		}),
	}
}

func sampleStackedGauge() chart.StackedGauge {
	ramp := chart.GaugeRamp(3, 235)
	return chart.StackedGauge{
		Value:  "$9.6k",
		Footer: "revenue vs. best day",
		Tiers: []chart.StackedTier{
			{Label: "Rentals", ValueStr: "$4.9k", Pct: 0.82, Color: ramp[0]},
			{Label: "Food", ValueStr: "$3.3k", Pct: 0.55, Color: ramp[1]},
			{Label: "Parking", ValueStr: "$1.4k", Pct: 0.31, Color: ramp[2]},
		},
		Tip: chart.BuildTipHTML("", "Revenue today", []chart.TipRow{
			{Label: "rentals", Value: "$4.9k"},
			{Label: "food", Value: "$3.3k"},
			{Label: "parking", Value: "$1.4k"},
		}),
	}
}

func sampleBillboard() chart.Billboard {
	return chart.LayoutBillboard(chart.Billboard{
		Value:      "12,408",
		Label:      "VISITORS TODAY",
		Subtitle:   "all beaches, season to date +4%",
		TrendValue: "+8.2% vs. last Saturday",
		TrendDir:   "up",
		SparklineD: chart.BillboardSparklinePath(wave(20, 9000, 2400, 0.45, 110)),
		Tip: chart.BuildTipHTML("var(--color-series-a)", "Visitors", []chart.TipRow{
			{Label: "today", Value: "12,408"},
			{Label: "last Saturday", Value: "11,467"},
		}),
	})
}

// --- distributions ------------------------------------------------------------------

func sampleScatter() chart.Scatter {
	mk := func(label string, n int, baseX, baseY, spread float64) chart.ScatterSeries {
		pts := make([]chart.ScatterPoint, n)
		for i := range pts {
			x := float64(i)
			pts[i] = chart.ScatterPoint{
				X:     baseX + x*0.35 + spread*math.Sin(x*1.7),
				Y:     baseY + x*0.55 + spread*1.4*math.Sin(x*2.3+0.7),
				Label: fmt.Sprintf("session %d", i+1),
			}
		}
		return chart.ScatterSeries{Label: label, Points: pts}
	}
	return chart.LayoutScatter(chart.ScatterData{
		XLabel: "wave height (ft)",
		YLabel: "ride length (s)",
		Series: []chart.ScatterSeries{
			mk("Longboards", 16, 2, 8, 0.8),
			mk("Shortboards", 16, 3, 4, 1.1),
		},
	})
}

func sampleBoxPlot() chart.BoxPlot {
	return chart.LayoutBoxPlot(chart.BoxPlotData{
		YLabel: "wave height",
		Unit:   "ft",
		Groups: []chart.BoxGroup{
			{Label: "North", Min: 1.1, Q1: 2.4, Med: 3.4, Q3: 4.6, Max: 6.8},
			{Label: "Cove", Min: 0.6, Q1: 1.2, Med: 1.9, Q3: 2.6, Max: 3.8},
			{Label: "Pier", Min: 1.8, Q1: 3.1, Med: 4.2, Q3: 5.4, Max: 7.9},
			{Label: "Dunes", Min: 0.9, Q1: 1.8, Med: 2.7, Q3: 3.9, Max: 5.2},
			{Label: "Gull Pt", Min: 1.4, Q1: 2.2, Med: 3.0, Q3: 4.1, Max: 6.1},
		},
	})
}

func sampleBollinger() chart.Bollinger {
	vals := wave(36, 66, 5, 0.34, 0.16)
	pts := make([]chart.BollingerPoint, len(vals))
	for i, v := range vals {
		pts[i] = chart.BollingerPoint{Label: fmt.Sprintf("d%02d", i+1), Value: math.Round(v*10) / 10}
	}
	return chart.LayoutBollinger(chart.BollingerData{
		Points: pts,
		Window: 7,
		K:      2,
		YLabel: "water temp",
		Unit:   "°F",
	})
}

// --- small-multiple rows -------------------------------------------------------------

func sampleHorizon() chart.Horizon {
	mk := func(label string, base, amp, freq float64) chart.HorizonSeries {
		return chart.HorizonSeries{Label: label, Values: wave(36, base, amp, freq, 0)}
	}
	return chart.LayoutHorizon(chart.HorizonData{
		Bands: 3,
		Series: []chart.HorizonSeries{
			mk("North", 0, 40, 0.30),
			mk("Cove", 0, 24, 0.42),
			mk("Pier", 0, 33, 0.23),
			mk("Dunes", 0, 18, 0.51),
		},
	})
}

func sampleRidgeline() chart.Ridgeline {
	mk := func(label string, peak float64) chart.RidgelineSeries {
		vals := make([]float64, 24)
		for i := range vals {
			x := float64(i)
			vals[i] = math.Exp(-((x-peak)*(x-peak))/18) * 100
		}
		return chart.RidgelineSeries{Label: label, Values: vals}
	}
	return chart.LayoutRidgeline(chart.RidgelineData{
		Series: []chart.RidgelineSeries{
			mk("Swimmers", 13),
			mk("Surfers", 8),
			mk("Anglers", 6),
			mk("Joggers", 7.5),
			mk("Picnics", 12),
		},
	})
}

func sampleDifference() chart.Difference {
	n := 18
	labels := make([]string, n)
	for i := range labels {
		labels[i] = fmt.Sprintf("w%d", i+1)
	}
	return chart.LayoutDifference(chart.DifferenceData{
		Labels:  labels,
		SeriesA: chart.DiffSeries{Label: "North swell", Values: wave(n, 4.2, 1.8, 0.45, 0)},
		SeriesB: chart.DiffSeries{Label: "South swell", Values: wave(n, 3.6, 1.4, 0.38, 0.09)},
		YLabel:  "swell",
		Unit:    "ft",
	})
}

func sampleTimeline() chart.Timeline {
	return chart.LayoutTimeline(chart.TimelineData{
		Rows: []chart.TimelineRow{
			{Label: "Dune fencing", Start: 0, End: 18, Group: "build"},
			{Label: "Lifeguard towers", Start: 8, End: 30, Group: "build"},
			{Label: "Boardwalk stalls", Start: 22, End: 44, Group: "build"},
			{Label: "Swim season", Start: 40, End: 110, Group: "season"},
			{Label: "Surf school", Start: 46, End: 102, Group: "season"},
			{Label: "Night market", Start: 60, End: 96, Group: "season"},
			{Label: "Stall pack-down", Start: 104, End: 122, Group: "teardown"},
			{Label: "Tower removal", Start: 112, End: 126, Group: "teardown"},
		},
	})
}

// --- flows and grids -----------------------------------------------------------------

func sampleStream() chart.Stream {
	mk := func(label string, base, amp, freq float64) chart.StreamLayer {
		vals := wave(12, base, amp, freq, 0)
		for i, v := range vals {
			if v < 4 {
				vals[i] = 4
			} else {
				vals[i] = math.Round(v)
			}
		}
		return chart.StreamLayer{Label: label, Values: vals}
	}
	return chart.LayoutStream(chart.StreamData{
		Labels: monthLabels,
		Layers: []chart.StreamLayer{
			mk("Rentals", 60, 45, 0.52),
			mk("Food", 80, 30, 0.48),
			mk("Parking", 45, 28, 0.55),
			mk("Lessons", 25, 18, 0.61),
		},
		Unit: "k",
	})
}

func sampleSankey() chart.Sankey {
	node := func(id, label string) chart.SankeyNode { return chart.SankeyNode{ID: id, Label: label} }
	link := func(src, dst string, v float64) chart.SankeyLink {
		return chart.SankeyLink{Source: src, Target: dst, Value: v}
	}
	return chart.LayoutSankey(chart.SankeyData{
		Nodes: []chart.SankeyNode{
			node("north-lot", "North Lot"), node("south-lot", "South Lot"), node("transit", "Transit"),
			node("boardwalk", "Boardwalk"), node("beach", "Beach"),
			node("rentals", "Rentals"), node("food", "Food Hall"), node("pier", "Pier Shows"),
		},
		Links: []chart.SankeyLink{
			link("north-lot", "boardwalk", 420), link("north-lot", "beach", 380),
			link("south-lot", "beach", 510), link("transit", "boardwalk", 260),
			link("boardwalk", "food", 340), link("boardwalk", "rentals", 200), link("boardwalk", "pier", 140),
			link("beach", "rentals", 460), link("beach", "food", 240), link("beach", "pier", 190),
		},
	})
}

func sampleChord() chart.Chord {
	return chart.LayoutChord(chart.ChordData{
		Groups: []string{"North", "Cove", "Pier", "Dunes"},
		Matrix: [][]float64{
			{0, 110, 75, 40},
			{95, 0, 60, 35},
			{80, 55, 0, 70},
			{30, 45, 65, 0},
		},
		Unit: " walkers",
	})
}

func sampleBundle() chart.Bundle {
	node := func(id, label, group string) chart.BundleNode {
		return chart.BundleNode{ID: id, Label: label, Group: group}
	}
	link := func(src, dst string) chart.BundleLink { return chart.BundleLink{Source: src, Target: dst, Value: 1} }
	return chart.LayoutBundle(chart.BundleData{
		Nodes: []chart.BundleNode{
			node("n1", "Gull Point", "north"), node("n2", "North Jetty", "north"),
			node("n3", "Long Bar", "north"), node("n4", "The Cove", "central"),
			node("n5", "Main Stairs", "central"), node("n6", "Pier North", "central"),
			node("n7", "Pier South", "central"), node("n8", "Flats", "south"),
			node("n9", "Dunes", "south"), node("n10", "South Jetty", "south"),
			node("n11", "Backshore", "south"), node("n12", "Inlet", "south"),
		},
		Links: []chart.BundleLink{
			link("n1", "n4"), link("n1", "n6"), link("n2", "n3"), link("n2", "n7"),
			link("n3", "n5"), link("n4", "n8"), link("n5", "n9"), link("n6", "n10"),
			link("n7", "n9"), link("n8", "n12"), link("n9", "n11"), link("n10", "n12"),
			link("n4", "n7"), link("n5", "n6"),
		},
	})
}

func sampleHeatmap() chart.Heatmap {
	rows := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	cols := []string{"8a", "10a", "12p", "2p", "4p", "6p", "8p", "10p"}
	cells := make([]chart.HeatmapCell, 0, len(rows)*len(cols))
	for r := range rows {
		for c := range cols {
			v := 25 + 40*math.Sin(float64(c)*0.55) + 8*float64(r)
			if r >= 5 { // weekend bump
				v += 30
			}
			if v < 0 {
				v = 0
			}
			cells = append(cells, chart.HeatmapCell{Row: r, Col: c, Value: math.Round(v)})
		}
	}
	return chart.LayoutHeatmap(chart.HeatmapData{Rows: rows, Cols: cols, Cells: cells, Unit: " visitors"})
}

func sampleCalendar() chart.Calendar {
	start := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC) // a Sunday, so DOW 0 aligns
	days := make([]chart.CalendarDay, 0, 52*7)
	for i := 0; i < 52*7; i++ {
		d := start.AddDate(0, 0, i)
		dow := i % 7
		v := 22 + 16*math.Sin(float64(i)/29.0) + 8*math.Sin(float64(i)/3.1)
		if dow == 0 || dow == 6 {
			v += 18
		}
		if v < 0 {
			v = 0
		}
		days = append(days, chart.CalendarDay{
			Date:  d.Format("2006-01-02"),
			Value: math.Round(v),
			Year:  d.Year(),
			Month: int(d.Month()),
			Day:   d.Day(),
			DOW:   dow,
			Week:  i / 7,
		})
	}
	return chart.LayoutCalendar(chart.CalendarData{Days: days, Unit: "rentals"})
}

// --- bar race (static frame) -----------------------------------------------------------

// sampleBarRace is one mid-race frame. The live version — server patching
// successive frames over SSE — runs in examples/boardwalk.
func sampleBarRace() chart.BarRaceLayout {
	return chart.LayoutBarRace(chart.BarRaceInput{
		Title: "Rental revenue by vendor — week 31",
		Bars: []chart.Datum{
			{Label: "Tide Rentals", Value: 9420},
			{Label: "Salt & Sugar", Value: 8110},
			{Label: "Boardwalk Grill", Value: 7660},
			{Label: "Gull Shack", Value: 5230},
			{Label: "Dune Surf Co", Value: 4870},
			{Label: "Pier Bait", Value: 2390},
		},
		Top: 6,
	})
}

// --- geo map -----------------------------------------------------------------------------

// sampleGeoMap is the visitor-origin choropleth: where the season's guests
// came from, with the megacity reference layer on.
func sampleGeoMap() chart.GeoMap {
	return chart.LayoutGeoMap(chart.GeoMapData{
		Regions: []chart.GeoRegion{
			{Code: "US", Value: 9420}, {Code: "CA", Value: 3110}, {Code: "MX", Value: 1240},
			{Code: "GB", Value: 980}, {Code: "DE", Value: 720}, {Code: "FR", Value: 640},
			{Code: "BR", Value: 410}, {Code: "JP", Value: 350}, {Code: "AU", Value: 290},
			{Code: "NO", Value: 120},
		},
		ShowCities: true,
		Unit:       "visitors",
	})
}

// sampleGeoMapInteractive is the same choropleth with the client-side zoom/pan
// module enabled. No DrillAction — the specimen has no backend to drill into,
// so the map is explorable but does not fetch.
func sampleGeoMapInteractive() chart.GeoMap {
	return chart.LayoutGeoMap(chart.GeoMapData{
		Regions: []chart.GeoRegion{
			{Code: "US", Value: 9420}, {Code: "CA", Value: 3110}, {Code: "MX", Value: 1240},
			{Code: "GB", Value: 980}, {Code: "DE", Value: 720}, {Code: "FR", Value: 640},
			{Code: "BR", Value: 410}, {Code: "JP", Value: 350}, {Code: "AU", Value: 290},
			{Code: "NO", Value: 120},
		},
		Unit:        "visitors",
		Ramp:        "royal",
		Interactive: true,
	})
}

// sampleGlobeThemed is a static themed-globe frame: the visitor choropleth on
// the orthographic sphere, camera over the Atlantic.
func sampleGlobeThemed() chart.Globe {
	return chart.LayoutGlobe(chart.GlobeData{
		Style: "themed",
		Regions: []chart.GeoRegion{
			{Code: "US", Value: 9420}, {Code: "CA", Value: 3110}, {Code: "MX", Value: 1240},
			{Code: "GB", Value: 980}, {Code: "DE", Value: 720}, {Code: "FR", Value: 640},
			{Code: "BR", Value: 410}, {Code: "NO", Value: 120},
		},
		Unit:      "visitors",
		Ramp:      "ocean",
		Lon0:      -30,
		Lat0:      25,
		Graticule: true,
	})
}

// sampleGlobeWire is a static wireframe globe: fill-less country outlines with
// the graticule always on, camera over the Pacific.
func sampleGlobeWire() chart.Globe {
	return chart.LayoutGlobe(chart.GlobeData{
		Style: "wire",
		Lon0:  150,
		Lat0:  10,
	})
}

// sampleSunClock is the day/night terminator at a FIXED instant (northern
// solstice, mid-afternoon UTC) so the specimen renders identically on every
// build — a live clock would drift the terminator each load.
func sampleSunClock() chart.GeoClock {
	return chart.LayoutGeoClock(chart.GeoClockData{
		Time:         time.Date(2026, 6, 21, 15, 30, 0, 0, time.UTC),
		ShowCities:   true,
		ShowTwilight: true,
	})
}

// --- accessibility section ------------------------------------------------------

// contrastRows renders every pair the kit paints, in both schemes, measured at
// render time.
//
// The list used to be duplicated here — a readable subset, kept in step with the
// test by hand. It is not any more: theme.Scheme.Pairs() is the single list, and
// the test and this table walk the same one. A contrast table that could drift
// from the assertion behind it is worse than no table, because it advertises a
// guarantee nobody is holding.
func contrastRows() [][]string {
	t, err := theme.BuildPreset(view.ThemePreset)
	if err != nil {
		return [][]string{{"theme did not derive", err.Error(), "", "", "FAIL"}}
	}
	var rows [][]string
	for _, s := range []struct {
		name   string
		scheme theme.Scheme
	}{{"dark", t.Dark}, {"light", t.Light}} {
		for _, p := range s.scheme.Pairs() {
			verdict := "pass"
			if !p.Passes() {
				verdict = "FAIL"
			}
			rows = append(rows, []string{
				s.name,
				p.What,
				strconv.FormatFloat(p.Ratio(), 'f', 2, 64) + ":1",
				strconv.FormatFloat(p.Min, 'f', 1, 64) + ":1",
				p.Criterion,
				verdict,
			})
		}
	}
	return rows
}

// contrastRowCount is how many rows the table should hold: every pair, twice.
func contrastRowCount() int {
	t, err := theme.BuildPreset(view.ThemePreset)
	if err != nil {
		return 1
	}
	return len(t.Dark.Pairs()) + len(t.Light.Pairs())
}

// specimenLang reports the language the document declared, so the page can show
// that <html lang> followed the request rather than a hardcoded default.
func specimenLang(ctx context.Context) string {
	if loc := i18n.Locale(ctx); loc != "" {
		return loc
	}
	return "en (no locale configured)"
}

// --- spacing ladder --------------------------------------------------------------

// spaceRung is one row of the specimen's spacing table.
type spaceRung struct{ Name, Class, Value string }

// spaceRungs walks driftwood.Spaces so the table cannot list a rung the kit does
// not have, or miss one it does. The bar in each row is painted by the real
// padding class, so the page is measuring the ladder rather than describing it.
func spaceRungs() []spaceRung {
	vals := map[driftwood.Space]string{
		driftwood.SpaceNone: "0", driftwood.Space2XS: "0.25rem", driftwood.SpaceXS: "0.5rem",
		driftwood.SpaceSm: "0.75rem", driftwood.SpaceMd: "1rem", driftwood.SpaceLg: "1.5rem",
		driftwood.SpaceXL: "2rem", driftwood.Space2XL: "3rem", driftwood.Space3XL: "4rem",
	}
	out := make([]spaceRung, 0, len(driftwood.Spaces))
	for _, s := range driftwood.Spaces {
		out = append(out, spaceRung{
			Name:  "Space" + strings.ToUpper(string(s)),
			Class: "dw-padx-" + string(s),
			Value: vals[s],
		})
	}
	return out
}
