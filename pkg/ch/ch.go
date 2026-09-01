// Package ch is the framework's ClickHouse client: native-protocol connection,
// goose migrations, a generic non-blocking ingest batcher, and a typed query
// helper.
//
// ClickHouse is the observation store — high-volume, append-only events and
// metrics you aggregate and chart. Postgres remains the system of record.
//
// Everything here is optional: an empty DSN means ch features are off and the
// app boots fine. Callers nil-check the connection MustConn returns.
package ch

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Conn is the native-protocol ClickHouse connection. It is the driver's own
// interface — Query/Select/Exec/PrepareBatch — re-exported so callers depend on
// this package, not the driver directly.
type Conn = driver.Conn

// dsnByConn remembers the DSN behind each live connection so Migrate, which is
// handed only a Conn, can open its own short-lived *sql.DB for goose. The native
// driver.Conn does not expose the options it was built from.
var dsnByConn sync.Map // Conn -> string

// MustConn opens a native ClickHouse connection and pings it, panicking on
// failure. An empty DSN returns a nil Conn: ch is optional and the caller treats
// a nil Conn as "analytics off".
func MustConn(ctx context.Context, dsn string) Conn {
	conn, err := Connect(ctx, dsn)
	if err != nil {
		panic(fmt.Sprintf("ch: connect: %v", err))
	}
	return conn
}

// Connect is MustConn without the panic, for callers that want to degrade
// gracefully. A nil Conn and nil error means the DSN was empty.
func Connect(ctx context.Context, dsn string) (Conn, error) {
	if dsn == "" {
		return nil, nil
	}
	opt, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("ch: parse dsn: %w", err)
	}
	conn, err := clickhouse.Open(opt)
	if err != nil {
		return nil, fmt.Errorf("ch: open: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ch: ping: %w", err)
	}
	dsnByConn.Store(conn, dsn)
	return conn, nil
}

// openDB returns a database/sql handle for the same server conn was opened
// against — goose speaks database/sql, the rest of the package uses native.
func openDB(conn Conn) (*sql.DB, error) {
	v, ok := dsnByConn.Load(conn)
	if !ok {
		return nil, fmt.Errorf("ch: unknown connection (not created by ch.MustConn)")
	}
	opt, err := clickhouse.ParseDSN(v.(string))
	if err != nil {
		return nil, fmt.Errorf("ch: parse dsn: %w", err)
	}
	return clickhouse.OpenDB(opt), nil
}
