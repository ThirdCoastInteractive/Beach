package chart

import (
	"io"
	"math"
	"time"
)

// CalendarInput configures a GitHub-style calendar heatmap for one year.
type CalendarInput struct {
	Title string
	Year  int
	Days  map[string]float64 // "YYYY-MM-DD" -> value
	Cell  float64            // cell size; defaults to 11
}

// CalendarLayout is the geometry for a calendar heatmap.
type CalendarLayout struct {
	W, H  float64
	Title string
	Cells []calCell
}

type calCell struct {
	Box     Box
	Opacity float64
	Color   string
	Tip     string
}

// LayoutCalendarHeatmap computes geometry for a one-year calendar heatmap, laid
// out in week columns (Sunday-started) like a contribution graph.
func LayoutCalendarHeatmap(in CalendarInput) CalendarLayout {
	cell := in.Cell
	if cell == 0 {
		cell = 11
	}
	gap := 2.0
	year := in.Year
	if year == 0 {
		year = time.Now().Year()
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)

	max := 0.0
	for _, v := range in.Days {
		max = math.Max(max, v)
	}
	if max <= 0 {
		max = 1
	}
	color := QualitativeVar(0)
	left, top := 8.0, 8.0

	var cells []calCell
	week := 0
	// Align week column to the day-of-week of Jan 1.
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dow := int(d.Weekday()) // Sunday=0
		if d.Equal(start) {
			// nothing; week stays 0
		} else if dow == 0 {
			week++
		}
		key := d.Format("2006-01-02")
		v := in.Days[key]
		op := 0.08
		if v > 0 {
			op = 0.15 + 0.85*(v/max)
		}
		cells = append(cells, calCell{
			Box: Box{
				X: left + float64(week)*(cell+gap),
				Y: top + float64(dow)*(cell+gap),
				W: cell, H: cell,
			},
			Opacity: op, Color: color,
			Tip: key + ": " + fnum(v),
		})
	}
	w := left*2 + float64(week+1)*(cell+gap)
	h := top*2 + 7*(cell+gap)
	return CalendarLayout{W: w, H: h, Title: in.Title, Cells: cells}
}

// CalendarHeatmapSVG renders the calendar heatmap described by lay.
func CalendarHeatmapSVG(w io.Writer, lay CalendarLayout) error {
	s := newSVG(w)
	s.open(lay.W, lay.H, lay.Title)
	color := QualitativeVar(0)
	for _, c := range lay.Cells {
		s.p(`<g>`)
		s.title(c.Tip)
		s.rect(c.Box, color, `fill-opacity="`+fnum(c.Opacity)+`" rx="2"`)
		s.p(`</g>`)
	}
	s.close()
	return s.Err()
}
