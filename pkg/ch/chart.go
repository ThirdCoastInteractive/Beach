package ch

// This file is the thin bridge between a ClickHouse query and a server-side
// chart. The query is the whole API: you write SQL that buckets and aggregates,
// scan its rows into a typed struct with ch.Rows, then hand the slice to one of
// the adapters below to get a chart input you can lay out and render.
//
// There is deliberately no query DSL. Each adapter takes the row slice plus one
// or two small projector funcs that pull the label/value/x/y out of a row, so it
// works for any struct shape without reflection or struct-tag coupling.
//
//	type row struct {
//	    Rack  string `ch:"rack"`
//	    Units int64  `ch:"units"`
//	}
//	rows, _ := ch.Rows[row](ctx, conn,
//	    `SELECT rack, count() AS units
//	       FROM assets GROUP BY rack ORDER BY units DESC`)
//	in := ch.ToHBarSeries(rows,
//	    func(r row) string { return r.Rack },
//	    func(r row) int { return int(r.Units) })
//	in.Unit = " units"
//	// In a .templ view: @chart.BarChartSVG(chart.LayoutHBar(in))
//
// # Deferred dashboard-widget pattern
//
// A chart fed by a ClickHouse aggregate is the textbook case for ui.Defer: the
// query shouldn't block first paint and the SVG can exceed the 14KB budget. Place
// a ui.Defer placeholder in the page; its @get lands on an ordinary
// beach.PageFunc that runs the query and returns the chart fragment. The
// framework's dual-purpose branch morphs the fragment into the placeholder.
//
//	// In the page body: reserve the box, defer the fetch.
//	@ui.Defer(ui.DeferProps{ID: "rack-units", Height: "180px", Get: "/widgets/rack-units"})
//
//	// The widget route: a normal PageFunc. ch is optional, so a nil Conn yields
//	// an empty series and the chart renders blank rather than erroring.
//	func RackUnits(conn ch.Conn) beach.PageFunc {
//	    return func(c *beach.Ctx) (beach.View, error) {
//	        type row struct {
//	            Rack  string `ch:"rack"`
//	            Units int64  `ch:"units"`
//	        }
//	        rows, err := ch.Rows[row](c.Context(), conn,
//	            `SELECT rack, count() AS units
//	               FROM assets GROUP BY rack ORDER BY units DESC LIMIT 10`)
//	        if err != nil {
//	            return beach.View{}, err
//	        }
//	        in := ch.ToHBarSeries(rows,
//	            func(r row) string { return r.Rack },
//	            func(r row) int { return int(r.Units) })
//	        in.Unit = " units"
//	        // chart.ChartBarFragment wraps the SVG in a figure carrying the same
//	        // id the placeholder used, so the patch morphs cleanly.
//	        return beach.View{Fragment: chart.ChartBarFragment("rack-units", chart.LayoutHBar(in))}, nil
//	    }
//	}
//
// The adapters construct chart's input structs through the typed projectors —
// no reflection, no struct-tag coupling. (They live in ch because ch.Rows is
// the upstream half of the bridge; the caller does the one
// chart.Layout*/chart.*SVG call.)

import "github.com/ThirdCoastInteractive/Beach/pkg/chart"

// ToSeries turns query rows into a []chart.Datum (label + value), the input
// shape for the SSE bar race (chart.BarRaceInput.Bars). label and value pull
// the two fields out of each row; row order is preserved (so ORDER BY in SQL
// controls chart order).
func ToSeries[T any](rows []T, label func(T) string, value func(T) float64) []chart.Datum {
	out := make([]chart.Datum, 0, len(rows))
	for _, r := range rows {
		out = append(out, chart.Datum{Label: label(r), Value: value(r)})
	}
	return out
}

// ToHBarSeries turns query rows into a chart.HBarSeries, the common case for a
// "top N by metric" ranking. Set Unit/Max on the returned value before laying
// it out with chart.LayoutHBar; the Series field is already populated.
func ToHBarSeries[T any](rows []T, label func(T) string, value func(T) int) chart.HBarSeries {
	entries := make([]chart.HBarEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, chart.HBarEntry{Label: label(r), Value: value(r)})
	}
	return chart.HBarSeries{Series: entries}
}

// ToStackedBarSeries turns query rows into a chart.StackedBarSeries for
// chart.LayoutStackedBar. Each row becomes one bar; segments projects a row
// into its stacked segments (a few sumIf aggregates per row fit naturally).
func ToStackedBarSeries[T any](rows []T, label func(T) string, segments func(T) []chart.StackedBarSegment) chart.StackedBarSeries {
	out := make([]chart.StackedBarRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, chart.StackedBarRow{Label: label(r), Segments: segments(r)})
	}
	return chart.StackedBarSeries{Series: out}
}

// ToLineSeries turns query rows into a single named chart.LineSeries, the
// per-series shape for line/area charts. x is the bucket label (e.g. a
// formatted toStartOfHour); y is the aggregate. Rows are taken in order, so
// ORDER BY the time column in SQL.
//
// For a single series wrap the result with ToLineSeriesData; put several
// ToLineSeries results in one chart.LineSeriesData for a multi-line chart.
func ToLineSeries[T any](name string, rows []T, x func(T) string, y func(T) float64) chart.LineSeries {
	pts := make([]chart.LinePoint, 0, len(rows))
	for _, r := range rows {
		pts = append(pts, chart.LinePoint{X: x(r), Y: y(r)})
	}
	return chart.LineSeries{Label: name, Points: pts}
}

// ToLineSeriesData is ToLineSeries pre-wrapped in a chart.LineSeriesData
// holding one series, the common case for a single deferred time-series
// widget. Set YLabel/ShowArea/MovingAvgWindow on the returned value before
// laying it out with chart.LayoutLine.
func ToLineSeriesData[T any](name string, rows []T, x func(T) string, y func(T) float64) chart.LineSeriesData {
	return chart.LineSeriesData{Series: []chart.LineSeries{ToLineSeries(name, rows, x, y)}}
}

// ToLineSeriesByEntity turns rows grouped by entity into a multi-line
// chart.LineSeriesData — one chart.LineSeries per distinct entity. It is the
// adapter for the per-entity bucket/rollup shapes (Bucket/Rollup with By set,
// or AsOf-over-time): name pulls the series name, x the bucket label, y the
// value. Series and points are taken in first-seen order, so an ORDER BY in the
// SQL (entity then time, as the builders emit) gives stable, monotone lines.
func ToLineSeriesByEntity[T any](rows []T, name func(T) string, x func(T) string, y func(T) float64) chart.LineSeriesData {
	var data chart.LineSeriesData
	idx := make(map[string]int) // entity name -> index into data.Series
	for _, r := range rows {
		n := name(r)
		i, ok := idx[n]
		if !ok {
			i = len(data.Series)
			idx[n] = i
			data.Series = append(data.Series, chart.LineSeries{Label: n})
		}
		data.Series[i].Points = append(data.Series[i].Points, chart.LinePoint{X: x(r), Y: y(r)})
	}
	return data
}
