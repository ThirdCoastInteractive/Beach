package ch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Batch configures a Batcher's flush triggers.
type Batch struct {
	Size          int           // flush once this many rows are buffered (default 1000)
	FlushInterval time.Duration // flush at least this often (default 5s)
}

func (b Batch) withDefaults() Batch {
	if b.Size <= 0 {
		b.Size = 1000
	}
	if b.FlushInterval <= 0 {
		b.FlushInterval = 5 * time.Second
	}
	return b
}

// sink writes a slice of rows to the destination. The real sink talks to
// ClickHouse via a prepared batch; tests substitute a fake.
type sink[T any] func(ctx context.Context, rows []T) error

// Batcher buffers rows and flushes them to ClickHouse in the background. Add is
// non-blocking and never fails: when the buffer is full rows are dropped and
// counted, never blocking the request path. Ingestion is fire-and-forget —
// ClickHouse being down degrades analytics, never the app.
type Batcher[T any] struct {
	table string
	cfg   Batch
	write sink[T]

	mu     sync.Mutex
	buf    []T
	closed bool

	dropped atomic.Uint64

	flush   chan struct{} // signals the loop a size-triggered flush is due
	done    chan struct{} // closed to stop the loop
	stopped chan struct{} // closed once the loop has fully drained and exited
}

// NewBatcher creates a Batcher that inserts T into table over conn and starts
// its background flush loop. A nil Conn yields a Batcher whose sink is a no-op
// (every Add is dropped and counted) so callers need not nil-check — ch is
// optional and a disabled batcher still satisfies the interface.
func NewBatcher[T any](conn Conn, table string, cfg Batch) *Batcher[T] {
	var write sink[T]
	if conn == nil {
		write = func(context.Context, []T) error { return nil }
	} else {
		write = func(ctx context.Context, rows []T) error {
			batch, err := conn.PrepareBatch(ctx, "INSERT INTO "+table)
			if err != nil {
				return err
			}
			for i := range rows {
				if err := batch.AppendStruct(&rows[i]); err != nil {
					_ = batch.Abort()
					return err
				}
			}
			return batch.Send()
		}
	}
	return newBatcher(table, cfg, write)
}

// newBatcher is the constructor the tests drive with a fake sink.
func newBatcher[T any](table string, cfg Batch, write sink[T]) *Batcher[T] {
	b := &Batcher[T]{
		table:   table,
		cfg:     cfg.withDefaults(),
		write:   write,
		flush:   make(chan struct{}, 1),
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	b.buf = make([]T, 0, b.cfg.Size)
	go b.loop()
	return b
}

// Add buffers a row for ingest. It is non-blocking: if the batcher is closed or
// the buffer is at capacity (size-triggered flush hasn't drained yet) the row is
// dropped and the dropped counter is incremented. Add never returns an error and
// never blocks — the request path is sacred.
func (b *Batcher[T]) Add(v T) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		b.dropped.Add(1)
		return
	}
	// Cap the buffer at twice the flush size so a stalled flush can't grow it
	// without bound; beyond that we shed load by dropping.
	if len(b.buf) >= 2*b.cfg.Size {
		b.mu.Unlock()
		b.dropped.Add(1)
		return
	}
	b.buf = append(b.buf, v)
	full := len(b.buf) >= b.cfg.Size
	b.mu.Unlock()

	if full {
		b.signalFlush()
	}
}

// Dropped returns the total number of rows dropped since the batcher was created.
func (b *Batcher[T]) Dropped() uint64 { return b.dropped.Load() }

// Len returns the number of rows currently buffered (waiting to flush).
func (b *Batcher[T]) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}

// Close stops the flush loop after a final synchronous flush of buffered rows.
// After Close, Add drops every row. Close is idempotent.
func (b *Batcher[T]) Close(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		<-b.stopped
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	close(b.done)
	<-b.stopped
	return b.drain(ctx)
}

func (b *Batcher[T]) signalFlush() {
	select {
	case b.flush <- struct{}{}:
	default: // a flush is already pending; coalesce
	}
}

func (b *Batcher[T]) loop() {
	defer close(b.stopped)
	t := time.NewTicker(b.cfg.FlushInterval)
	defer t.Stop()
	for {
		select {
		case <-b.done:
			return
		case <-t.C:
			_ = b.drain(context.Background())
		case <-b.flush:
			_ = b.drain(context.Background())
		}
	}
}

// drain swaps out the current buffer and writes it. A write failure drops the
// batch (counted) — analytics degrade, the app does not.
func (b *Batcher[T]) drain(ctx context.Context) error {
	b.mu.Lock()
	if len(b.buf) == 0 {
		b.mu.Unlock()
		return nil
	}
	rows := b.buf
	b.buf = make([]T, 0, b.cfg.Size)
	b.mu.Unlock()

	if err := b.write(ctx, rows); err != nil {
		b.dropped.Add(uint64(len(rows)))
		return fmt.Errorf("ch: flush %q: %w", b.table, err)
	}
	return nil
}
