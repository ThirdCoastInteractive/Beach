// Package analytics is driftbottle's off-the-hot-path persistence and analytics.
// The live matchmaking and the rolling feed are in-memory (package chat) — that
// is the fan-out benchmark and nothing here may touch its latency. This package
// holds:
//
//   - the ClickHouse firehose: one append-only `event` row per thing that
//     happened, batched in fire-and-forget (Track), aggregated only by /stats;
//   - the Postgres transcript archive: pairings + messages written by a
//     background writer goroutine (persistLoop) draining a buffered channel,
//     never inline in a hot-path handler;
//   - the /stats widgets: per-day aggregates read straight from ClickHouse.
//
// Every entry point is nil-safe: the unit tests build the chat server with a nil
// Analytics, so Track and the persist queue are simply never called there. A
// zero-value Analytics here (events/pool nil) is also safe should one be
// constructed without wiring.
package analytics

import (
	"context"
	"embed"
	"io/fs"
	"strconv"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ch"
	"github.com/ThirdCoastInteractive/Beach/pkg/chart"

	"github.com/jackc/pgx/v5/pgxpool"

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

// Event is one driftbottle_events row. The ch tags map fields to columns; the
// batcher AppendStructs them into the prepared insert. Kind is one of
// session | queued | paired | message | unpaired; Len is the message length
// (0 for non-message kinds), Pair the private pair topic when one is involved.
type Event struct {
	TS   time.Time `ch:"ts"`
	Kind string    `ch:"kind"`
	SID  string    `ch:"sid"`
	Pair string    `ch:"pair"`
	Len  uint32    `ch:"len"`
}

// Analytics is the off-the-hot-path sink the chat server fires events and
// archive ops at. It owns the ClickHouse firehose batcher, the Postgres
// transcript pool, and the buffered hand-off channel its writer goroutine
// drains. It also serves the /stats widgets that query the firehose.
type Analytics struct {
	pool    *pgxpool.Pool
	chConn  ch.Conn
	events  *ch.Batcher[Event]
	persist chan persistOp
}

// New builds an Analytics over an already-connected Postgres pool and ClickHouse
// handle. It creates the firehose batcher and the buffered persist channel; call
// Run to start the background archive writer.
func New(pool *pgxpool.Pool, conn ch.Conn) *Analytics {
	return &Analytics{
		pool:    pool,
		chConn:  conn,
		events:  ch.NewBatcher[Event](conn, "driftbottle_events", ch.Batch{}),
		persist: make(chan persistOp, 1024),
	}
}

// Run starts the single archive writer goroutine: it drains the persist channel
// and does the Postgres IO the hot-path handlers deliberately skip. It runs for
// the life of the process; ctx cancellation stops it.
func (a *Analytics) Run(ctx context.Context) {
	go a.persistLoop(ctx)
}

// Track records one event on the firehose. Fire-and-forget and nil-safe: with no
// ClickHouse wired it is a no-op, and Add itself never blocks — the hot path is
// never delayed by it.
func (a *Analytics) Track(e Event) {
	if a == nil || a.events == nil {
		return
	}
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	a.events.Add(e)
}

// --- archive op helpers (called from the chat hot path) ----------------------

// Pair queues the archive write for a new pairing.
func (a *Analytics) Pair(pair, aSID, bSID string) {
	a.enqueuePersist(persistOp{Op: "pair", Pair: pair, A: aSID, B: bSID})
}

// Unpair queues the archive close-out (ended_at) for a pairing.
func (a *Analytics) Unpair(pair string) {
	a.enqueuePersist(persistOp{Op: "unpair", Pair: pair})
}

// Message queues the archive write for one message body.
func (a *Analytics) Message(pair, from, body string) {
	a.enqueuePersist(persistOp{Op: "message", Pair: pair, From: from, Body: body})
}

// --- Postgres transcript archive (off the hot path) --------------------------

// persistOp is one unit of archive work handed to persistLoop. Op is the verb;
// the rest are the columns each verb needs.
type persistOp struct {
	Op   string // "pair" | "unpair" | "message"
	Pair string // pair topic (all ops)
	A, B string // the two session ids (pair)
	From string // sender session id (message)
	Body string // message body (message)
}

// enqueuePersist hands op to the background writer without blocking. Nil-safe
// (no pool / no channel ⇒ drop) and non-blocking (a full buffer drops the op
// rather than stalling the hot path): the archive is best-effort, the benchmark
// is sacred.
func (a *Analytics) enqueuePersist(op persistOp) {
	if a == nil || a.pool == nil || a.persist == nil {
		return
	}
	select {
	case a.persist <- op:
	default: // buffer full — shed the archive write, never block the caller
	}
}

// persistLoop is the single writer goroutine: it drains the persist channel and
// does the Postgres IO that the hot-path handlers deliberately skip.
func (a *Analytics) persistLoop(ctx context.Context) {
	if a.pool == nil || a.persist == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case op := <-a.persist:
			a.applyPersist(ctx, op)
		}
	}
}

// applyPersist performs one archive write. Errors are swallowed: a failed
// transcript write must never take down the live wall.
func (a *Analytics) applyPersist(ctx context.Context, op persistOp) {
	switch op.Op {
	case "pair":
		_, _ = a.pool.Exec(ctx,
			`INSERT INTO db_pairings (pair_topic, a_sid, b_sid) VALUES ($1, $2, $3)`,
			op.Pair, op.A, op.B)
	case "unpair":
		// Stamp the most recent open pairing for this topic.
		_, _ = a.pool.Exec(ctx,
			`UPDATE db_pairings SET ended_at = now()
			  WHERE id = (SELECT id FROM db_pairings
			               WHERE pair_topic = $1 AND ended_at IS NULL
			               ORDER BY id DESC LIMIT 1)`,
			op.Pair)
	case "message":
		_, _ = a.pool.Exec(ctx,
			`INSERT INTO db_messages (pair_topic, from_sid, body) VALUES ($1, $2, $3)`,
			op.Pair, op.From, op.Body)
	}
}

// --- /stats widgets (read straight from ClickHouse) --------------------------

// Widget adapts a fragment-builder into a deferred-widget PageFunc: the fragment
// carries the placeholder's id so the patch morphs cleanly, and a direct
// navigation still renders the bare fragment instead of a 404. build runs per
// request — these are live queries, not cached fragments.
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

// perDay buckets the last `days` calendar days (today included) of events whose
// kind matches into zero-filled per-day counts — WITH FILL closes the gaps so
// quiet days chart as zero instead of vanishing. kind is a trusted literal from
// the callers below, never user input.
func (a *Analytics) perDay(c *beach.Ctx, kind string, days int) ([]dayCount, error) {
	back := strconv.Itoa(days - 1) // today plus `back` days behind it = `days` buckets
	return ch.Rows[dayCount](c.Context(), a.chConn, `
		SELECT toDate(ts) AS d, count() AS n
		  FROM driftbottle_events
		 WHERE toDate(ts) >= toDate(now()) - `+back+` AND kind = '`+kind+`'
		 GROUP BY d ORDER BY d
		  WITH FILL FROM toDate(now()) - `+back+` TO toDate(now()) + 1`)
}

// dayLabel formats a per-day bucket for an axis ("Jun 2").
func dayLabel(r dayCount) string { return r.D.Format("Jan 2") }

// Pairings is pairings per day, 14 days, as a filled line.
func (a *Analytics) Pairings(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.perDay(c, "paired", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToLineSeriesData("pairings", rows, dayLabel, func(r dayCount) float64 { return float64(r.N) })
	in.ShowArea = true
	return chart.ChartLineFragment(chart.LayoutLine("anl-pairings", in, chart.LineOpts{RateUnit: "day"})), nil
}

// Messages is messages per day, 14 days, as bars.
func (a *Analytics) Messages(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.perDay(c, "message", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToHBarSeries(rows, dayLabel, func(r dayCount) int { return int(r.N) })
	in.Unit = " messages"
	return chart.ChartBarFragment("anl-messages", chart.LayoutHBar(in)), nil
}

// Sessions is sessions started per day, 14 days, as a filled line — the raw
// arrival rate that feeds the matchmaker.
func (a *Analytics) Sessions(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.perDay(c, "session", 14)
	if err != nil {
		return nil, err
	}
	in := ch.ToLineSeriesData("sessions", rows, dayLabel, func(r dayCount) float64 { return float64(r.N) })
	in.ShowArea = true
	return chart.ChartLineFragment(chart.LayoutLine("anl-sessions", in, chart.LineOpts{RateUnit: "day"})), nil
}

// StatsPage is the public analytics page handler.
func (a *Analytics) StatsPage(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: statsDocument()}, nil
}
