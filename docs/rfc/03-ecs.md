# RFC 3 — What's new: the ECS

[← docs index](../README.md) · prev: [Scope](02-scope.md) · next: [WebSockets](05-websockets.md)

This is the RFC-level pitch. The full designs live in
[architecture/07-ecs.md](../architecture/07-ecs.md) (the engine) and
[architecture/08-sim.md](../architecture/08-sim.md) (the web integration);
the overall spine diagram is in the [docs index](../README.md#the-realtime-spine).

ECS is a module the framework offers for realtime features — not the framework's
spine. Routing, auth, forms, and CRUD pages stay handler → sqlc → templ. The research
landed on:

- **A custom, standalone engine (`ecs`).** No third-party ECS. `ecs` is a
  zero-dependency package with no knowledge of Beach, hubs, or Postgres —
  reusable by any Go project (game server, simulation, CLI tooling); `sim` is just
  the web integration on top. Storage follows the proven archetype/columnar design —
  fast enough for the live slice (connected users + their hot components), which is the
  only thing the sim ever holds. The engine isn't competing with Ark or Arche on raw
  throughput; it earns its keep by bringing ECS into web apps.
- **Schema-first components.** Components are declared once in a schema file;
  `beach ecs gen` generates the Go structs, the storage accessors, and the SQL
  component-mirror migrations. Write once, generate typed code everywhere: the
  exact ethos that makes sqlc and templ the house tools.
- **Change detection is first-class.** Tick-stamped component columns and
  `Changed[T]` queries are built into the engine, not bolted on via observers —
  it is the heart of the rendering pipeline and the single strongest argument for
  owning the engine.
- **Single-writer sim.** One goroutine owns the `ecs.Store`; handlers send typed
  commands over a channel. No locks, deterministic ticks, trivially testable.
- **Change detection drives projections.** Per-tick dirty set → projections (the
  app's mapping from changed components to hub topics + templ views) → render each
  dirty projection once per topic → fan out via `hub` to every subscribed SSE
  connection. This is Bevy's `Changed<T>` reborn as "only re-render subscribed
  fragments", and it composes with the no-pop-in rule: the server always renders
  final state.
- **Postgres stays the system of record.** Component-mirror tables via sqlc.
  Money/items/trades: synchronous transactions (the sim is a cache of the committed
  result). XP/presence/cooldowns: write-behind batches each tick. Token ledger:
  append-only journal. Snapshots only as crash recovery for ephemera.
- **LISTEN/NOTIFY is the wake-up bell, not the data bus.** Payloads are
  `{table, id, op}`; listeners re-query. In-process mutations feed the topic router
  directly and never round-trip through Postgres.
- **The hybrid boundary is explicit.** The sim hosts the live slice (connected users
  + active show + their hot components), loaded on connect, evicted on disconnect.
  CRUD, auth, payments, and ad-hoc relational queries never enter the sim. The
  database already is an ECS for cold data; we don't build a worse Postgres in RAM.
