package web

import (
	"fmt"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ch"
	"github.com/ThirdCoastInteractive/Beach/pkg/chart"
	"github.com/a-h/templ"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/analytics"
)

// The dashboard widgets read real data: the activity line comes from the
// ClickHouse firehose (pantry_events), the category/expiry/waste widgets from
// Postgres (the system of record). Each is a deferred widget — a per-request
// query behind ui.Defer — so first paint never blocks on a chart.

// anlWidget adapts a fragment-builder into a deferred-widget PageFunc: the
// fragment carries the placeholder's id so the patch morphs cleanly, and a
// direct navigation still renders the bare fragment instead of a 404. build runs
// per request — these are live queries, not cached fragments.
func anlWidget(id string, build func(c *beach.Ctx) (templ.Component, error)) beach.PageFunc {
	return func(c *beach.Ctx) (beach.View, error) {
		f, err := build(c)
		if err != nil {
			return beach.View{}, err
		}
		return beach.View{Page: f, Fragment: f, Target: id}, nil
	}
}

// wActivity is the activity line: items added per day for 14 days, from the
// ClickHouse firehose. The firehose is the only "events over time" source; the
// other widgets read inventory state from Postgres.
func (a *app) wActivity(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.anl.ActivityPerDay(c.Context(), 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToLineSeriesData("items added", rows, analytics.DayLabel, func(r analytics.DayCount) float64 { return float64(r.N) })
	in.ShowArea = true
	return chart.ChartLineFragment(chart.LayoutLine("w-spend", in, chart.LineOpts{RateUnit: "day"})), nil
}

// catRow is one (location-kind, category, count) aggregate row.
type catRow struct {
	LocKind  string
	Category string
	N        int
}

// wCategory is inventory composition: one stacked bar per storage-location kind,
// one segment per grocery category. Read from Postgres — current inventory, not
// history.
func (a *app) wCategory(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.store.Pool().Query(c.Context(), `
		SELECT COALESCE(l.kind, 'other') AS loc_kind, i.category, count(*) AS n
		  FROM pantry_items i
		  LEFT JOIN pantry_locations l ON l.id = i.location_id
		 WHERE i.deleted_at IS NULL
		 GROUP BY loc_kind, i.category
		 ORDER BY loc_kind, i.category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Group rows into one StackedBarRow per location kind, in first-seen order.
	var bars []chart.StackedBarRow
	idx := map[string]int{}
	for rows.Next() {
		var r catRow
		if err := rows.Scan(&r.LocKind, &r.Category, &r.N); err != nil {
			return nil, err
		}
		i, ok := idx[r.LocKind]
		if !ok {
			bars = append(bars, chart.StackedBarRow{Label: r.LocKind})
			i = len(bars) - 1
			idx[r.LocKind] = i
		}
		bars[i].Segments = append(bars[i].Segments, chart.StackedBarSegment{Label: r.Category, Value: r.N})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chart.ChartStackedBarFragment("w-category", chart.LayoutStackedBar(chart.StackedBarSeries{Unit: " items", Series: bars})), nil
}

// wExpiry is the expiry calendar heatmap for the current year: every day
// zero-filled so the grid reads as a calendar, with real per-day expiry counts
// from Postgres lit up.
func (a *app) wExpiry(c *beach.Ctx) (templ.Component, error) {
	year := time.Now().Year()

	rows, err := a.store.Pool().Query(c.Context(), `
		SELECT expires_at::date AS d, count(*) AS n
		  FROM pantry_items
		 WHERE deleted_at IS NULL AND expires_at IS NOT NULL
		 GROUP BY d`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDate := map[string]float64{}
	for rows.Next() {
		var d time.Time
		var n int64
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		if d.Year() == year {
			byDate[d.Format("2006-01-02")] = float64(n)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	days := make([]chart.CalendarDay, 0, 366)
	week := 0
	for t := start; t.Before(end); t = t.AddDate(0, 0, 1) {
		dow := int(t.Weekday())
		key := t.Format("2006-01-02")
		days = append(days, chart.CalendarDay{
			Date: key, Value: byDate[key], Year: year,
			Month: int(t.Month()), Day: t.Day(), DOW: dow, Week: week,
		})
		if dow == 6 { // a new layout column starts after each Saturday
			week++
		}
	}
	return chart.ChartCalendarFragment("w-expiry", chart.LayoutCalendar(chart.CalendarData{Days: days, Unit: "items"})), nil
}

// wWaste is the spoilage gauge: the fraction of current inventory already past
// its expiry date (a real waste proxy), from Postgres.
func (a *app) wWaste(c *beach.Ctx) (templ.Component, error) {
	var expired, total int
	if err := a.store.Pool().QueryRow(c.Context(), `
		SELECT count(*) FILTER (WHERE expires_at IS NOT NULL AND expires_at < current_date),
		       count(*)
		  FROM pantry_items
		 WHERE deleted_at IS NULL`).Scan(&expired, &total); err != nil {
		return nil, err
	}
	var pct float64
	if total > 0 {
		pct = float64(expired) / float64(total)
	}
	return chart.ChartGaugeFragment("w-waste", chart.Gauge{
		Value:    fmt.Sprintf("%d%%", int(pct*100+0.5)),
		Pct:      pct,
		Subtitle: "Expired of total",
	}), nil
}
