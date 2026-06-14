// ClickHouse analytics: the wall's observation store. Postgres holds the posts
// and the gate (the system of record); ClickHouse holds the firehose — one
// append-only row per thing that happened (views, posts, votes, signups,
// logins), batched in off the request path and aggregated into the admin
// dashboard at /admin/analytics. ClickHouse is required (CLICKHOUSE_DSN), so the
// firehose is always on.
package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strconv"
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

// event is one hush_events row. The ch tags map fields to columns; the batcher
// AppendStructs them into the prepared insert. Meta mirrors the moderation
// Meta on posts (IP/Country/City/UA) — admin eyes only, like everything here.
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

// track records one event off r's request context. Fire-and-forget: Add never
// blocks. handle may be "" (an anonymous view), postID 0 (no post involved).
func (a *app) track(r *http.Request, kind, handle string, postID int64) {
	m := clientMeta(r)
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

// analyticsPage is the admin dashboard: a grid of deferred widgets, each a
// placeholder that @gets its fragment from one of the widget PageFuncs below
// (the pantry dashboard pattern).
func (a *app) analyticsPage(c *beach.Ctx) (beach.View, error) {
	handle, _, admin := a.acct(c)
	return beach.View{Page: analyticsView(handle, admin, a.tierOf(a.rep.score(handle)))}, nil
}

// anlWidget adapts a fragment-builder into a deferred-widget PageFunc: the
// fragment carries the placeholder's id so the patch morphs cleanly, and a
// direct navigation still renders the bare fragment instead of a 404. build
// runs per request — these are live queries, not cached fragments.
func anlWidget(id string, build func(c *beach.Ctx) (templ.Component, error)) beach.PageFunc {
	return func(c *beach.Ctx) (beach.View, error) {
		f, err := build(c)
		if err != nil {
			return beach.View{}, err
		}
		return beach.View{Page: f, Fragment: f, Target: id}, nil
	}
}

// dayCount is one per-day aggregate row. toDate() scans into time.Time,
// count() into uint64.
type dayCount struct {
	D time.Time `ch:"d"`
	N uint64    `ch:"n"`
}

// labelCount is one label+count ranking row (countries, IPs).
type labelCount struct {
	L string `ch:"l"`
	N uint64 `ch:"n"`
}

// recentEvent is one row of the recent-events table.
type recentEvent struct {
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
func (a *app) perDay(c *beach.Ctx, cond string, days int) ([]dayCount, error) {
	back := strconv.Itoa(days - 1) // today plus `back` days behind it = `days` buckets
	return ch.Rows[dayCount](c.Context(), a.chConn, `
		SELECT toDate(ts) AS d, count() AS n
		  FROM hush_events
		 WHERE toDate(ts) >= toDate(now()) - `+back+` AND `+cond+`
		 GROUP BY d ORDER BY d
		  WITH FILL FROM toDate(now()) - `+back+` TO toDate(now()) + 1`)
}

// dayLabel formats a per-day bucket for an axis ("Jun 2").
func dayLabel(r dayCount) string { return r.D.Format("Jan 2") }

// anlEvents is the headline widget: every event, per day, 14 days, as a line.
func (a *app) anlEvents(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.perDay(c, "1", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToLineSeriesData("events", rows, dayLabel, func(r dayCount) float64 { return float64(r.N) })
	in.ShowArea = true
	return chart.ChartLineFragment(chart.LayoutLine("anl-events", in, chart.LineOpts{RateUnit: "day"})), nil
}

// anlPosts is posts per day (text + image), 14 days, as bars.
func (a *app) anlPosts(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.perDay(c, "kind IN ('post_text', 'post_image')", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(rows, dayLabel, func(r dayCount) int { return int(r.N) })
	in.Unit = " posts"
	return chart.ChartBarFragment("anl-posts", chart.LayoutHBar(in)), nil
}

// anlVotes is votes per day (up + down), 14 days, as bars.
func (a *app) anlVotes(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.perDay(c, "kind IN ('vote_up', 'vote_down')", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(rows, dayLabel, func(r dayCount) int { return int(r.N) })
	in.Unit = " votes"
	return chart.ChartBarFragment("anl-votes", chart.LayoutHBar(in)), nil
}

// anlCountries ranks the last 30 days by country (Cloudflare's CF-IPCountry;
// empty in dev, so blank rows are excluded rather than charted as "").
func (a *app) anlCountries(c *beach.Ctx) (templ.Component, error) {
	rows, err := ch.Rows[labelCount](c.Context(), a.chConn, `
		SELECT country AS l, count() AS n
		  FROM hush_events
		 WHERE ts >= now() - INTERVAL 30 DAY AND country != ''
		 GROUP BY l ORDER BY n DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(rows, func(r labelCount) string { return r.L }, func(r labelCount) int { return int(r.N) })
	in.Unit = " events"
	return chart.ChartBarFragment("anl-countries", chart.LayoutHBar(in)), nil
}

// anlIPs ranks the last 30 days by source IP — a table, not a chart: the
// labels are the data here, and an admin reads them to feed a blocklist.
func (a *app) anlIPs(c *beach.Ctx) (templ.Component, error) {
	rows, err := ch.Rows[labelCount](c.Context(), a.chConn, `
		SELECT ip AS l, count() AS n
		  FROM hush_events
		 WHERE ts >= now() - INTERVAL 30 DAY AND ip != ''
		 GROUP BY l ORDER BY n DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	return ipsTable(rows), nil
}

// anlRecent is the raw tail: the last 100 events, newest first.
func (a *app) anlRecent(c *beach.Ctx) (templ.Component, error) {
	rows, err := ch.Rows[recentEvent](c.Context(), a.chConn, `
		SELECT ts, kind, handle, post_id, ip, country, path
		  FROM hush_events
		 ORDER BY ts DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	return recentTable(rows), nil
}
