package ch

// This file holds the small set of composable time-series / rollup SQL builders
// that the recurring analytics shapes were copy-pasting bespoke. They are NOT a
// query DSL: each builder emits a plain ClickHouse SQL string (a well-tested
// fragment) that you read, edit, and feed to ch.Rows exactly like hand-written
// SQL. SQL is still the query language — these just stop the four common shapes
// (bucket, gap-fill, last-value-as-of, rollup) from being re-typed and
// mis-aligned every time.
//
// Each builder pairs with a Query* convenience that runs the built SQL through
// ch.Rows, so the nil-Conn contract (empty result, nil error) is inherited for
// free and ch-optional callers stay branch-free. The builders themselves are
// pure string functions, which is what the tests assert against — no live
// ClickHouse needed to prove bucket alignment, the WITH FILL gap-fill, or the
// argMax as-of correctness.
//
//	// Bucket raw samples to 15-minute aligned boundaries, then chart them.
//	sql := ch.BucketSQL(ch.Bucket{
//	    Table: "rack_draw", Time: "ts", Value: "kw",
//	    Interval: 15 * time.Minute, Agg: ch.Avg,
//	})
//	type row struct {
//	    Bucket time.Time `ch:"bucket"`
//	    V      float64   `ch:"v"`
//	}
//	rows, _ := ch.Rows[row](ctx, conn, sql, since)
//	in := ch.ToLineSeriesData("kW", rows,
//	    func(r row) string { return r.Bucket.Format("15:04") },
//	    func(r row) float64 { return r.V })

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Agg is a ClickHouse aggregate function applied to a value column when
// bucketing or rolling up. It is a fixed enum — not free-form SQL — so a builder
// can never emit an injectable function name.
type Agg string

const (
	Avg   Agg = "avg"
	Sum   Agg = "sum"
	Min   Agg = "min"
	Max   Agg = "max"
	Count Agg = "count"
	Last  Agg = "anyLast" // last value seen within the bucket (ordered input)
)

func (a Agg) withDefault() Agg {
	if a == "" {
		return Avg
	}
	return a
}

// apply renders the aggregate call. count() takes no column; everything else
// wraps the value expression.
func (a Agg) apply(value string) string {
	if a.withDefault() == Count {
		return "count()"
	}
	return string(a.withDefault()) + "(" + value + ")"
}

// chInterval renders a Go duration as a ClickHouse INTERVAL clause body
// ("15 MINUTE", "1 HOUR", "1 DAY"), choosing the coarsest exact unit so the
// toStartOfInterval boundaries align to natural clock edges. A zero or negative
// duration defaults to one hour.
func chInterval(d time.Duration) string {
	if d <= 0 {
		d = time.Hour
	}
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%d DAY", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%d HOUR", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%d MINUTE", d/time.Minute)
	default:
		return fmt.Sprintf("%d SECOND", d/time.Second)
	}
}

// Bucket describes a time-bucketed aggregate: group a value column into aligned
// Interval-wide buckets over a time column and aggregate it. The result is one
// row per non-empty bucket, ordered by bucket ascending — feed it straight to
// ch.Rows and a line/area adapter. For a hole-free series across the whole
// range use GapFill instead.
type Bucket struct {
	Table    string        // source table (required)
	Time     string        // DateTime column to bucket on (required)
	Value    string        // value column to aggregate ("" with Agg=Count is fine)
	Interval time.Duration // bucket width; aligned via toStartOfInterval (default 1h)
	Agg      Agg           // aggregate over Value (default Avg)
	Where    string        // optional extra predicate, ANDed in (no "WHERE")
	By       string        // optional entity column to also group by (one series each)
}

// BucketSQL builds the bucketing query. The bucket boundary is
// toStartOfInterval(<time>, INTERVAL <n unit>), which ClickHouse aligns to the
// epoch so every bucket edge is a natural clock boundary regardless of the
// query's start instant. The query takes one positional argument: the inclusive
// lower bound on Time (use a far-past value for "all time"). When By is set the
// entity column is selected and grouped first so the rows arrive grouped per
// entity for a multi-line adapter.
func (b Bucket) SQL() string {
	value := b.Value
	if value == "" {
		value = "1" // only valid target is Count, whose apply() ignores it
	}
	bucketExpr := fmt.Sprintf("toStartOfInterval(%s, INTERVAL %s)", b.Time, chInterval(b.Interval))

	var sel, grp, ord strings.Builder
	if b.By != "" {
		fmt.Fprintf(&sel, "%s AS entity, ", b.By)
		grp.WriteString("entity, ")
		ord.WriteString("entity, ")
	}
	fmt.Fprintf(&sel, "%s AS bucket, %s AS v", bucketExpr, b.Agg.apply(value))
	grp.WriteString("bucket")
	ord.WriteString("bucket")

	where := fmt.Sprintf("%s >= ?", b.Time)
	if b.Where != "" {
		where += " AND " + b.Where
	}

	return fmt.Sprintf("SELECT %s FROM %s WHERE %s GROUP BY %s ORDER BY %s",
		sel.String(), b.Table, where, grp.String(), ord.String())
}

// BucketSQL is the package-level form of Bucket.SQL for callers that prefer a
// function call to a method.
func BucketSQL(b Bucket) string { return b.SQL() }

// QueryBucket builds Bucket's SQL and runs it through ch.Rows, scanning into T.
// since is the inclusive lower bound bound to the query's one positional arg. A
// nil Conn yields no rows and nil error (inherited from ch.Rows), so
// ch-optional callers stay branch-free.
func QueryBucket[T any](ctx context.Context, conn Conn, b Bucket, since time.Time) ([]T, error) {
	return Rows[T](ctx, conn, b.SQL(), since)
}

// GapFill is a Bucket that emits one row per bucket across the whole [From, To)
// range, including buckets with no underlying samples, so a trend chart has no
// holes. It is the Bucket shape plus ClickHouse's ORDER BY ... WITH FILL.
type GapFill struct {
	Bucket
	From time.Time // inclusive range start (also the Time lower bound)
	To   time.Time // exclusive range end the fill runs up to
	Fill string    // value for synthesized buckets (default "0"); e.g. "nan" to break the line
}

// SQL builds the gap-filled query. It reuses the bucket alignment and aggregate
// of the embedded Bucket, then appends WITH FILL FROM <start> TO <end> STEP
// INTERVAL <n unit> so ClickHouse materializes every missing bucket between the
// range bounds at the same interval. Synthesized buckets get Fill for their
// value (0 by default; "nan" to leave a visible gap). The query takes two
// positional args: From then To.
//
// WITH FILL needs literal aligned bounds, so From/To are pre-aligned to the
// bucket edge with toStartOfInterval in the FILL clause; the leading WHERE still
// binds From/To as args for the scan filter.
func (g GapFill) SQL() string {
	b := g.Bucket
	value := b.Value
	if value == "" {
		value = "1"
	}
	interval := chInterval(b.Interval)
	bucketExpr := fmt.Sprintf("toStartOfInterval(%s, INTERVAL %s)", b.Time, interval)
	fill := g.Fill
	if fill == "" {
		fill = "0"
	}

	var sel, grp strings.Builder
	if b.By != "" {
		fmt.Fprintf(&sel, "%s AS entity, ", b.By)
		grp.WriteString("entity, ")
	}
	fmt.Fprintf(&sel, "%s AS bucket, %s AS v", bucketExpr, b.Agg.apply(value))
	grp.WriteString("bucket")

	where := fmt.Sprintf("%s >= ? AND %s < ?", b.Time, b.Time)
	if b.Where != "" {
		where += " AND " + b.Where
	}

	// FILL bounds are aligned to the bucket grid so the synthesized edges line
	// up exactly with the real buckets toStartOfInterval produced.
	fromExpr := fmt.Sprintf("toStartOfInterval(?, INTERVAL %s)", interval)
	toExpr := fmt.Sprintf("toStartOfInterval(?, INTERVAL %s)", interval)

	return fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s GROUP BY %s ORDER BY bucket WITH FILL FROM %s TO %s STEP INTERVAL %s INTERPOLATE (v AS %s)",
		sel.String(), b.Table, where, grp.String(), fromExpr, toExpr, interval, fill)
}

// GapFillSQL is the package-level form of GapFill.SQL.
func GapFillSQL(g GapFill) string { return g.SQL() }

// QueryGapFill builds GapFill's SQL and runs it through ch.Rows. The four
// positional args are From, To (the scan filter) then From, To again (the FILL
// bounds), mirroring SQL()'s placeholder order. A nil Conn yields no rows and
// nil error.
func QueryGapFill[T any](ctx context.Context, conn Conn, g GapFill) ([]T, error) {
	return Rows[T](ctx, conn, g.SQL(), g.From, g.To, g.From, g.To)
}

// AsOf describes a last-value-as-of query: for each entity, the latest reading
// of one or more value columns at or before a cutoff instant. It is the
// "current state from an append-only log" shape — argMax picks the row with the
// greatest Time per entity in a single pass.
type AsOf struct {
	Table  string   // source table (required)
	Time   string   // DateTime column ranked on (required)
	Entity string   // entity column, one row out per distinct value (required)
	Values []string // value columns to carry from the latest row (required)
	Where  string   // optional extra predicate, ANDed in (no "WHERE")
}

// SQL builds the as-of query with argMax(<value>, <time>): for each Entity it
// returns the Value column(s) from the row with the maximum Time at or before
// the cutoff. The cutoff is the one positional argument. Output columns are the
// entity (AS entity), the cutoff-bounded latest time (AS ts), and each value
// aliased to its own name. Ordered by entity for stable output.
//
// argMax is chosen over "LIMIT 1 BY entity ORDER BY time DESC" because it stays
// a single grouped aggregate (no per-entity sort, no subquery) and reads
// naturally alongside the other rollup builders here.
func (a AsOf) SQL() string {
	var sel strings.Builder
	fmt.Fprintf(&sel, "%s AS entity, max(%s) AS ts", a.Entity, a.Time)
	for _, v := range a.Values {
		fmt.Fprintf(&sel, ", argMax(%s, %s) AS %s", v, a.Time, v)
	}

	where := fmt.Sprintf("%s <= ?", a.Time)
	if a.Where != "" {
		where += " AND " + a.Where
	}

	return fmt.Sprintf("SELECT %s FROM %s WHERE %s GROUP BY %s ORDER BY entity",
		sel.String(), a.Table, where, a.Entity)
}

// AsOfSQL is the package-level form of AsOf.SQL.
func AsOfSQL(a AsOf) string { return a.SQL() }

// QueryAsOf builds AsOf's SQL and runs it through ch.Rows. asOf is the cutoff
// bound to the query's one positional arg. A nil Conn yields no rows and nil
// error.
func QueryAsOf[T any](ctx context.Context, conn Conn, a AsOf, asOf time.Time) ([]T, error) {
	return Rows[T](ctx, conn, a.SQL(), asOf)
}

// Rollup downsamples already-bucketed (or raw) fine samples to a coarser window
// for trend and exhaustion-projection queries: it re-buckets the source Time
// column to the coarse Every interval and aggregates Value. It is Bucket
// specialized for the fine→coarse intent, with an Every name that reads as a
// downsample and an optional per-entity grouping for projections per resource.
type Rollup struct {
	Table string        // source table (required)
	Time  string        // DateTime column to re-bucket (required)
	Value string        // value column to aggregate (required unless Agg=Count)
	Every time.Duration // coarse window the fine samples roll up to (default 1h)
	Agg   Agg           // aggregate over Value (default Avg)
	By    string        // optional entity column; one rolled-up series per entity
	Where string        // optional extra predicate, ANDed in (no "WHERE")
}

// SQL builds the downsample query by delegating to Bucket with Interval=Every:
// the coarse window is just a wider toStartOfInterval bucket. The query takes
// one positional argument, the inclusive lower bound on Time.
func (r Rollup) SQL() string {
	return Bucket{
		Table:    r.Table,
		Time:     r.Time,
		Value:    r.Value,
		Interval: r.Every,
		Agg:      r.Agg,
		Where:    r.Where,
		By:       r.By,
	}.SQL()
}

// RollupSQL is the package-level form of Rollup.SQL.
func RollupSQL(r Rollup) string { return r.SQL() }

// QueryRollup builds Rollup's SQL and runs it through ch.Rows. since is the
// inclusive lower bound. A nil Conn yields no rows and nil error.
func QueryRollup[T any](ctx context.Context, conn Conn, r Rollup, since time.Time) ([]T, error) {
	return Rows[T](ctx, conn, r.SQL(), since)
}
