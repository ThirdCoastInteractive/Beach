package main

import (
	"strconv"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/chart"
	"github.com/a-h/templ"
)

// The dashboard widgets are fed by sample, in-memory analytics so they render
// without a ClickHouse store. In the ch showcase these inputs would come from
// ch.Rows[T](...) over toStartOfInterval aggregates; the chart.Layout* call
// and templ fragment at the end are identical either way (see 14-analytics.md).

// widgetPage adapts a chart fragment into a deferred-widget PageFunc. frag
// runs per request; the fragment carries the placeholder's id so the Datastar
// patch morphs cleanly, and a direct navigation still renders the bare chart
// rather than a 404.
func widgetPage(id string, frag func() templ.Component) beach.PageFunc {
	return func(c *beach.Ctx) (beach.View, error) {
		f := frag()
		return beach.View{Page: f, Fragment: f, Target: id}, nil
	}
}

// spendWidget is the spend-over-time line chart: 12 weeks of household
// grocery spend.
func spendWidget() templ.Component {
	weeks := []float64{84, 92, 78, 105, 96, 88, 120, 99, 110, 102, 94, 130}
	pts := make([]chart.LinePoint, len(weeks))
	for i, v := range weeks {
		pts[i] = chart.LinePoint{X: "Wk " + strconv.Itoa(i+1), Y: v, T: float64(i + 1)}
	}
	lay := chart.LayoutLine("w-spend", chart.LineSeriesData{
		Series:   []chart.LineSeries{{Label: "Weekly spend", Points: pts}},
		YLabel:   "$",
		ShowArea: true,
	}, chart.LineOpts{RateUnit: "week"})
	return chart.ChartLineFragment(lay)
}

// categoryWidget is the spend-by-category chart: one stacked bar per week,
// one segment per grocery category.
func categoryWidget() templ.Component {
	lay := chart.LayoutStackedBar(chart.StackedBarSeries{
		Unit: "$",
		Series: []chart.StackedBarRow{
			{Label: "Wk 1", Segments: []chart.StackedBarSegment{
				{Label: "Produce", Value: 32}, {Label: "Dairy", Value: 18},
				{Label: "Dry goods", Value: 24}, {Label: "Frozen", Value: 10},
			}},
			{Label: "Wk 2", Segments: []chart.StackedBarSegment{
				{Label: "Produce", Value: 28}, {Label: "Dairy", Value: 22},
				{Label: "Dry goods", Value: 20}, {Label: "Frozen", Value: 14},
			}},
			{Label: "Wk 3", Segments: []chart.StackedBarSegment{
				{Label: "Produce", Value: 35}, {Label: "Dairy", Value: 16},
				{Label: "Dry goods", Value: 26}, {Label: "Frozen", Value: 12},
			}},
			{Label: "Wk 4", Segments: []chart.StackedBarSegment{
				{Label: "Produce", Value: 30}, {Label: "Dairy", Value: 20},
				{Label: "Dry goods", Value: 28}, {Label: "Frozen", Value: 16},
			}},
		},
	})
	return chart.ChartStackedBarFragment("w-category", lay)
}

// expiryWidget is the expiry calendar heatmap for the current year: every day
// zero-filled so the grid reads as a calendar, with the seeded items' expiry
// dates (plus a little ambient density) lit up.
func expiryWidget(items []Item) templ.Component {
	year := time.Now().Year()

	byDate := map[string]float64{}
	for _, it := range items {
		if it.ExpiresAt != nil && it.ExpiresAt.Year() == year {
			byDate[it.ExpiresAt.Format("2006-01-02")] += it.Quantity
		}
	}
	base := time.Now()
	for d := 0; d < 60; d += 3 {
		t := base.AddDate(0, 0, d)
		if t.Year() == year {
			byDate[t.Format("2006-01-02")] += float64((d % 5) + 1)
		}
	}

	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	days := make([]chart.CalendarDay, 0, 366)
	week := 0
	for t := start; t.Before(end); t = t.AddDate(0, 0, 1) {
		dow := int(t.Weekday())
		key := t.Format("2006-01-02")
		days = append(days, chart.CalendarDay{
			Date:  key,
			Value: byDate[key],
			Year:  year,
			Month: int(t.Month()),
			Day:   t.Day(),
			DOW:   dow,
			Week:  week,
		})
		if dow == 6 { // a new layout column starts after each Saturday
			week++
		}
	}
	lay := chart.LayoutCalendar(chart.CalendarData{Days: days, Unit: "items"})
	return chart.ChartCalendarFragment("w-expiry", lay)
}

// wasteWidget is the food-waste gauge (percent of purchases thrown out).
func wasteWidget() templ.Component {
	return chart.ChartGaugeFragment("w-waste", chart.Gauge{
		Value:    "12%",
		Pct:      0.12,
		Subtitle: "Wasted this month",
	})
}
