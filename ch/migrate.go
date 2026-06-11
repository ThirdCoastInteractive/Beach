package ch

import (
	"context"
	"fmt"
	"io/fs"
	"sync"

	"github.com/pressly/goose/v3"
)

// migrateMu serializes migrations across the process. ClickHouse has no advisory
// lock of its own; in the framework's single-process boot path this mutex is the
// whole story. Apps that boot many replicas against one cluster gate Migrate
// behind their Postgres advisory lock (the system of record) before calling it.
var migrateMu sync.Mutex

// Migrate runs every pending goose migration in fsys against conn using the
// ClickHouse dialect. A nil Conn is a no-op: ch is optional, so an app booted
// without a CLICKHOUSE_DSN migrates nothing and carries on.
//
// fsys holds .sql migrations (goose clickhouse dialect). Calls are serialized.
func Migrate(ctx context.Context, conn Conn, fsys fs.FS) error {
	if conn == nil {
		return nil
	}
	migrateMu.Lock()
	defer migrateMu.Unlock()

	db, err := openDB(conn)
	if err != nil {
		return err
	}
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectClickHouse, db, fsys)
	if err != nil {
		return fmt.Errorf("ch: goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("ch: migrate: %w", err)
	}
	return nil
}
