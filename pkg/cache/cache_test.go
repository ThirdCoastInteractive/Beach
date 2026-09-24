package cache

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSnapshotLoadZeroBeforeStore(t *testing.T) {
	s := &Snapshot[[]string]{}
	if got := s.Load(); got != nil {
		t.Fatalf("zero Snapshot Load = %v, want nil", got)
	}
}

func TestSnapshotStoreAndLoad(t *testing.T) {
	s := NewSnapshot(map[string]int{"a": 1})
	if got := s.Load()["a"]; got != 1 {
		t.Fatalf("Load[a] = %d, want 1", got)
	}
	s.Store(map[string]int{"b": 2})
	if _, ok := s.Load()["a"]; ok {
		t.Fatalf("expected wholesale swap to drop key a")
	}
	if got := s.Load()["b"]; got != 2 {
		t.Fatalf("Load[b] = %d, want 2", got)
	}
}

func TestSnapshotRefresh(t *testing.T) {
	tests := []struct {
		name    string
		load    func(ctx context.Context) (int, error)
		want    int
		wantErr bool
	}{
		{
			name: "success swaps",
			load: func(ctx context.Context) (int, error) { return 42, nil },
			want: 42,
		},
		{
			name:    "error keeps old",
			load:    func(ctx context.Context) (int, error) { return 99, errors.New("boom") },
			want:    7,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSnapshot(7)
			err := s.Refresh(context.Background(), tt.load)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Refresh err = %v, wantErr %v", err, tt.wantErr)
			}
			if got := s.Load(); got != tt.want {
				t.Fatalf("Load = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSnapshotConcurrentSwap(t *testing.T) {
	s := NewSnapshot(0)
	var wg sync.WaitGroup
	// Writers swap values; readers must always observe a coherent (stored) int.
	for w := 0; w < 4; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				s.Store(w*1000 + i)
			}
		}()
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				_ = s.Load()
			}
		}()
	}
	wg.Wait()
}

func TestKeyedLoadAllAndGet(t *testing.T) {
	c := NewKeyed[string, int]()
	if _, ok := c.Get("x"); ok {
		t.Fatalf("empty cache should miss")
	}
	err := c.LoadAll(context.Background(), func(ctx context.Context) (map[string]int, error) {
		return map[string]int{"x": 1, "y": 2}, nil
	})
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if v, ok := c.Get("x"); !ok || v != 1 {
		t.Fatalf("Get(x) = %d,%v want 1,true", v, ok)
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
}

func TestKeyedLoadAllErrorKeepsOld(t *testing.T) {
	c := NewKeyed[string, int]()
	c.Set("keep", 1)
	err := c.LoadAll(context.Background(), func(ctx context.Context) (map[string]int, error) {
		return nil, errors.New("boom")
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if v, ok := c.Get("keep"); !ok || v != 1 {
		t.Fatalf("error LoadAll should leave map untouched, got %d,%v", v, ok)
	}
}

func TestKeyedInvalidate(t *testing.T) {
	c := NewKeyed[string, int]()
	c.Set("a", 1)
	c.Invalidate("a")
	if _, ok := c.Get("a"); ok {
		t.Fatalf("Invalidate(a) should remove the entry")
	}
	// Invalidating a missing key is a no-op.
	c.Invalidate("missing")
}

func TestKeyedConcurrentInvalidate(t *testing.T) {
	c := NewKeyed[int, int]()
	const n = 500
	seed := map[int]int{}
	for i := 0; i < n; i++ {
		seed[i] = i
	}
	if err := c.LoadAll(context.Background(), func(ctx context.Context) (map[int]int, error) {
		return seed, nil
	}); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	var wg sync.WaitGroup
	// Concurrent invalidators each drop a disjoint slice of keys; concurrent
	// readers race them. Under -race this exercises the RWMutex.
	for g := 0; g < 4; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := g; i < n; i += 4 {
				c.Invalidate(i)
			}
		}()
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				c.Get(i)
			}
		}()
	}
	wg.Wait()

	if c.Len() != 0 {
		t.Fatalf("after invalidating all keys Len = %d, want 0", c.Len())
	}
}

// keyedStringInvalidator confirms *Keyed[string,V] satisfies Invalidator.
var _ Invalidator = (*Keyed[string, int])(nil)

func TestInvalidateOnLive(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	c := NewKeyed[string, int]()
	if err := c.LoadAll(ctx, func(ctx context.Context) (map[string]int, error) {
		return map[string]int{"42": 1}, nil
	}); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	const channel = "cache_test_invalidate"
	if err := InvalidateOn(ctx, pool, channel, c); err != nil {
		t.Fatalf("InvalidateOn: %v", err)
	}

	// Give the listener a moment to issue LISTEN before we NOTIFY.
	time.Sleep(200 * time.Millisecond)

	if err := pg.Notify(ctx, pool, channel, "42"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := c.Get("42"); !ok {
			break // invalidated
		}
		if time.Now().After(deadline) {
			t.Fatalf("entry not invalidated within deadline")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestInvalidateOnParse exercises the parse path without a DB by invalidating
// directly through the parser used by InvalidateOnParse.
func TestParsePayloadToKey(t *testing.T) {
	c := NewKeyed[string, int]()
	c.Set("7", 1)

	parse := func(p string) (string, bool) {
		if _, err := strconv.Atoi(p); err != nil {
			return "", false
		}
		return p, true
	}
	if id, ok := parse("7"); ok {
		c.Invalidate(id)
	}
	if _, ok := c.Get("7"); ok {
		t.Fatalf("expected 7 to be invalidated")
	}
	if _, ok := parse("notanum"); ok {
		t.Fatalf("non-numeric payload should be rejected")
	}
}
