// Package pg is the boot spine for everything Postgres-connection-shaped: the
// pool, migrations, transactions, and LISTEN/NOTIFY.
//
// See docs/architecture/02-boot-spine.md. House rule: NOTIFY payloads are ids,
// not data — listeners re-query.
package pg

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"runtime/debug"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Pool builds a pgxpool and pings it, returning the error instead of panicking.
// It is the canonical pool opener; use it when the caller wants to report a
// clean boot failure itself. MustPool wraps it for the panic-on-failure path.
func Pool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pg: parse dsn: %w", err)
	}
	// Recycle and probe so a wedged or idle connection cannot sit in the
	// pool forever. MaxConns still honors pool_max_conns in the DSN (or
	// pgx's CPU default); we only fill in the lifetimes when unset.
	if cfg.MaxConnLifetime == 0 {
		cfg.MaxConnLifetime = 30 * time.Minute
	}
	if cfg.MaxConnIdleTime == 0 {
		cfg.MaxConnIdleTime = 5 * time.Minute
	}
	if cfg.HealthCheckPeriod == 0 {
		cfg.HealthCheckPeriod = 30 * time.Second
	}
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pg: new pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pg: ping: %w", err)
	}
	return pool, nil
}

// MustPool builds a pgxpool, pings it, and panics on failure. Boot-time wiring
// has no graceful path: if the database is unreachable the process cannot serve.
func MustPool(ctx context.Context, dsn string) *pgxpool.Pool {
	pool, err := Pool(ctx, dsn)
	if err != nil {
		panic(err)
	}
	return pool
}

// migrationsDir is the conventional embed directory name for goose migrations.
const migrationsDir = "migrations"

// versionTable derives a stable, per-source goose version-table name from the
// source's FIRST migration filename. Distinct sources start with distinct first
// migrations (e.g. 00001_identity vs 00001_sessions), so they get distinct tables
// and can share one database without colliding on goose's single version
// sequence.
//
// Only the first filename is hashed, NOT the whole set: that is what keeps the
// name stable as a source grows. Hashing every filename meant adding 00002_*.sql
// renamed the table, so on the next boot goose could not find the record that
// 00001 was applied and re-ran it — failing with "relation already exists". It is
// also backward compatible: a version table created when the source had only its
// first migration was already named for that first filename (the old code hashed
// the single name), so existing databases are adopted in place, not re-run.
func versionTable(fsys fs.FS) string {
	names, _ := fs.Glob(fsys, migrationsDir+"/*.sql")
	sort.Strings(names)
	h := fnv.New32a()
	if len(names) > 0 {
		_, _ = h.Write([]byte(names[0]))
	}
	return fmt.Sprintf("goose_db_version_%08x", h.Sum32())
}

// Migrate drives goose through the pool, serialized by a pg_advisory_lock keyed
// off the app's module path so N containers can boot concurrently without racing
// the schema. It returns early when the schema is already current and never
// migrates down automatically.
//
// migrationsFS is an embed.FS (or any fs.FS) rooted such that the SQL files live
// under a "migrations" directory.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsFS fs.FS) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("pg: goose dialect: %w", err)
	}
	goose.SetBaseFS(migrationsFS)
	defer goose.SetBaseFS(nil)

	// Each migration source tracks its version in its own table, so independent
	// sources applied to the same database (e.g. the framework's session/auth
	// migrations plus an app's schema) don't collide on goose's single global
	// version sequence — without this, the second source is skipped as "already
	// at version N".
	goose.SetTableName(versionTable(migrationsFS))
	defer goose.SetTableName("goose_db_version")

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	key := advisoryLockKey(modulePath())

	// Take a session-level advisory lock. N containers serialize here; the
	// lock is released when this dedicated connection is returned.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("pg: migrate conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
		return fmt.Errorf("pg: advisory lock: %w", err)
	}
	defer func() {
		// Best-effort unlock; releasing the connection drops it regardless.
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key)
	}()

	// Early-return if the schema is already current: compare the DB version
	// against the highest available migration. Avoids re-running goose machinery
	// when N containers contend and all but one find nothing to do.
	current, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("pg: db version: %w", err)
	}
	target, err := highestMigration(migrationsFS)
	if err != nil {
		return fmt.Errorf("pg: scan migrations: %w", err)
	}
	if current >= target {
		return nil
	}

	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("pg: goose up: %w", err)
	}
	return nil
}

// highestMigration returns the largest numeric migration version found in the
// FS, or 0 if none. Mirrors goose's filename numbering (e.g. 00003_foo.sql).
func highestMigration(fsys fs.FS) (int64, error) {
	entries, err := fs.ReadDir(fsys, migrationsDir)
	if err != nil {
		// No migrations dir means nothing to apply.
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	var max int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		v, err := goose.NumericComponent(e.Name())
		if err != nil {
			// Non-migration file (README, etc.) — skip.
			continue
		}
		if v > max {
			max = v
		}
	}
	return max, nil
}

// modulePath returns the main module path from the build info, or a stable
// fallback when build info is unavailable (e.g. `go test` without module data).
func modulePath() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Path != "" {
		return bi.Main.Path
	}
	return "github.com/ThirdCoastInteractive/Beach"
}

// advisoryLockKey derives a stable int64 key from the module path so two
// different beach apps sharing a cluster don't serialize each other, while all
// instances of the same app do. FNV-1a over the path, reinterpreted as int64.
func advisoryLockKey(modulePath string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(modulePath))
	return int64(h.Sum64())
}

// InTx runs fn inside a transaction, committing on nil and rolling back on error
// or panic. It removes the manual Begin/WithTx/Rollback boilerplate from handlers.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pg: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg: commit: %w", err)
	}
	return nil
}

// Notify sends a NOTIFY on channel with payload. Per house rule, payloads are
// ids, not data ({table, id, op} JSON or a bare id) — listeners re-query.
func Notify(ctx context.Context, pool *pgxpool.Pool, channel, payload string) error {
	if _, err := pool.Exec(ctx, "SELECT pg_notify($1, $2)", channel, payload); err != nil {
		return fmt.Errorf("pg: notify %q: %w", channel, err)
	}
	return nil
}

const (
	backoffMin = 1 * time.Second
	backoffMax = 30 * time.Second
)

// nextBackoff doubles d, capped at backoffMax. A zero or negative d starts at
// backoffMin.
func nextBackoff(d time.Duration) time.Duration {
	if d < backoffMin {
		return backoffMin
	}
	d *= 2
	if d > backoffMax {
		return backoffMax
	}
	return d
}

// listenBuffer bounds the delivery channel. A full buffer means a slow consumer;
// per doctrine we drop the payload rather than block (subscribers reconcile with
// a catch-up cursor).
const listenBuffer = 256

// Listen subscribes to a Postgres channel on a dedicated connection (hijacked
// from the pool, never a pooled borrow) and returns a receive-only channel of
// payloads. It reconnects with exponential backoff capped at 30s and drops
// payloads rather than block a slow consumer. The returned channel is closed
// when ctx is cancelled.
func Listen(ctx context.Context, pool *pgxpool.Pool, channel string) (<-chan string, error) {
	// Validate connectivity up front so callers get an immediate error on a
	// misconfigured pool rather than a silent reconnect loop.
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pg: listen ping: %w", err)
	}

	out := make(chan string, listenBuffer)
	go listenLoop(ctx, pool, channel, out)
	return out, nil
}

// listenLoop owns the dedicated connection lifecycle: connect, LISTEN, pump
// notifications, reconnect on error with backoff. It closes out on ctx done.
func listenLoop(ctx context.Context, pool *pgxpool.Pool, channel string, out chan<- string) {
	defer close(out)

	var backoff time.Duration
	for {
		if ctx.Err() != nil {
			return
		}

		err := listenOnce(ctx, pool, channel, out)
		if ctx.Err() != nil {
			return
		}
		// listenOnce returned an error (or clean exit on cancellation handled
		// above). Back off, then retry.
		_ = err
		backoff = nextBackoff(backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		// On a successful connect, listenOnce resets backoff via the inner loop;
		// we conservatively keep growing here until the next clean run resets it.
	}
}

// listenOnce acquires a dedicated connection, issues LISTEN, and pumps
// notifications onto out until an error occurs or ctx is cancelled. The
// connection is hijacked so it leaves the pool permanently and is closed on
// return.
func listenOnce(ctx context.Context, pool *pgxpool.Pool, channel string, out chan<- string) error {
	poolConn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	// Hijack removes the connection from the pool so a long-lived LISTEN never
	// starves the pool or gets recycled underneath us.
	conn := poolConn.Hijack()
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return err
	}

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		select {
		case out <- n.Payload:
		default:
			// Slow consumer: drop rather than block. Subscribers reconcile via
			// a catch-up cursor.
		}
	}
}
