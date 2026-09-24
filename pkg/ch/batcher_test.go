package ch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type event struct {
	ID int
}

// fakeSink records every batch it receives and can be told to fail.
type fakeSink[T any] struct {
	mu      sync.Mutex
	batches [][]T
	fail    error
	calls   int
}

func (f *fakeSink[T]) write(_ context.Context, rows []T) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail != nil {
		return f.fail
	}
	cp := make([]T, len(rows))
	copy(cp, rows)
	f.batches = append(f.batches, cp)
	return nil
}

func (f *fakeSink[T]) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.batches {
		n += len(b)
	}
	return n
}

func (f *fakeSink[T]) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// waitFor polls cond until true or the deadline passes.
func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}

func TestBatchDefaults(t *testing.T) {
	got := Batch{}.withDefaults()
	if got.Size != 1000 {
		t.Errorf("Size default = %d, want 1000", got.Size)
	}
	if got.FlushInterval != 5*time.Second {
		t.Errorf("FlushInterval default = %v, want 5s", got.FlushInterval)
	}

	custom := Batch{Size: 10, FlushInterval: time.Minute}.withDefaults()
	if custom.Size != 10 || custom.FlushInterval != time.Minute {
		t.Errorf("custom values not preserved: %+v", custom)
	}
}

func TestSizeTriggeredFlush(t *testing.T) {
	fs := &fakeSink[event]{}
	b := newBatcher("ev", Batch{Size: 3, FlushInterval: time.Hour}, fs.write)
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	for i := 0; i < 3; i++ {
		b.Add(event{ID: i})
	}
	// Reaching Size signals a flush; the loop drains asynchronously.
	waitFor(t, time.Second, func() bool { return fs.total() == 3 })

	if b.Dropped() != 0 {
		t.Errorf("Dropped = %d, want 0", b.Dropped())
	}
	if fs.callCount() != 1 {
		t.Errorf("sink calls = %d, want 1", fs.callCount())
	}
}

func TestIntervalTriggeredFlush(t *testing.T) {
	fs := &fakeSink[event]{}
	b := newBatcher("ev", Batch{Size: 1000, FlushInterval: 10 * time.Millisecond}, fs.write)
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	b.Add(event{ID: 1})
	b.Add(event{ID: 2})
	// Below Size, so only the interval ticker can flush.
	waitFor(t, time.Second, func() bool { return fs.total() == 2 })
}

func TestCloseFlushesRemainder(t *testing.T) {
	fs := &fakeSink[event]{}
	b := newBatcher("ev", Batch{Size: 1000, FlushInterval: time.Hour}, fs.write)

	b.Add(event{ID: 1})
	b.Add(event{ID: 2})
	if fs.total() != 0 {
		t.Fatalf("flushed early: %d", fs.total())
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if fs.total() != 2 {
		t.Errorf("after Close total = %d, want 2", fs.total())
	}
}

func TestAddAfterCloseDrops(t *testing.T) {
	fs := &fakeSink[event]{}
	b := newBatcher("ev", Batch{Size: 1000, FlushInterval: time.Hour}, fs.write)
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b.Add(event{ID: 1})
	b.Add(event{ID: 2})
	if b.Dropped() != 2 {
		t.Errorf("Dropped = %d, want 2", b.Dropped())
	}
	if fs.total() != 0 {
		t.Errorf("sink received rows after close: %d", fs.total())
	}
}

func TestCloseIdempotent(t *testing.T) {
	fs := &fakeSink[event]{}
	b := newBatcher("ev", Batch{Size: 1000, FlushInterval: time.Hour}, fs.write)
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestOverflowDrops(t *testing.T) {
	// A sink that blocks until released, so the buffer can fill past 2*Size.
	release := make(chan struct{})
	var once sync.Once
	blocking := func(ctx context.Context, rows []event) error {
		<-release
		return nil
	}
	b := newBatcher("ev", Batch{Size: 2, FlushInterval: time.Hour}, blocking)
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		_ = b.Close(context.Background())
	})

	// Buffer cap is 2*Size = 4. Add many; the size-2 trigger fires a flush that
	// blocks, so rows pile up and everything past 4 is dropped.
	const n = 20
	for i := 0; i < n; i++ {
		b.Add(event{ID: i})
	}
	if b.Dropped() == 0 {
		t.Errorf("expected drops under a blocked sink, got 0 (buffered=%d)", b.Len())
	}
	if b.Len() > 4 {
		t.Errorf("buffer grew past cap: %d", b.Len())
	}
	once.Do(func() { close(release) })
}

func TestWriteFailureCountsDrops(t *testing.T) {
	fs := &fakeSink[event]{fail: errors.New("clickhouse down")}
	b := newBatcher("ev", Batch{Size: 2, FlushInterval: time.Hour}, fs.write)
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	b.Add(event{ID: 1})
	b.Add(event{ID: 2})
	// The size trigger flushes; the sink errors; those rows are counted dropped.
	waitFor(t, time.Second, func() bool { return b.Dropped() == 2 })
	if fs.total() != 0 {
		t.Errorf("failed sink recorded rows: %d", fs.total())
	}
}

func TestNilConnBatcherIsNoOp(t *testing.T) {
	b := NewBatcher[event](nil, "ev", Batch{Size: 2, FlushInterval: 10 * time.Millisecond})
	t.Cleanup(func() { _ = b.Close(context.Background()) })
	b.Add(event{ID: 1})
	b.Add(event{ID: 2})
	// No panic, no sink; the no-op write succeeds so nothing is counted dropped.
	waitFor(t, time.Second, func() bool { return b.Len() == 0 })
	if b.Dropped() != 0 {
		t.Errorf("Dropped = %d, want 0", b.Dropped())
	}
}
