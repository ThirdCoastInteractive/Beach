# Architecture 7 — The custom `ecs` engine

[← docs index](../README.md) · prev: [UI toolkit](06-ui.md) · next: [Sim](08-sim.md)

No third-party engine. `ecs` is a standalone, zero-dependency archetype ECS with no
knowledge of the-beach, hubs, or Postgres — importable by a game server, a simulation
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

## The core boundary

The engine is four things and stays four things: archetype tables, queries, change
ticks, snapshots. Systems are plain functions over queries; ticks, commands, and
projections live in [`sim`](08-sim.md). Anything richer — scheduling, scripting,
reflection-based queries, an event bus — is `sim`'s job or the app's, which keeps the
core small enough to hold in your head whole.
