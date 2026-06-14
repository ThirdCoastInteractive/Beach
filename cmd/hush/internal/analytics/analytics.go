// Package analytics is the wall's observation store. Postgres holds the posts
// and the gate (the system of record); ClickHouse holds the firehose — one
// append-only row per thing that happened (views, posts, votes, signups,
// logins), batched in off the request path and aggregated into the admin
// dashboard at /admin/analytics. ClickHouse is required (CLICKHOUSE_DSN), so the
// firehose is always on. The web layer owns the dashboard's shell and tables;
// this package owns the events, the migrations, and the live aggregate queries.
package analytics

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/store"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ch"
	"github.com/ThirdCoastInteractive/the-beach/pkg/chart"

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

// event is one hush_events row. The ch tags map fields to columns; the batcher
// AppendStructs them into the prepared insert. Mirrors the moderation Meta on
// posts (IP/Country/City/UA) — admin eyes only, like everything here.
type event struct {
	TS      time.Time `ch:"ts"`
	Kind    string    `ch:"kind"`
	Handle  string    `ch:"handle"`
	PostID  int64     `ch:"post_id"`
	IP      string    `ch:"ip"`
	Country string    `ch:"country"`
	City    string    `ch:"city"`
	UA      string    `ch:"ua"`
	Path    string    `ch:"path"`
}

// Analytics wraps the ClickHouse connection and the event batcher.
type Analytics struct {
	conn   ch.Conn
	events *ch.Batcher[event]
}

// New connects to ClickHouse, runs the migrations, and opens the event batcher.
func New(ctx context.Context, dsn string) (*Analytics, error) {
	conn, err := ch.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := ch.Migrate(ctx, conn, Migrations()); err != nil {
		return nil, err
	}
	return &Analytics{
		conn:   conn,
		events: ch.NewBatcher[event](conn, "hush_events", ch.Batch{}),
	}, nil
}

// Track records one event off r's request context. Fire-and-forget: Add never
// blocks. handle may be "" (an anonymous view), postID 0 (no post involved).
func (a *Analytics) Track(r *http.Request, kind, handle string, postID int64) {
	m := store.ClientMeta(r)
	a.events.Add(event{
		TS:      time.Now(),
		Kind:    kind,
		Handle:  handle,
		PostID:  postID,
		IP:      m.IP,
		Country: m.Country,
		City:    m.City,
		UA:      m.UA,
		Path:    r.URL.Path,
	})
}

// dayCount is one per-day aggregate row. toDate() scans into time.Time,
// count() into uint64.
type dayCount struct {
	D time.Time `ch:"d"`
	N uint64    `ch:"n"`
}

// LabelCount is one label+count ranking row (countries, IPs).
type LabelCount struct {
	L string `ch:"l"`
	N uint64 `ch:"n"`
}

// RecentEvent is one row of the recent-events table.
type RecentEvent struct {
	TS      time.Time `ch:"ts"`
	Kind    string    `ch:"kind"`
	Handle  string    `ch:"handle"`
	PostID  int64     `ch:"post_id"`
	IP      string    `ch:"ip"`
	Country string    `ch:"country"`
	Path    string    `ch:"path"`
}

// perDay buckets the last `days` calendar days (today included) of events
// matching cond into zero-filled per-day counts — WITH FILL closes the gaps so
// quiet days chart as zero instead of vanishing. cond is a trusted SQL snippet
// from the callers below, never user input.
func (a *Analytics) perDay(ctx context.Context, cond string, days int) ([]dayCount, error) {
	back := strconv.Itoa(days - 1) // today plus `back` days behind it = `days` buckets
	return ch.Rows[dayCount](ctx, a.conn, `
		SELECT toDate(ts) AS d, count() AS n
		  FROM hush_events
		 WHERE toDate(ts) >= toDate(now()) - `+back+` AND `+cond+`
		 GROUP BY d ORDER BY d
		  WITH FILL FROM toDate(now()) - `+back+` TO toDate(now()) + 1`)
}

// dayLabel formats a per-day bucket for an axis ("Jun 2").
func dayLabel(r dayCount) string { return r.D.Format("Jan 2") }

// Events is the headline widget: every event, per day, 14 days, as a line.
func (a *Analytics) Events(ctx context.Context) (templ.Component, error) {
	rows, err := a.perDay(ctx, "1", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToLineSeriesData("events", rows, dayLabel, func(r dayCount) float64 { return float64(r.N) })
	in.ShowArea = true
	return chart.ChartLineFragment(chart.LayoutLine("anl-events", in, chart.LineOpts{RateUnit: "day"})), nil
}

// Posts is posts per day (text + image), 14 days, as bars.
func (a *Analytics) Posts(ctx context.Context) (templ.Component, error) {
	rows, err := a.perDay(ctx, "kind IN ('post_text', 'post_image')", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(rows, dayLabel, func(r dayCount) int { return int(r.N) })
	in.Unit = " posts"
	return chart.ChartBarFragment("anl-posts", chart.LayoutHBar(in)), nil
}

// Votes is votes per day (up + down), 14 days, as bars.
func (a *Analytics) Votes(ctx context.Context) (templ.Component, error) {
	rows, err := a.perDay(ctx, "kind IN ('vote_up', 'vote_down')", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(rows, dayLabel, func(r dayCount) int { return int(r.N) })
	in.Unit = " votes"
	return chart.ChartBarFragment("anl-votes", chart.LayoutHBar(in)), nil
}

// Countries ranks the last 30 days by country (Cloudflare's CF-IPCountry; empty
// in dev, so blank rows are excluded rather than charted as "").
func (a *Analytics) Countries(ctx context.Context) (templ.Component, error) {
	rows, err := ch.Rows[LabelCount](ctx, a.conn, `
		SELECT country AS l, count() AS n
		  FROM hush_events
		 WHERE ts >= now() - INTERVAL 30 DAY AND country != ''
		 GROUP BY l ORDER BY n DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(rows, func(r LabelCount) string { return r.L }, func(r LabelCount) int { return int(r.N) })
	in.Unit = " events"
	return chart.ChartBarFragment("anl-countries", chart.LayoutHBar(in)), nil
}

// IPs ranks the last 30 days by source IP — the web layer renders these as a
// table, not a chart: the labels are the data here, and an admin reads them to
// feed a blocklist.
func (a *Analytics) IPs(ctx context.Context) ([]LabelCount, error) {
	return ch.Rows[LabelCount](ctx, a.conn, `
		SELECT ip AS l, count() AS n
		  FROM hush_events
		 WHERE ts >= now() - INTERVAL 30 DAY AND ip != ''
		 GROUP BY l ORDER BY n DESC LIMIT 20`)
}

// Recent is the raw tail: the last 100 events, newest first.
func (a *Analytics) Recent(ctx context.Context) ([]RecentEvent, error) {
	return ch.Rows[RecentEvent](ctx, a.conn, `
		SELECT ts, kind, handle, post_id, ip, country, path
		  FROM hush_events
		 ORDER BY ts DESC LIMIT 100`)
}
