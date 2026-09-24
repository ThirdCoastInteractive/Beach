// Package cache provides two in-process cache shapes and the NOTIFY-driven
// invalidation that wires them to Postgres.
//
// See docs/architecture/05-services.md.
//
//   - Snapshot[T] — an atomic-pointer immutable snapshot with lock-free reads;
//     Refresh builds a new value and swaps it wholesale. For small, hot,
//     read-everywhere data (plans, palettes, feature tables).
//   - Keyed[K,V] — an RWMutex map with LoadAll at boot and per-id Invalidate.
//     For collections where entries change one at a time (emotes, assets).
//
// InvalidateOn wires a pg.Listen listener that parses an id payload and
// invalidates the entry — NOTIFY-driven invalidation is the default.
package cache

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot holds a single immutable value of type T behind an atomic pointer.
// Reads are lock-free; Refresh swaps the whole value wholesale. T should be a
// type that is safe to share without copying (a slice, map, or struct that
// callers treat as read-only).
type Snapshot[T any] struct {
	v atomic.Pointer[T]
}

// NewSnapshot returns a Snapshot pre-populated with initial.
func NewSnapshot[T any](initial T) *Snapshot[T] {
	s := &Snapshot[T]{}
	s.v.Store(&initial)
	return s
}

// Load returns the current snapshot value. It is lock-free and safe to call
// concurrently. The zero value of T is returned before the first Store/Refresh.
func (s *Snapshot[T]) Load() T {
	p := s.v.Load()
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// Store replaces the current value wholesale.
func (s *Snapshot[T]) Store(v T) {
	s.v.Store(&v)
}

// Refresh calls load to build a new value and, on success, swaps it in. The old
// value is left untouched for any in-flight readers. load's error is returned
// without mutating the snapshot.
func (s *Snapshot[T]) Refresh(ctx context.Context, load func(ctx context.Context) (T, error)) error {
	v, err := load(ctx)
	if err != nil {
		return err
	}
	s.v.Store(&v)
	return nil
}

// Keyed is an RWMutex-guarded map of K to V. Entries are loaded wholesale at
// boot via LoadAll and invalidated one at a time via Invalidate. A missing key
// is a cache miss; callers re-load on demand or wait for the next LoadAll.
type Keyed[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// NewKeyed returns an empty Keyed cache.
func NewKeyed[K comparable, V any]() *Keyed[K, V] {
	return &Keyed[K, V]{m: map[K]V{}}
}

// Get returns the value for k and whether it was present.
func (c *Keyed[K, V]) Get(k K) (V, bool) {
	c.mu.RLock()
	v, ok := c.m[k]
	c.mu.RUnlock()
	return v, ok
}

// Set inserts or replaces the value for k.
func (c *Keyed[K, V]) Set(k K, v V) {
	c.mu.Lock()
	if c.m == nil {
		c.m = map[K]V{}
	}
	c.m[k] = v
	c.mu.Unlock()
}

// LoadAll calls load to build the full entry set and replaces the map
// wholesale. On load error the existing map is left untouched.
func (c *Keyed[K, V]) LoadAll(ctx context.Context, load func(ctx context.Context) (map[K]V, error)) error {
	m, err := load(ctx)
	if err != nil {
		return err
	}
	if m == nil {
		m = map[K]V{}
	}
	c.mu.Lock()
	c.m = m
	c.mu.Unlock()
	return nil
}

// Invalidate drops the entry for k. The next Get is a miss until the entry is
// re-set or the next LoadAll runs.
func (c *Keyed[K, V]) Invalidate(k K) {
	c.mu.Lock()
	delete(c.m, k)
	c.mu.Unlock()
}

// Len returns the number of entries currently cached.
func (c *Keyed[K, V]) Len() int {
	c.mu.RLock()
	n := len(c.m)
	c.mu.RUnlock()
	return n
}

// Invalidator is the minimal surface InvalidateOn needs: something that drops a
// cache entry keyed by a string id. *Keyed[string,V] satisfies it directly.
type Invalidator interface {
	Invalidate(id string)
}

// InvalidateOn wires a pg.Listen listener on channel and invalidates the entry
// in c named by each NOTIFY payload. Per house rule, NOTIFY payloads are ids,
// not data — the payload string is the cache key. Empty payloads are dropped.
// The listener runs until ctx is cancelled.
//
// For non-string keys, use InvalidateOnParse, which takes a parse func.
func InvalidateOn(ctx context.Context, pool *pgxpool.Pool, channel string, c Invalidator) error {
	return InvalidateOnParse(ctx, pool, channel, c, func(p string) (string, bool) {
		return p, p != ""
	})
}

// InvalidateOnParse is InvalidateOn with an explicit payload parser, for caches
// whose keys are not the raw payload string. parse turns the payload into the id
// passed to c.Invalidate; payloads it rejects are dropped.
func InvalidateOnParse(ctx context.Context, pool *pgxpool.Pool, channel string, c Invalidator, parse func(payload string) (string, bool)) error {
	ch, err := pg.Listen(ctx, pool, channel)
	if err != nil {
		return err
	}
	go func() {
		for payload := range ch {
			id, ok := parse(payload)
			if !ok {
				continue
			}
			c.Invalidate(id)
		}
	}()
	return nil
}
