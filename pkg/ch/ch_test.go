package ch

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestConnectEmptyDSN(t *testing.T) {
	conn, err := Connect(context.Background(), "")
	if err != nil {
		t.Fatalf("Connect(\"\") error = %v, want nil", err)
	}
	if conn != nil {
		t.Fatalf("Connect(\"\") conn = %v, want nil", conn)
	}
}

func TestMigrateNilConnNoop(t *testing.T) {
	if err := Migrate(context.Background(), nil, fstest.MapFS{}); err != nil {
		t.Fatalf("Migrate(nil) = %v, want nil", err)
	}
}

func TestRowsNilConn(t *testing.T) {
	rows, err := Rows[event](context.Background(), nil, "SELECT 1")
	if err != nil {
		t.Fatalf("Rows(nil) error = %v, want nil", err)
	}
	if rows != nil {
		t.Fatalf("Rows(nil) = %v, want nil", rows)
	}
}

// --- time-series / rollup builders (timeseries.go) ---------------------------
//
// These assert the generated SQL string: bucket alignment, the WITH FILL
// gap-fill, and the argMax as-of are all provable offline, no live ClickHouse.

// mustContain fails the test unless every want fragment appears in got.
func mustContain(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("SQL missing %q\n  in: %s", w, got)
		}
	}
}

func TestChIntervalUnits(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{15 * time.Minute, "15 MINUTE"},
		{90 * time.Minute, "90 MINUTE"}, // not an exact hour → stays in minutes
		{time.Hour, "1 HOUR"},
		{6 * time.Hour, "6 HOUR"},
		{24 * time.Hour, "1 DAY"},
		{48 * time.Hour, "2 DAY"},
		{30 * time.Second, "30 SECOND"},
		{0, "1 HOUR"}, // zero defaults to one hour
	}
	for _, c := range cases {
		if got := chInterval(c.d); got != c.want {
			t.Errorf("chInterval(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestBucketSQLAlignment(t *testing.T) {
	got := Bucket{
		Table:    "rack_draw",
		Time:     "ts",
		Value:    "kw",
		Interval: 15 * time.Minute,
		Agg:      Avg,
	}.SQL()
	// Aligned bucket boundary via toStartOfInterval at the chosen interval.
	mustContain(t, got,
		"toStartOfInterval(ts, INTERVAL 15 MINUTE) AS bucket",
		"avg(kw) AS v",
		"FROM rack_draw",
		"WHERE ts >= ?",
		"GROUP BY bucket",
		"ORDER BY bucket")
}

func TestBucketSQLDefaultsAndCount(t *testing.T) {
	// Empty Agg → Avg; empty Interval → 1 HOUR.
	got := Bucket{Table: "t", Time: "ts", Value: "v"}.SQL()
	mustContain(t, got, "toStartOfInterval(ts, INTERVAL 1 HOUR)", "avg(v) AS v")

	// Count needs no value column and renders count().
	cnt := Bucket{Table: "t", Time: "ts", Interval: time.Hour, Agg: Count}.SQL()
	mustContain(t, cnt, "count() AS v")
	if strings.Contains(cnt, "count(1)") {
		t.Errorf("Count must render count(), got: %s", cnt)
	}
}

func TestBucketSQLByEntity(t *testing.T) {
	got := Bucket{
		Table: "draw", Time: "ts", Value: "kw",
		Interval: time.Hour, Agg: Sum, By: "rack",
	}.SQL()
	mustContain(t, got,
		"rack AS entity,",
		"GROUP BY entity, bucket",
		"ORDER BY entity, bucket",
		"sum(kw) AS v")
}

func TestBucketSQLWhere(t *testing.T) {
	got := Bucket{
		Table: "draw", Time: "ts", Value: "kw",
		Interval: time.Hour, Where: "site = 'pdx'",
	}.SQL()
	mustContain(t, got, "WHERE ts >= ? AND site = 'pdx'")
}

func TestGapFillSQL(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	got := GapFill{
		Bucket: Bucket{Table: "draw", Time: "ts", Value: "kw", Interval: time.Hour, Agg: Avg},
		From:   from, To: to,
	}.SQL()
	// One row per bucket across the range: WITH FILL at the same interval,
	// bounds aligned to the bucket grid, default 0 fill via INTERPOLATE.
	mustContain(t, got,
		"WHERE ts >= ? AND ts < ?",
		"ORDER BY bucket WITH FILL",
		"FROM toStartOfInterval(?, INTERVAL 1 HOUR)",
		"TO toStartOfInterval(?, INTERVAL 1 HOUR)",
		"STEP INTERVAL 1 HOUR",
		"INTERPOLATE (v AS 0)")
}

func TestGapFillSQLCustomFill(t *testing.T) {
	got := GapFill{
		Bucket: Bucket{Table: "t", Time: "ts", Value: "v", Interval: 5 * time.Minute},
		Fill:   "nan",
	}.SQL()
	mustContain(t, got, "STEP INTERVAL 5 MINUTE", "INTERPOLATE (v AS nan)")
}

func TestAsOfSQL(t *testing.T) {
	got := AsOf{
		Table:  "battery_reading",
		Time:   "ts",
		Entity: "battery_id",
		Values: []string{"soc", "volts"},
	}.SQL()
	// Latest reading per entity at/before the cutoff via argMax(value, time).
	mustContain(t, got,
		"battery_id AS entity",
		"max(ts) AS ts",
		"argMax(soc, ts) AS soc",
		"argMax(volts, ts) AS volts",
		"FROM battery_reading",
		"WHERE ts <= ?",
		"GROUP BY battery_id",
		"ORDER BY entity")
}

func TestAsOfSQLWhere(t *testing.T) {
	got := AsOf{
		Table: "r", Time: "ts", Entity: "id", Values: []string{"v"},
		Where: "site = 'pdx'",
	}.SQL()
	mustContain(t, got, "WHERE ts <= ? AND site = 'pdx'")
}

func TestRollupSQL(t *testing.T) {
	// Fine samples downsampled to a coarse 6-hour window, one series per entity.
	got := Rollup{
		Table: "samples", Time: "ts", Value: "bytes",
		Every: 6 * time.Hour, Agg: Max, By: "host",
	}.SQL()
	mustContain(t, got,
		"toStartOfInterval(ts, INTERVAL 6 HOUR) AS bucket",
		"max(bytes) AS v",
		"host AS entity,",
		"GROUP BY entity, bucket",
		"ORDER BY entity, bucket")
}

func TestRollupSQLDefaultEvery(t *testing.T) {
	got := Rollup{Table: "t", Time: "ts", Value: "v"}.SQL()
	mustContain(t, got, "toStartOfInterval(ts, INTERVAL 1 HOUR)")
}

// Every builder's Query* honors the nil-Conn contract: empty result, nil error.
func TestTimeseriesQueriesNilConn(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	type row struct {
		V float64 `ch:"v"`
	}

	if rows, err := QueryBucket[row](ctx, nil, Bucket{Table: "t", Time: "ts", Value: "v"}, now); err != nil || rows != nil {
		t.Fatalf("QueryBucket(nil) = (%v, %v), want (nil, nil)", rows, err)
	}
	if rows, err := QueryGapFill[row](ctx, nil, GapFill{
		Bucket: Bucket{Table: "t", Time: "ts", Value: "v"}, From: now, To: now,
	}); err != nil || rows != nil {
		t.Fatalf("QueryGapFill(nil) = (%v, %v), want (nil, nil)", rows, err)
	}
	if rows, err := QueryAsOf[row](ctx, nil, AsOf{Table: "t", Time: "ts", Entity: "id", Values: []string{"v"}}, now); err != nil || rows != nil {
		t.Fatalf("QueryAsOf(nil) = (%v, %v), want (nil, nil)", rows, err)
	}
	if rows, err := QueryRollup[row](ctx, nil, Rollup{Table: "t", Time: "ts", Value: "v"}, now); err != nil || rows != nil {
		t.Fatalf("QueryRollup(nil) = (%v, %v), want (nil, nil)", rows, err)
	}
}

// liveDSN returns the integration DSN or skips the test when it is unset, so the
// offline `go test` run stays green.
func liveDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip("TEST_CLICKHOUSE_DSN unset; skipping live ClickHouse test")
	}
	return dsn
}

func TestLiveConnPingAndQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn := MustConn(ctx, liveDSN(t))
	defer conn.Close()

	type one struct {
		N uint8 `ch:"n"`
	}
	rows, err := Rows[one](ctx, conn, "SELECT 1 AS n")
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 1 || rows[0].N != 1 {
		t.Fatalf("got %+v, want [{1}]", rows)
	}
}

func TestLiveMigrateAndIngest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn := MustConn(ctx, liveDSN(t))
	defer conn.Close()

	migrations := fstest.MapFS{
		"00001_events.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
CREATE TABLE IF NOT EXISTS ch_test_events (
    id   UInt64,
    name LowCardinality(String),
    ts   DateTime DEFAULT now()
) ENGINE = MergeTree ORDER BY id;
-- +goose Down
DROP TABLE IF EXISTS ch_test_events;
`)},
	}
	if err := Migrate(ctx, conn, migrations); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Exec(context.Background(), "DROP TABLE IF EXISTS ch_test_events")
	})

	type rec struct {
		ID   uint64 `ch:"id"`
		Name string `ch:"name"`
	}
	b := NewBatcher[rec](conn, "ch_test_events", Batch{Size: 2, FlushInterval: 200 * time.Millisecond})
	b.Add(rec{ID: 1, Name: "a"})
	b.Add(rec{ID: 2, Name: "b"})
	if err := b.Close(ctx); err != nil {
		t.Fatalf("batcher Close: %v", err)
	}

	type count struct {
		C uint64 `ch:"c"`
	}
	got, err := Rows[count](ctx, conn, "SELECT count() AS c FROM ch_test_events")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if len(got) != 1 || got[0].C != 2 {
		t.Fatalf("count = %+v, want 2", got)
	}
}
