package ch

import (
	"context"
	"os"
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
