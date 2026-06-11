package ch

// This file is the thin bridge between a ClickHouse query and a server-side
// chart. The query is the whole API: you write SQL that buckets and aggregates,
// scan its rows into a typed struct with ch.Rows, then hand the slice to one of
// the adapters below to get a chart.*Input you can lay out and render.
//
// There is deliberately no query DSL. Each adapter takes the row slice plus one
// or two small projector funcs that pull the label/value/x/y out of a row, so it
// works for any struct shape without reflection or struct-tag coupling.
//
//	type row struct {
//	    Rack string  `ch:"rack"`
//	    Draw float64 `ch:"draw"`
//	}
//	rows, _ := ch.Rows[row](ctx, conn,
//	    `SELECT rack, sum(watts)/1000 AS draw
//	       FROM power GROUP BY rack ORDER BY draw DESC`)
//	in := ch.ToHBarSeries(rows,
//	    func(r row) string { return r.Rack },
//	    func(r row) float64 { return r.Draw })
//	in.Title, in.Unit = "Rack draw", " kW"
//	chart.HBarSVG(out, chart.LayoutHBar(in))
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
//	ui.Defer(ui.DeferProps{ID: "rack-draw", Height: "180px", Get: "/widgets/rack-draw"})
//
//	// The widget route: a normal PageFunc. ch is optional, so a nil Conn yields
//	// an empty series and the chart renders blank rather than erroring.
//	func RackDraw(conn ch.Conn) beach.PageFunc {
//	    return func(c *beach.Ctx) (beach.View, error) {
//	        type row struct {
//	            Rack string  `ch:"rack"`
//	            Draw float64 `ch:"draw"`
//	        }
//	        rows, err := ch.Rows[row](c.Context(), conn,
//	            `SELECT rack, sum(watts)/1000 AS draw
//	               FROM power GROUP BY rack ORDER BY draw DESC LIMIT 10`)
//	        if err != nil {
//	            return nil, err
//	        }
//	        in := ch.ToHBarSeries(rows,
//	            func(r row) string { return r.Rack },
//	            func(r row) float64 { return r.Draw })
//	        in.Title, in.Unit = "Rack draw", " kW"
//	        var buf bytes.Buffer
//	        buf.WriteString(`<div id="rack-draw">`)
//	        if err := chart.HBarSVG(&buf, chart.LayoutHBar(in)); err != nil {
//	            return nil, err
//	        }
//	        buf.WriteString(`</div>`)
//	        // Wrap the SVG in the same #rack-draw element the placeholder used so
//	        // the patch morphs cleanly. templ.Raw makes it the Fragment; an empty
//	        // Target morphs by the fragment's own element id.
//	        return beach.View{Fragment: templ.Raw(buf.String())}, nil
//	    }
//	}
//
// The adapters import nothing from chart — they return chart's input structs by
// constructing them via the typed projectors, keeping ch free of a hard chart
// dependency. (They live in ch because ch.Rows is the upstream half of the
// bridge; the caller does the one chart.Layout*/chart.*SVG call.)

import "github.com/ThirdCoastInteractive/the-beach/chart"

// ToSeries turns query rows into a []chart.Datum (label + value), the input shape
// for bar, gauge, billboard, and bar-race charts. label and value pull the two
// fields out of each row; row order is preserved (so ORDER BY in SQL controls
// chart order).
func ToSeries[T any](rows []T, label func(T) string, value func(T) float64) []chart.Datum {
	out := make([]chart.Datum, 0, len(rows))
	for _, r := range rows {
		out = append(out, chart.Datum{Label: label(r), Value: value(r)})
	}
	return out
}

// ToHBarSeries is ToSeries pre-wrapped in a chart.HBarInput, the common case for
// a "top N by metric" ranking. Set Title/Unit/Max on the returned value before
// laying it out; the Series field is already populated.
func ToHBarSeries[T any](rows []T, label func(T) string, value func(T) float64) chart.HBarInput {
	return chart.HBarInput{Series: ToSeries(rows, label, value)}
}

// ToTimeSeries turns query rows into a single named chart.XYSeries, the input
// shape for line, area, and scatter charts. x is typically a bucketed time as a
// float (e.g. unix seconds from toUnixTimestamp, or the bucket ordinal); y is the
// aggregate. Rows are taken in order, so ORDER BY the time column in SQL.
//
// For a single series this is the whole story; wrap several ToTimeSeries results
// in a []chart.XYSeries for a multi-line chart.
func ToTimeSeries[T any](name string, rows []T, x func(T) float64, y func(T) float64) chart.XYSeries {
	pts := make([]chart.Point, 0, len(rows))
	for _, r := range rows {
		pts = append(pts, chart.Point{X: x(r), Y: y(r)})
	}
	return chart.XYSeries{Name: name, Points: pts}
}

// ToLineInput is ToTimeSeries pre-wrapped in a chart.LineInput holding one
// series, the common case for a single deferred time-series widget. Set
// Title/Area/bounds on the returned value before laying it out.
func ToLineInput[T any](name string, rows []T, x func(T) float64, y func(T) float64) chart.LineInput {
	return chart.LineInput{Series: []chart.XYSeries{ToTimeSeries(name, rows, x, y)}}
}
