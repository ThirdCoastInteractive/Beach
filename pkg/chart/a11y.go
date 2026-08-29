package chart

import (
	"context"

	"github.com/ThirdCoastInteractive/the-beach/pkg/i18n"
)

// Accessibility of a server-rendered chart.
//
// A chart SVG is a bag of paths and numbers: read out node by node it says
// nothing, and `role="img"` with no accessible name announces it as an
// unlabelled graphic — worse than silence, because it interrupts to say
// "image" and stops. So the SVG itself is hidden from assistive technology and
// the <figure> around it carries the alternative:
//
//	@chart.ChartBarFragment("spend", c,
//		chart.Label("Spend by category, last 30 days"),
//		chart.Describe("Produce leads at 41%, then dairy at 22%."))
//
// Label is the chart's name; Describe is the sentence someone who cannot see it
// would need instead — the trend, the outlier, the answer the chart was drawn
// to give. Both are optional, and a chart with neither renders as decorative,
// which is the right answer when a titled card already names it (WCAG 1.1.1).

// FigureOpt configures the accessible wrapper around a chart. The options are
// variadic on every Chart*Fragment, so existing two-argument calls are
// unaffected.
type FigureOpt func(*figureConfig)

type figureConfig struct {
	label string
	desc  string
}

// Label sets the chart's accessible name — what it is, in a few words.
func Label(s string) FigureOpt { return func(c *figureConfig) { c.label = s } }

// Describe sets the chart's text alternative — what it shows. It renders as a
// screen-reader-only caption, so it costs nothing visually and is the only
// route to the chart's content for anyone who cannot see it.
func Describe(s string) FigureOpt { return func(c *figureConfig) { c.desc = s } }

// figureOf resolves an option set.
func figureOf(opts []FigureOpt) figureConfig {
	var c figureConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// named reports whether the figure has an accessible name, and therefore
// whether it may claim role="img". An unnamed figure stays a plain <figure>.
func (c figureConfig) named() bool { return c.label != "" }

// lineChartName is the interactive line chart's accessible name. It always ends
// with the keyboard hint, because a widget with a tab stop has to say what the
// keys do; a caller-supplied Label replaces the generic "Time series chart"
// half so the name says which chart it is.
func lineChartName(ctx context.Context, f figureConfig) string {
	name := f.label
	if name == "" {
		name = i18n.T(ctx, "ui.a11y.chart.line")
	}
	return name + " " + i18n.T(ctx, "ui.a11y.chart.keys")
}
