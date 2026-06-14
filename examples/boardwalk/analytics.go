// ClickHouse analytics: the game's action firehose. The live game lives in the
// in-memory ecs.Store and the durable copy in Postgres; ClickHouse holds one
// append-only row per thing a player did (join, roll, buy, rent, tax, chance,
// pass GO), emitted non-blocking from inside the sim loop and aggregated into
// the public /stats page. ClickHouse is required (CLICKHOUSE_DSN), so the
// firehose is always on.
package main

import (
	"embed"
	"io/fs"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ch"
	"github.com/ThirdCoastInteractive/the-beach/pkg/chart"

	"github.com/a-h/templ"
)

//go:embed chmigrations/*.sql
var chMigrationsFS embed.FS

// chMigrations re-roots the embed so the .sql files sit at the FS root, where
// ch.Migrate's goose provider looks for them.
func chMigrations() fs.FS {
	sub, err := fs.Sub(chMigrationsFS, "chmigrations")
	if err != nil {
		panic(err) // unreachable: the embed guarantees the directory exists
	}
	return sub
}

// event is one boardwalk_events row. The ch tags map fields to columns; the
// batcher AppendStructs them into the prepared insert. Seat/Tile are -1/0 when
// not applicable; Delta is the cash change (negative for a payment).
type event struct {
	TS    time.Time `ch:"ts"`
	Kind  string    `ch:"kind"`  // join | roll | pass_go | buy | rent | tax | chance
	Seat  int32     `ch:"seat"`  // acting seat index
	Token string    `ch:"token"` // acting player's piece glyph
	Tile  int32     `ch:"tile"`  // board square index involved
	Name  string    `ch:"name"`  // tile name involved
	Delta int64     `ch:"delta"` // cash change in dollars (negative = paid)
}

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

// anlRolls is the headline widget: rolls per day for 14 days, as a line. WITH
// FILL closes the gaps so quiet days chart as zero instead of vanishing.
func (s *server) anlRolls(c *beach.Ctx) (templ.Component, error) {
	rows, err := ch.Rows[dayCount](c.Context(), s.chConn, `
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

// anlTiles ranks the most-landed-on tiles over all time — where the action is.
func (s *server) anlTiles(c *beach.Ctx) (templ.Component, error) {
	rows, err := ch.Rows[labelCount](c.Context(), s.chConn, `
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

// anlCash is the live standings, read off the loop (not from ClickHouse): each
// seat's current cash as a bar. The firehose charts history; this one shows the
// board right now.
func (s *server) anlCash(c *beach.Ctx) (templ.Component, error) {
	snap, err := s.game.Snapshot(c.Context())
	if err != nil {
		return nil, err
	}
	type bar struct {
		label string
		cash  int
	}
	bars := make([]bar, 0, len(snap.players))
	for _, p := range snap.players {
		if p.Spec {
			continue
		}
		bars = append(bars, bar{label: p.Token + " " + p.Name, cash: p.Cash})
	}
	in := ch.ToHBarSeries(bars, func(b bar) string { return b.label }, func(b bar) int { return b.cash })
	in.Unit = " cash"
	return chart.ChartBarFragment("anl-cash", chart.LayoutHBar(in)), nil
}
