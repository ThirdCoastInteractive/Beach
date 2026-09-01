package chart

import (
	"context"

	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
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
	label       string
	desc        string
	interactive bool
	kind        string
}

// Label sets the chart's accessible name — what it is, in a few words.
func Label(s string) FigureOpt { return func(c *figureConfig) { c.label = s } }

// Interactive makes a chart operable from a keyboard: the figure becomes a
// focusable group with an announcement region, and the client layer walks its
// shapes on the arrow keys, announcing each one.
//
// It is an option rather than a default because most charts should stay
// decorative. A chart with no pointer interaction has nothing to step through,
// and a tab stop that does nothing is worse than no tab stop. Reach for it
// wherever a chart's information is reachable by pointer and would otherwise be
// reachable *only* by pointer (WCAG 2.1.1).
func Interactive() FigureOpt { return func(c *figureConfig) { c.interactive = true } }

// withKind tags the figure with the chart family, so the client layer knows how
// to walk it — which shapes are the samples, and where their labels live. It is
// set by the fragment, never by a caller: a chart does not get to lie about what
// it is.
func withKind(k string) FigureOpt {
	return func(c *figureConfig) {
		c.kind = k
		c.interactive = true
	}
}

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

// interactiveChartName is a keyboard-operable chart's accessible name. It always
// ends with the key hint, because a widget with a tab stop has to say what the
// keys do and nothing else on the page will. A caller-supplied Label replaces
// the generic half, so the name says *which* chart rather than only that it is
// one.
func interactiveChartName(ctx context.Context, f figureConfig) string {
	name := f.label
	if name == "" {
		name = i18n.T(ctx, "ui.a11y.chart.generic")
	}
	return name + " " + i18n.T(ctx, "ui.a11y.chart.keys")
}

// --- band-chart hover payload ----------------------------------------------------

// VBHover is the per-sample data behind a band chart's crosshair.
//
// Bollinger and Difference draw a crosshair that shows a *position* and never a
// value — which is enough for a pointer, whose owner can read the line under it,
// and nothing at all for a keyboard, which has no way to read anything. So the
// samples travel to the client, and the announcement text is built here where
// the labels, the units and the number formatting already live. A client module
// cannot reach the i18n catalog; the server can.
type VBHover struct {
	// Samples are in x order. FX is the sample's position across the plot as a
	// 0..1 fraction, which is all the crosshair needs to place itself.
	Samples []VBSample `json:"samples"`
}

// VBSample is one x position on a band chart and what to say about it.
type VBSample struct {
	FX float64 `json:"fx"`
	// Text is the finished announcement — label, values, unit — rather than
	// parts the client would have to assemble and, in doing so, decide
	// formatting and word order for a language it does not know.
	Text string `json:"text"`
}

// vbHover builds the payload from parallel label and value slices. n samples are
// spread evenly across the plot, which is how both band charts place their x
// axis, so fx is just the index fraction.
func vbHover(labels []string, format func(i int) string) VBHover {
	n := len(labels)
	h := VBHover{Samples: make([]VBSample, 0, n)}
	for i := range labels {
		fx := 0.0
		if n > 1 {
			fx = float64(i) / float64(n-1)
		}
		h.Samples = append(h.Samples, VBSample{FX: fx, Text: format(i)})
	}
	return h
}
