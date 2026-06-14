// ClickHouse analytics: pantry's activity firehose. Postgres holds the
// inventory (the system of record); ClickHouse holds one append-only row per
// thing that happened (items added, locations added, inventory views), batched
// in off the request path and charted on the dashboard's activity line. The
// inventory-derived widgets (category, expiry, waste) read Postgres directly —
// CH is the firehose, PG is the record. ClickHouse is required, so the firehose
// is always on.
package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/ThirdCoastInteractive/the-beach/pkg/ch"
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

// event is one pantry_events row. The ch tags map fields to columns; the batcher
// AppendStructs them into the prepared insert.
type event struct {
	TS       time.Time `ch:"ts"`
	Kind     string    `ch:"kind"` // item_added | location_added | item_viewed
	ItemName string    `ch:"item_name"`
	Category string    `ch:"category"`
	Quantity float64   `ch:"quantity"`
	Path     string    `ch:"path"`
}

// track records one event off r's request context. Fire-and-forget: Add never
// blocks. ts and path are filled here so callers only set the domain fields.
func (a *app) track(r *http.Request, e event) {
	e.TS = time.Now()
	e.Path = r.URL.Path
	a.events.Add(e)
}

// dayCount is one per-day aggregate row (the CH activity line).
type dayCount struct {
	D time.Time `ch:"d"`
	N uint64    `ch:"n"`
}

// dayLabel formats a per-day bucket for an axis ("Jun 2").
func dayLabel(r dayCount) string { return r.D.Format("Jan 2") }

// activityPerDay buckets the last `days` calendar days of item_added events into
// zero-filled per-day counts (WITH FILL closes the gaps so quiet days chart as
// zero), the headline of the dashboard's activity line.
func (a *app) activityPerDay(ctx context.Context, days int) ([]dayCount, error) {
	back := strconv.Itoa(days - 1)
	return ch.Rows[dayCount](ctx, a.chConn, `
		SELECT toDate(ts) AS d, count() AS n
		  FROM pantry_events
		 WHERE toDate(ts) >= toDate(now()) - `+back+` AND kind = 'item_added'
		 GROUP BY d ORDER BY d
		  WITH FILL FROM toDate(now()) - `+back+` TO toDate(now()) + 1`)
}
