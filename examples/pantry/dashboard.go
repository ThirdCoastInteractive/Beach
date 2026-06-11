package main

import (
	"bytes"
	"time"

	"github.com/ThirdCoastInteractive/the-beach/chart"
)

// The dashboard widgets are fed by sample, in-memory analytics so they render
// without a ClickHouse store. In the ch showcase these slices would come from
// ch.Rows[T](...) over toStartOfInterval aggregates; the chart.Layout*/*SVG call
// at the end is identical either way (see ch.To* adapters and 14-analytics.md).

// spendSVG renders the spend-over-time line chart.
func spendSVG() string {
	// 12 weeks of household grocery spend.
	weeks := []float64{84, 92, 78, 105, 96, 88, 120, 99, 110, 102, 94, 130}
	pts := make([]chart.Point, len(weeks))
	for i, v := range weeks {
		pts[i] = chart.Point{X: float64(i + 1), Y: v}
	}
	lay := chart.LayoutLine(chart.LineInput{
		Title:  "Spend over time",
		Area:   true,
		Width:  520,
		Height: 240,
		Series: []chart.XYSeries{{Name: "Weekly spend", Points: pts}},
	})
	return renderSVG(func(b *bytes.Buffer) error { return chart.LineSVG(b, lay) })
}

// categorySVG renders the category stacked bar chart.
func categorySVG() string {
	lay := chart.LayoutStackedBar(chart.StackedBarInput{
		Title:      "Spend by category",
		Width:      520,
		Height:     240,
		Categories: []string{"Wk1", "Wk2", "Wk3", "Wk4"},
		Series: []chart.Datum2{
			{Name: "Produce", Values: []float64{32, 28, 35, 30}},
			{Name: "Dairy", Values: []float64{18, 22, 16, 20}},
			{Name: "Dry goods", Values: []float64{24, 20, 26, 28}},
			{Name: "Frozen", Values: []float64{10, 14, 12, 16}},
		},
	})
	return renderSVG(func(b *bytes.Buffer) error { return chart.StackedBarSVG(b, lay) })
}

// expirySVG renders the expiry calendar heatmap for the current year, with the
// seeded items' expiry dates lit up.
func expirySVG(items []Item) string {
	days := map[string]float64{}
	for _, it := range items {
		if it.ExpiresAt != nil {
			days[it.ExpiresAt.Format("2006-01-02")] += it.Quantity
		}
	}
	// A little ambient density so the heatmap reads as a calendar, not two dots.
	base := time.Now()
	for d := 0; d < 60; d += 3 {
		days[base.AddDate(0, 0, d).Format("2006-01-02")] += float64((d % 5) + 1)
	}
	lay := chart.LayoutCalendarHeatmap(chart.CalendarInput{
		Title: "Expiry calendar",
		Year:  base.Year(),
		Days:  days,
	})
	return renderSVG(func(b *bytes.Buffer) error { return chart.CalendarHeatmapSVG(b, lay) })
}

// wasteSVG renders the food-waste gauge (percent of purchases thrown out).
func wasteSVG() string {
	lay := chart.LayoutGauge(chart.GaugeInput{
		Title: "Food waste",
		Label: "Wasted this month",
		Value: 12,
		Min:   0,
		Max:   100,
		Unit:  "%",
		Size:  220,
	})
	return renderSVG(func(b *bytes.Buffer) error { return chart.GaugeSVG(b, lay) })
}

// renderSVG runs an SVG-writing func into a string; an error yields "" so a
// widget renders blank rather than failing the page.
func renderSVG(fn func(*bytes.Buffer) error) string {
	var b bytes.Buffer
	if err := fn(&b); err != nil {
		return ""
	}
	return b.String()
}
