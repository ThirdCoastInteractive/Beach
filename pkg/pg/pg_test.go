package pg

import (
	"context"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestAdvisoryLockKey(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		same bool
	}{
		{"identical paths -> same key", "github.com/acme/app", "github.com/acme/app", true},
		{"different paths -> different keys", "github.com/acme/app", "github.com/acme/other", false},
		{"empty vs nonempty -> different keys", "", "github.com/acme/app", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ka := advisoryLockKey(tt.a)
			kb := advisoryLockKey(tt.b)
			if tt.same && ka != kb {
				t.Fatalf("expected equal keys for %q/%q, got %d and %d", tt.a, tt.b, ka, kb)
			}
			if !tt.same && ka == kb {
				t.Fatalf("expected distinct keys for %q/%q, both %d", tt.a, tt.b, ka)
			}
		})
	}
}

func TestAdvisoryLockKeyDeterministic(t *testing.T) {
	const path = "github.com/ThirdCoastInteractive/Beach"
	first := advisoryLockKey(path)
	for i := 0; i < 100; i++ {
		if got := advisoryLockKey(path); got != first {
			t.Fatalf("non-deterministic key: %d != %d", got, first)
		}
	}
}

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"zero starts at min", 0, backoffMin},
		{"negative starts at min", -1, backoffMin},
		{"below min starts at min", 500 * time.Millisecond, backoffMin},
		{"min doubles", backoffMin, 2 * time.Second},
		{"2s doubles to 4s", 2 * time.Second, 4 * time.Second},
		{"16s doubles but caps at 30s", 16 * time.Second, backoffMax},
		{"at cap stays at cap", backoffMax, backoffMax},
		{"above cap clamps to cap", 45 * time.Second, backoffMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextBackoff(tt.in); got != tt.want {
				t.Fatalf("nextBackoff(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestNextBackoffProgression walks the full reconnect curve to confirm the
// 1s->30s capped doubling described in the doc.
func TestNextBackoffProgression(t *testing.T) {
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30}
	var d time.Duration
	for i, w := range want {
		d = nextBackoff(d)
		if d != w*time.Second {
			t.Fatalf("step %d: got %v, want %v", i, d, w*time.Second)
		}
	}
}

func TestHighestMigration(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/00001_init.sql":    {Data: []byte("-- +goose Up")},
		"migrations/00002_users.sql":   {Data: []byte("-- +goose Up")},
		"migrations/00010_indexes.sql": {Data: []byte("-- +goose Up")},
		"migrations/README.md":         {Data: []byte("notes")},
	}
	got, err := highestMigration(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if got != 10 {
		t.Fatalf("highestMigration = %d, want 10", got)
	}
}

func TestHighestMigrationEmpty(t *testing.T) {
	// No migrations directory at all -> 0, no error.
	got, err := highestMigration(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("highestMigration = %d, want 0", got)
	}
}

// TestVersionTableStable confirms a source's version-table name does NOT change
// when a migration is added — the footgun that lost the record of 00001 and
// re-ran it, failing on "relation already exists" — and that distinct sources
// still get distinct tables. The name derives from the first filename, so a
// database created when the source had only that first migration is adopted in
// place rather than re-run.
func TestVersionTableStable(t *testing.T) {
	one := fstest.MapFS{
		"migrations/00001_identity.sql": {Data: []byte("-- +goose Up")},
	}
	two := fstest.MapFS{
		"migrations/00001_identity.sql":   {Data: []byte("-- +goose Up")},
		"migrations/00002_api_tokens.sql": {Data: []byte("-- +goose Up")},
	}
	if a, b := versionTable(one), versionTable(two); a != b {
		t.Fatalf("version table changed when a migration was added: %q -> %q", a, b)
	}

	// Distinct sources (distinct first migrations) keep distinct tables, so they
	// share a database without colliding on goose's single version sequence.
	other := fstest.MapFS{
		"migrations/00001_sessions.sql": {Data: []byte("-- +goose Up")},
	}
	if versionTable(one) == versionTable(other) {
		t.Fatalf("distinct sources collided on version table %q", versionTable(one))
	}

	// An empty source resolves to a deterministic name, never a panic.
	if got := versionTable(fstest.MapFS{}); got == "" {
		t.Fatal("versionTable returned empty for an empty source")
	}
}

func TestModulePathFallback(t *testing.T) {
	// Whatever the build context, modulePath must be non-empty so the advisory
	// lock key is always derivable.
	if modulePath() == "" {
		t.Fatal("modulePath returned empty string")
	}
}

// badDSN is unparseable, so Pool fails synchronously in pgxpool.New (before any
// network I/O) — letting these tests exercise the error/panic paths without a
// live Postgres.
const badDSN = "this is not a valid dsn ::: @@@"

func TestPoolBadDSNReturnsError(t *testing.T) {
	pool, err := Pool(context.Background(), badDSN)
	if err == nil {
		pool.Close()
		t.Fatal("expected error from Pool on a bad DSN, got nil")
	}
	if pool != nil {
		t.Fatalf("expected nil pool on error, got %v", pool)
	}
}

func TestMustPoolBadDSNPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected MustPool to panic on a bad DSN")
		}
	}()
	MustPool(context.Background(), badDSN)
}

// --- Integration tests (gated on TEST_POSTGRES_DSN) ---

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping live Postgres test")
	}
	return dsn
}

func TestMustPoolAndInTx(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	pool := MustPool(ctx, dsn)
	defer pool.Close()

	err := InTx(ctx, pool, func(tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, "SELECT 1").Scan(&n)
	})
	if err != nil {
		t.Fatalf("InTx: %v", err)
	}
}

func TestInTxRollback(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	pool := MustPool(ctx, dsn)
	defer pool.Close()

	sentinel := context.DeadlineExceeded
	err := InTx(ctx, pool, func(tx pgx.Tx) error {
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("expected sentinel error propagated, got %v", err)
	}
}

func TestListenNotify(t *testing.T) {
	dsn := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := MustPool(ctx, dsn)
	defer pool.Close()

	ch, err := Listen(ctx, pool, "beach_test")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	// Give the listener a moment to issue LISTEN before notifying.
	time.Sleep(200 * time.Millisecond)
	if err := Notify(ctx, pool, "beach_test", "42"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case got := <-ch:
		if got != "42" {
			t.Fatalf("payload = %q, want %q", got, "42")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for notification")
	}
}
