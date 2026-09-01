package sim

import (
	"context"

	"github.com/ThirdCoastInteractive/Beach/pkg/ecs"
)

// This file holds the plumbing for the three persistence lanes documented in
// docs/architecture/08-sim.md. The in-memory mechanics (the command/projection
// path that the lanes hang off) are solid and tested; the actual Postgres I/O is
// behind small interfaces so the pg-backed implementations live in the app and
// the lane wiring stays testable without a database.
//
//   1. Synchronous tx  — money/items/trades. Handler runs pg.InTx FIRST, then
//      Send(ApplyCommitted{...}). The sim only ever reflects committed results.
//   2. Write-behind     — XP/presence/stats. Dirty component rows are batched
//      each projection pass and handed to a Flusher (sqlc upserts in the app).
//   3. Journal          — append-only ordered log (token ledger). Commands that
//      implement Journaled have their entry appended on the loop before Apply.

// --- Lane 1: synchronous transaction → ApplyCommitted -----------------------

// CommitFn does the synchronous Postgres write. It runs OFF the loop (in the
// handler goroutine) and must fully commit before its result reaches the sim.
// Return the typed payload the sim needs to reflect the committed change.
type CommitFn[R any] func(ctx context.Context) (R, error)

// Committer reflects an already-committed Postgres result into the store. It
// runs on the loop goroutine inside ApplyCommitted; it must not do I/O, only
// store mutation. R is the payload CommitFn returned.
type Committer[R any] func(w *World, committed R)

// Commit is the synchronous lane in one call: run the Postgres transaction off
// the loop, and on success enqueue an ApplyCommitted command that mirrors the
// committed result into the store. The store is updated only after Postgres has
// durably committed — money/items/trades never lead the database.
//
// It returns the committed payload (for the handler's own response) and any
// error. A commit error means nothing was sent to the sim.
func Commit[R any](ctx context.Context, s *Sim, do CommitFn[R], reflect Committer[R]) (R, error) {
	var zero R
	committed, err := do(ctx)
	if err != nil {
		return zero, err
	}
	if err := s.Send(ctx, applyCommitted[R]{payload: committed, reflect: reflect}); err != nil {
		// Committed in Postgres but the sim is shutting down/saturated. The
		// committed truth is in Postgres; the sim reconciles on next load. Return
		// the payload and the send error so the caller can decide.
		return committed, err
	}
	return committed, nil
}

// applyCommitted is the command form of the synchronous lane: it carries an
// already-committed payload and the reflect closure that mirrors it.
type applyCommitted[R any] struct {
	payload R
	reflect Committer[R]
}

func (c applyCommitted[R]) Apply(w *World) { c.reflect(w, c.payload) }

// --- Lane 2: write-behind ----------------------------------------------------

// Row is one dirty component row destined for its mirror table. Schema is the
// component's stable schema id (the mirror table name in the app's mapping),
// Entity the row key, and Value the component value as collected by a Collector.
type Row struct {
	Schema string
	Entity ecs.Entity
	Value  any
}

// Flusher receives a batch of dirty rows once per projection pass and upserts
// them to their component-mirror tables. Implementations live in the app (sqlc
// upserts); the sim only batches and hands off. A returned error is logged by
// the caller-supplied error sink and otherwise non-fatal — write-behind is
// allowed to lag and retry.
type Flusher interface {
	Flush(ctx context.Context, rows []Row) error
}

// collector pulls the dirty rows for one component type since a tick. The
// generic Mirror[T] builds one; writeBehind holds a slice of them so the loop
// can scan every mirrored component each pass without knowing T.
type collector func(store *ecs.Store, since Tick, out *[]Row)

// writeBehind is the installed write-behind lane: the set of mirrored component
// collectors plus the Flusher they feed. flush runs on the loop goroutine each
// projection pass; the actual Flush is dispatched off-loop so Postgres latency
// never stalls ticks.
type writeBehind struct {
	collectors []collector
	flusher    Flusher
	// onError receives flush errors (off-loop). May be nil.
	onError func(error)
	// dispatch sends a batch to the flusher. Default is async (goroutine);
	// tests override it with a synchronous dispatch for determinism.
	dispatch func(rows []Row)
}

// NewWriteBehind builds a write-behind lane around a Flusher. Register the
// component types to mirror with Mirror, then install with Sim.UseWriteBehind.
func NewWriteBehind(f Flusher) *writeBehind {
	wb := &writeBehind{flusher: f}
	wb.dispatch = func(rows []Row) {
		go func() {
			if err := wb.flusher.Flush(context.Background(), rows); err != nil && wb.onError != nil {
				wb.onError(err)
			}
		}()
	}
	return wb
}

// OnError sets a sink for asynchronous flush errors. Returns wb for chaining.
func (wb *writeBehind) OnError(fn func(error)) *writeBehind { wb.onError = fn; return wb }

// SetDispatchSync makes flush dispatch synchronously on the loop goroutine.
// Intended for tests that assert the flusher saw a batch deterministically.
func (wb *writeBehind) SetDispatchSync() *writeBehind {
	wb.dispatch = func(rows []Row) {
		if err := wb.flusher.Flush(context.Background(), rows); err != nil && wb.onError != nil {
			wb.onError(err)
		}
	}
	return wb
}

// Mirror registers component T for write-behind mirroring on wb. Each projection
// pass, every entity whose T changed since the last pass contributes a Row keyed
// by its stable schema id. schema is the component's schema id / mirror table
// name (must match what the Flusher's sqlc upsert targets).
func Mirror[T any](wb *writeBehind, schema string) {
	wb.collectors = append(wb.collectors, func(store *ecs.Store, since Tick, out *[]Row) {
		for e, v := range ecs.Changed[T](store, since) {
			*out = append(*out, Row{Schema: schema, Entity: e, Value: v})
		}
	})
}

// flush collects the dirty rows for every mirrored component since `since` and
// dispatches them to the Flusher. Called on the loop goroutine; dispatch is
// async by default so the loop never waits on Postgres.
func (wb *writeBehind) flush(store *ecs.Store, since Tick) {
	if len(wb.collectors) == 0 {
		return
	}
	var rows []Row
	for _, c := range wb.collectors {
		c(store, since, &rows)
	}
	if len(rows) == 0 {
		return
	}
	wb.dispatch(rows)
}

// --- Lane 3: append-only journal --------------------------------------------

// Journal is the append-only ordered log (e.g. the token ledger table). The sim
// appends an entry for every Journaled command, on the loop goroutine, in tick
// order, before the command's Apply runs — so the journal is the authoritative
// sequence even if Apply or a later crash loses store state.
//
// Append must be cheap and non-blocking on the loop. App implementations buffer
// and write-behind to Postgres; the in-memory MemJournal here is for tests.
type Journal interface {
	Append(tick Tick, entry JournalEntry)
}

// JournalEntry is one opaque log record. Kind names the event ("xp.award",
// "token.debit"); Data is the app's payload (anything the app can serialize).
type JournalEntry struct {
	Kind string
	Data any
}

// Journaled is implemented by commands that must be logged to the journal. The
// sim checks for it while draining and appends JournalEntry() before Apply.
type Journaled interface {
	JournalEntry() JournalEntry
}

// MemJournal is an in-memory Journal for tests: it records every appended entry
// with the tick it landed on. Not for production (unbounded, no durability).
type MemJournal struct {
	Entries []JournalRecord
}

// JournalRecord is one row in a MemJournal.
type JournalRecord struct {
	Tick  Tick
	Entry JournalEntry
}

// Append implements Journal.
func (m *MemJournal) Append(tick Tick, entry JournalEntry) {
	m.Entries = append(m.Entries, JournalRecord{Tick: tick, Entry: entry})
}
