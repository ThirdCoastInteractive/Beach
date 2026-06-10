# Architecture 7 — The custom `ecs` engine

[← docs index](../README.md) · prev: [Chrome](06-chrome.md) · next: [World](08-world.md)

No third-party engine. `ecs` is a standalone, zero-dependency archetype ECS with no
knowledge of the-beach, hubs, or Postgres — importable by a game server, a simulation
CLI, or any other project. [`world`](08-world.md) is the web-framework integration
over it. The Go engines we surveyed (Ark, Arche, Donburi) are excellent but Go-bound;
our core model is deliberately language-neutral so the same component definitions and
data serve other contexts and, eventually, other runtimes.

## The portable core model

- **Entity**: a 64-bit handle — 32-bit index + 32-bit generation, so stale handles
  detect slot reuse. No pointers, no language-specific identity.
- **Components**: plain data records declared once in a schema file
  (`components.beach.yaml`). `beach ecs gen` generates the Go structs and typed
  storage accessors — and the same schema emits the SQL component-mirror migrations
  ([persistence](08-world.md#persistence)) and JSON/TypeScript shapes when another
  context needs them. One declaration, typed code everywhere: the sqlc/templ ethos
  applied to app state.
- **Storage**: archetype tables with columnar (SoA) component arrays — the design that
  makes Ark/Arche fast for iteration-heavy, low-churn workloads like ours. Entities
  move between archetypes on component add/remove; queries iterate matching
  archetypes' columns directly. [go-ecs-benchmarks](https://github.com/mlange-42/go-ecs-benchmarks)
  scenarios run in CI against Ark as the yardstick; staying within small-integer
  factors on query iteration and batch ops is a release gate.
- **Change detection, first-class**: every component column stores its last-write
  tick; mutation goes through generated accessors that stamp it. Queries filter
  `ecs.Changed[T](sinceTick)` — Bevy's model, native. This is the heart of the SSE
  rendering pipeline and the single strongest reason to own the engine rather than
  bolt observers onto someone else's.
- **Relationships**: entity-valued component fields (user→item, item→trade) with a
  storage-maintained reverse index for owner→items queries.
- **Snapshots**: versioned CBOR with stable schema ids — portable across restarts,
  binary-compatible across runtimes that share the schema.

## Non-features

As a contract (grug): no parallel system scheduler, no scripting, no
reflection-based queries, no internal event bus. Systems are plain functions over
queries; ticks, commands, and fan-out belong to [`world`](08-world.md).
