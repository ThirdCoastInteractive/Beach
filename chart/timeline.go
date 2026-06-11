package chart

import (
	"io"
	"math"
)

// TimelineSpan is one bar on a timeline: a labeled [Start,End] interval on a
// given row (lane). Start/End are in arbitrary caller units (e.g. days).
type TimelineSpan struct {
	Label      string
	Start, End float64
	Row        int
}

// TimelineInput configures a timeline / Gantt-style chart.
type TimelineInput struct {
	Title string
	Spans []TimelineSpan
	Width float64 // defaults to 640
	RowH  float64 // defaults to 28
}

// TimelineLayout is the geometry for a timeline.
type TimelineLayout struct {
	W, H  float64
	Title string
	Bars  []timelineBar
}

type timelineBar struct {
	Box   Box
	Color string
	Label string
	Tip   string
}

// LayoutTimeline computes geometry for a timeline chart.
func LayoutTimeline(in TimelineInput) TimelineLayout {
	w := in.Width
	if w == 0 {
		w = 640
	}
	rowH := in.RowH
	if rowH == 0 {
		rowH = 28
	}
	lay := TimelineLayout{W: w, Title: in.Title}
	if len(in.Spans) == 0 {
		lay.H = rowH
		return lay
	}
	min, max := math.Inf(1), math.Inf(-1)
	maxRow := 0
	for _, sp := range in.Spans {
		min = math.Min(min, sp.Start)
		max = math.Max(max, sp.End)
		if sp.Row > maxRow {
			maxRow = sp.Row
		}
	}
	if max <= min {
		max = min + 1
	}
	left := 8.0
	trackW := w - left - 8
	sx := func(v float64) float64 { return left + (v-min)/(max-min)*trackW }
	top := 8.0
	for i, sp := range in.Spans {
		y := top + float64(sp.Row)*rowH
		x0 := sx(sp.Start)
		x1 := sx(sp.End)
		lay.Bars = append(lay.Bars, timelineBar{
			Box:   Box{X: x0, Y: y + 4, W: x1 - x0, H: rowH - 10},
			Color: QualitativeVar(i),
			Label: sp.Label,
			Tip:   sp.Label + ": " + fnum(sp.Start) + "–" + fnum(sp.End),
		})
	}
	lay.H = top*2 + float64(maxRow+1)*rowH
	return lay
}

// TimelineSVG renders the timeline described by lay.
func TimelineSVG(w io.Writer, lay TimelineLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	for _, b := range lay.Bars {
		s.p(`<g>`)
		s.title(b.Tip)
		s.rect(b.Box, b.Color, `rx="3" fill-opacity="0.85"`)
		s.p(`</g>`)
		s.text(b.Box.X+6, b.Box.Y+b.Box.H/2+4, "start", "chart-label", b.Label)
	}
	s.close()
	return s.Err()
}
