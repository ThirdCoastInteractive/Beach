# Architecture 7 — The custom `ecs` engine

[← docs index](../README.md) · prev: [UI toolkit](06-ui.md) · next: [Sim](08-sim.md)

No third-party engine. `ecs` is a standalone, zero-dependency archetype ECS with no
knowledge of Beach, hubs, or Postgres — importable by a game server, a simulation
CLI, or any other Go project. [`sim`](08-sim.md) is the web-framework integration
over it. We own the engine for two concrete reasons: tick-stamped change detection
is the heart of the rendering pipeline and deserves to be native rather than bolted
onto someone else's observer API, and a schema-first component model keeps the
sqlc/templ "declare once, generate typed code" ethos.

## The portable core model

The container is an `ecs.Store` — entities and component columns, nothing more.
(Not "world": the store doesn't tick, schedule, or know about time; that's
[`sim`](08-sim.md)'s job.)

- **Entity**: a 64-bit handle — 32-bit index + 32-bit generation, so stale handles
  detect slot reuse. A plain integer: portable to any language, with no pointer to dangle.
- **Components**: plain data records declared once in a schema file
  (`components.beach.yaml`). `beach ecs gen` generates the Go structs, the typed
  storage accessors, and the SQL component-mirror migrations
  ([persistence](08-sim.md#persistence)). One declaration, typed code everywhere:
  the sqlc/templ ethos applied to app state.
- **Storage**: archetype tables with columnar (SoA) component arrays — the proven
  design for iteration-heavy, low-churn workloads like ours. The live slice is small
  (connected users + their hot components), so the bar is "fast enough for that," not
  parity with engines built for million-entity scenes. Entities move between archetypes
  on component add/remove; queries iterate matching archetypes' columns directly. A
  small benchmark runs in CI as a regression tripwire — it catches a pathological O(n²),
  it is not a competitive target.
- **Change detection, first-class**: every component column stores its last-write
  tick; mutation goes through generated accessors that stamp it. Queries filter
  `ecs.Changed[T](sinceTick)` — Bevy's model, native. This is the heart of the SSE
  rendering pipeline and the single strongest reason to own the engine rather than
  bolt observers onto someone else's.
- **Relationships**: entity-valued component fields (user→item, item→trade) with a
  storage-maintained reverse index for owner→items queries.
- **Snapshots**: versioned CBOR with stable schema ids — crash recovery and
  restarts survive schema evolution.

## Off-loop reads: `View`

Once [`sim`](08-sim.md)'s loop goroutine owns a `Store`, reading it from any other
goroutine is a data race. A **`View`** is the lightweight in-memory answer: an
immutable, off-loop-safe read snapshot of the live slice, made by `Store.View()`.

```go
v := s.View()                          // call with exclusive store access
for e, xp := range ecs.ViewQuery[comp.XP](v) {
    render(e, xp)                      // safe off-loop while the loop mutates
}
```

The concurrency guarantee is the whole point, and it is concrete: `View()` deep-copies
every live component column — values and their per-row last-write ticks — into a
fresh, owned structure. It must be called with exclusive store access (on the loop
goroutine, e.g. inside a command or via [`sim.Sim.Snapshot`](08-sim.md#off-loop-reads-snapshot),
or before `Run` starts), so the copy is consistent as of one tick. Once built, a
`View` shares **no memory** with the `Store`: the loop may mutate the `Store` while
any number of goroutines read the `View`. A `View` never changes after construction —
it is frozen at its capture tick. To observe later state, take a fresh one.

A `View` is read-only, with the read mirrors of the `Store` helpers:

- `ViewQuery[T](v)` — iterate every entity that held `T` at capture, yielding a copy
  of its value (mirror of `Query`); iterate it from any goroutine, as often as you like.
- `ViewGet[T](v, e)` — `(value, true)` if `e` held `T` at capture, else `(zero, false)`
  (mirror of `Get`).
- `ViewChanged[T](v, sinceTick)` — iterate every entity whose `T` column was stamped
  strictly after `sinceTick` as of capture, the dirty set frozen into the `View`
  (mirror of `Changed`). Because the copy carries the per-row ticks, change detection
  works off-loop too; pass `0` for every entity holding `T`.
- `ViewHas[T](v, e)` — whether `e` held `T` at capture.
- `v.Tick()` — the store tick the `View` was captured at; `v.Len()` — distinct live
  entities captured.

It deliberately has no mutators and no relation reads — it is the rendered-state read
model for the catch-up path, not a second store. The heavyweight alternative is the
CBOR `Save`/`Load` path (snapshots, below), which also copies on the loop but marshals
the whole store; `View` is the cheap in-memory equivalent aimed at catch-up rendering.

## The core boundary

The engine is four things and stays four things: archetype tables, queries, change
ticks, snapshots. Systems are plain functions over queries; ticks, commands, and
projections live in [`sim`](08-sim.md). Anything richer — scheduling, scripting,
reflection-based queries, an event bus — is `sim`'s job or the app's, which keeps the
core small enough to hold in your head whole.
