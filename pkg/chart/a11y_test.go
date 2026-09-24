package chart

// The chart accessibility law, as a test.
//
// A chart is a bag of paths and numbers, so its SVG is either named or hidden —
// there is no third option. `role="img"` with no accessible name announces
// "image" and stops, which interrupts to say that something unidentifiable is
// there; that was the state of 26 of these before RFC 06.
//
// Charts render through templ, so `beach-vet` sees their markup in the generated
// string literals and would catch a nameless role="img" written by hand. What it
// cannot see is a name that arrives at runtime through a FigureOpt, which is
// exactly what these fragments do — hence this suite.

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderChart(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

// roleImgRe finds an element claiming to be a graphic.
var roleImgRe = regexp.MustCompile(`<[a-z][^>]*\brole="img"[^>]*>`)

// namedRe matches either way of giving one an accessible name.
var namedRe = regexp.MustCompile(`aria-(label|labelledby)="[^"]`)

// noNamelessGraphic is the law: every role="img" in the output carries a name.
func noNamelessGraphic(t *testing.T, name, out string) {
	t.Helper()
	for _, tag := range roleImgRe.FindAllString(out, -1) {
		if !namedRe.MatchString(tag) {
			t.Errorf("%s: role=\"img\" with no accessible name — name it, or drop the role and mark it aria-hidden\n%s", name, tag)
		}
	}
}

// chartCases is every chart fragment, rendered from the same shapes an app
// builds. The data is deliberately minimal: this suite is about the wrapper, and
// the geometry has its own tests.
func chartCases() map[string]templ.Component {
	return map[string]templ.Component{
		"bar":          ChartBarFragment("c", BarChart{}),
		"stackedbar":   ChartStackedBarFragment("c", StackedBar{}),
		"stackedgauge": ChartStackedGaugeFragment("c", StackedGauge{}),
		"gauge":        ChartGaugeFragment("c", Gauge{}),
		"billboard":    ChartBillboardFragment("c", Billboard{}),
		"boxplot":      ChartBoxPlotFragment("c", BoxPlot{}),
		"calendar":     ChartCalendarFragment("c", Calendar{}),
		"bollinger":    ChartBollingerFragment("c", Bollinger{}),
		"horizon":      ChartHorizonFragment("c", Horizon{}),
		"ridgeline":    ChartRidgelineFragment("c", Ridgeline{}),
		"difference":   ChartDifferenceFragment("c", Difference{}),
		"timeline":     ChartTimelineFragment("c", Timeline{}),
		"bundle":       ChartBundleFragment("c", Bundle{}),
		"groupedbar":   ChartGroupedBarFragment("c", GroupedBar{}),
		"stream":       ChartStreamFragment("c", Stream{}),
		"scatter":      ChartScatterFragment("c", Scatter{}),
		"sankey":       ChartSankeyFragment("c", Sankey{}),
		"singleline":   ChartSingleLineFragment("c", SingleLine{}),
		"chord":        ChartChordFragment("c", Chord{}),
		"sparkline":    ChartSparklineFragment("c", Sparkline{}),
		"heatmap":      ChartHeatmapFragment("c", Heatmap{}),
		"phase":        ChartPhaseFragment("c", Phase{}),
		"bulletbar":    ChartBulletBarFragment("c", BulletBar{}),
		"geomap":       ChartGeoMapFragment("c", GeoMap{}),
		"line":         ChartLineFragment(LineChart{HoverID: "c"}),
		"barrace":      BarRaceSVG(BarRaceLayout{W: 10, H: 10}),
		"barracetitle": BarRaceSVG(BarRaceLayout{W: 10, H: 10, Title: "Race"}),
	}
}

// TestChartsAreNamedOrHidden runs the law over every chart, unnamed and named.
func TestChartsAreNamedOrHidden(t *testing.T) {
	for name, c := range chartCases() {
		noNamelessGraphic(t, name, renderChart(t, c))
	}
	// And again with a Label. A chart the pointer can interrogate is a widget,
	// not a picture, so a labelled one comes out as a named role="group" rather
	// than a named role="img". The name has to be there either way.
	labelled := map[string]templ.Component{
		"bar":    ChartBarFragment("c", BarChart{}, Label("Visits by beach")),
		"geomap": ChartGeoMapFragment("c", GeoMap{}, Label("Visits by country")),
	}
	for name, c := range labelled {
		out := renderChart(t, c)
		noNamelessGraphic(t, name, out)
		if !strings.Contains(out, `aria-label="Visits by`) {
			t.Errorf("%s: Label did not reach the figure's name\n%s", name, out)
		}
		if !strings.Contains(out, `role="group"`) && !strings.Contains(out, `role="img"`) {
			t.Errorf("%s: a labelled figure that claims no role at all\n%s", name, out)
		}
	}
}

// TestUnnamedChartIsDecorative pins the other half of the law. Without a Label
// the figure stays a plain <figure> and the SVG is hidden — the honest shape for
// a chart whose card heading already names it. Claiming role="img" here would be
// the defect this replaced.
func TestUnnamedChartIsDecorative(t *testing.T) {
	// A chart with no per-shape tooltip and no name: nothing to step through,
	// nothing to announce, so it takes no role and no tab stop. A tab stop that
	// does nothing is worse than none.
	out := renderChart(t, ChartHorizonFragment("c", Horizon{}))
	if strings.Contains(out, `role="img"`) || strings.Contains(out, `role="group"`) {
		t.Errorf("a decorative chart claimed a role:\n%s", out)
	}
	if strings.Contains(out, "tabindex") {
		t.Errorf("a decorative chart took a tab stop that leads nowhere:\n%s", out)
	}
	if !strings.Contains(out, `aria-hidden="true"`) {
		t.Errorf("a decorative chart's SVG is not hidden, so it announces as an unlabelled graphic:\n%s", out)
	}
}

// TestHoverChartsAreKeyboardReachable is WCAG 2.1.1 for the charts whose
// information lives in a tooltip.
//
// Every one of these used to be pointer-only: the shapes carrying data-tip sit
// inside an aria-hidden SVG, so there was no route to them that did not involve
// a mouse. Each now renders a focusable group, a name saying which keys work,
// and the region its client layer announces into. A chart missing any one of the
// three looks operable and is not.
func TestHoverChartsAreKeyboardReachable(t *testing.T) {
	for name, c := range chartCases() {
		out := renderChart(t, c)
		if !strings.Contains(out, "data-chart-keys") {
			continue // decorative by design; TestUnnamedChartIsDecorative covers that
		}
		if !strings.Contains(out, `tabindex="0"`) {
			t.Errorf("%s: marked keyboard-operable but takes no focus\n%s", name, out)
		}
		if !strings.Contains(out, `role="group"`) {
			t.Errorf("%s: operable but claims no role\n%s", name, out)
		}
		if !strings.Contains(out, `aria-live="polite"`) {
			t.Errorf("%s: nowhere to announce the sample a keyboard lands on\n%s", name, out)
		}
		if !strings.Contains(out, "arrow keys") {
			t.Errorf("%s: has a tab stop but never says what the keys do\n%s", name, out)
		}
	}
}

// TestDescribeRendersTextAlternative checks the part that carries the chart's
// actual content to someone who cannot see it — the sentence the chart was drawn
// to say, in a caption that costs nothing visually.
func TestDescribeRendersTextAlternative(t *testing.T) {
	out := renderChart(t, ChartBarFragment("c", BarChart{},
		Label("Visits by beach"),
		Describe("North Beach leads at 9,420.")))
	if !strings.Contains(out, `<figcaption class="sr-only">North Beach leads at 9,420.</figcaption>`) {
		t.Errorf("Describe did not render a screen-reader caption:\n%s", out)
	}
}

// TestLineChartKeepsItsKeyboardHint guards the one chart that is a widget rather
// than a picture: it has a tab stop, so its name has to say what the keys do. A
// caller-supplied Label replaces the generic half, never the hint.
func TestLineChartKeepsItsKeyboardHint(t *testing.T) {
	plain := renderChart(t, ChartLineFragment(LineChart{HoverID: "c"}))
	named := renderChart(t, ChartLineFragment(LineChart{HoverID: "c"}, Label("Visitors by hour")))
	for what, out := range map[string]string{"unnamed": plain, "named": named} {
		if !strings.Contains(out, "arrow keys") {
			t.Errorf("%s line chart dropped the keyboard hint from its name:\n%s", what, out)
		}
		if !strings.Contains(out, `tabindex="0"`) || !strings.Contains(out, `role="group"`) {
			t.Errorf("%s line chart is no longer a focusable widget:\n%s", what, out)
		}
		if !strings.Contains(out, `aria-live="polite"`) {
			t.Errorf("%s line chart lost the live region its keyboard stepping announces into:\n%s", what, out)
		}
	}
	if !strings.Contains(named, "Visitors by hour") {
		t.Errorf("Label did not reach the line chart's name:\n%s", named)
	}
}
