// Package analytics is the game's ClickHouse action firehose and the /stats
// widgets over it. The live game lives in the in-memory ecs.Store and the
// durable copy in Postgres; ClickHouse holds one append-only row per thing a
// player did (join, roll, buy, rent, tax, chance, pass GO), emitted non-blocking
// from inside the sim loop and aggregated into the public /stats page.
// ClickHouse is required (CLICKHOUSE_DSN), so the firehose is always on.
package analytics

import (
	"embed"
	"io/fs"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ch"
	"github.com/ThirdCoastInteractive/Beach/pkg/chart"

	"github.com/a-h/templ"
)

//go:embed chmigrations/*.sql
var chMigrationsFS embed.FS

// Migrations re-roots the embed so the .sql files sit at the FS root, where
// ch.Migrate's goose provider looks for them.
func Migrations() fs.FS {
	sub, err := fs.Sub(chMigrationsFS, "chmigrations")
	if err != nil {
		panic(err) // unreachable: the embed guarantees the directory exists
	}
	return sub
}

// Event is one boardwalk_events row. The ch tags map fields to columns; the
// batcher AppendStructs them into the prepared insert. Seat/Tile are -1/0 when
// not applicable; Delta is the cash change (negative for a payment).
type Event struct {
	TS    time.Time `ch:"ts"`
	Kind  string    `ch:"kind"`  // join | roll | pass_go | buy | rent | tax | chance
	Seat  int32     `ch:"seat"`  // acting seat index
	Token string    `ch:"token"` // acting player's piece glyph
	Tile  int32     `ch:"tile"`  // board square index involved
	Name  string    `ch:"name"`  // tile name involved
	Delta int64     `ch:"delta"` // cash change in dollars (negative = paid)
}

// CashBar is one live-standings bar: a seat label and its current cash. The web
// layer supplies these (read off the sim loop) so analytics never imports game.
type CashBar struct {
	Label string
	Cash  int
}

// Stats holds the dependencies the /stats widgets need: the ClickHouse
// connection the rolls/tiles widgets query, and a CashBars hook that reads the
// live standings off the sim loop. main wires CashBars from game.Snapshot so
// this package stays free of any game import.
type Stats struct {
	Conn     ch.Conn
	CashBars func(c *beach.Ctx) ([]CashBar, error)
}

// Widget adapts a fragment-builder into a deferred-widget PageFunc: the
// fragment carries the placeholder's id so the patch morphs cleanly, and a
// direct navigation still renders the bare fragment instead of a 404. build runs
// per request — these are live queries, not cached fragments.
func Widget(id string, build func(c *beach.Ctx) (templ.Component, error)) beach.PageFunc {
	return func(c *beach.Ctx) (beach.View, error) {
		f, err := build(c)
		if err != nil {
			return beach.View{}, err
		}
		return beach.View{Page: f, Fragment: f, Target: id}, nil
	}
}

// dayCount is one per-day aggregate row. toDate() scans into time.Time, count()
// into uint64.
type dayCount struct {
	D time.Time `ch:"d"`
	N uint64    `ch:"n"`
}

// labelCount is one label+count ranking row (busiest tiles).
type labelCount struct {
	L string `ch:"l"`
	N uint64 `ch:"n"`
}

// dayLabel formats a per-day bucket for an axis ("Jun 2").
func dayLabel(r dayCount) string { return r.D.Format("Jan 2") }

// Rolls is the headline widget: rolls per day for 14 days, as a line. WITH
// FILL closes the gaps so quiet days chart as zero instead of vanishing.
func (s *Stats) Rolls(c *beach.Ctx) (templ.Component, error) {
	rows, err := ch.Rows[dayCount](c.Context(), s.Conn, `
		SELECT toDate(ts) AS d, count() AS n
		  FROM boardwalk_events
		 WHERE kind = 'roll' AND toDate(ts) >= toDate(now()) - 13
		 GROUP BY d ORDER BY d
		  WITH FILL FROM toDate(now()) - 13 TO toDate(now()) + 1`)
	if err != nil {
		return nil, err
	}
	in := ch.ToLineSeriesData("rolls", rows, dayLabel, func(r dayCount) float64 { return float64(r.N) })
	in.ShowArea = true
	return chart.ChartLineFragment(chart.LayoutLine("anl-rolls", in, chart.LineOpts{RateUnit: "day"})), nil
}

// Tiles ranks the most-landed-on tiles over all time — where the action is.
func (s *Stats) Tiles(c *beach.Ctx) (templ.Component, error) {
	rows, err := ch.Rows[labelCount](c.Context(), s.Conn, `
		SELECT name AS l, count() AS n
		  FROM boardwalk_events
		 WHERE kind = 'roll' AND name != ''
		 GROUP BY l ORDER BY n DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(rows, func(r labelCount) string { return r.L }, func(r labelCount) int { return int(r.N) })
	in.Unit = " landings"
	return chart.ChartBarFragment("anl-tiles", chart.LayoutHBar(in)), nil
}

// Cash is the live standings, read off the loop (not from ClickHouse): each
// seat's current cash as a bar. The firehose charts history; this one shows the
// board right now. CashBars supplies the standings so analytics never reads the
// store directly.
func (s *Stats) Cash(c *beach.Ctx) (templ.Component, error) {
	bars, err := s.CashBars(c)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(bars, func(b CashBar) string { return b.Label }, func(b CashBar) int { return b.Cash })
	in.Unit = " cash"
	return chart.ChartBarFragment("anl-cash", chart.LayoutHBar(in)), nil
}
