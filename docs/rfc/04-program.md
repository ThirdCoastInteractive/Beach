# RFC 4 — Program of projects, validation, risks

[← docs index](../README.md) · prev: [The ECS pitch](03-ecs.md)

## Program of projects

Each project is independently shippable and lands complete — code, tests, docs, and
example usage. Order is dependency order, not priority tiers.

1. **`config` + `pg` + `pgnotify`** — the boot spine. Env loader, pool wrapper,
   advisory-lock migrations, `InTx`, LISTEN with backoff, NOTIFY helper.
2. **`web` + `datastar`** — App builder (middleware stack, healthz, static cache,
   merged FS, CSP presets, graceful shutdown), the typed handler shapes
   (`PageFunc`/`ActionFunc`/`StreamFunc`, `web.Ctx`, guards, the error contract),
   `IsDatastar`/`NewSSE` underneath with flush-per-event SSE gzip, typed attribute
   builders, vet analyzers (`lint-datastar`, `lint-pgtype`, `lint-uuid`,
   naked-`echo.HandlerFunc`).
3. **`hub` + `cache` + `session`** — topic registry with per-connection buffered
   fan-out and drop-on-full, ticker producers, snapshot caches with NOTIFY
   invalidation, Valkey sessions with auth middleware.
4. **`chrome`** — component interface, registry, context injection, icon API, semantic
   token contract, and `driftwood`: a minimal reference chrome proving the interface
   (RetroChrome stays in manifold; porting it is manifold's business).
5. **`ecs`** — the standalone engine: component schema + codegen, archetype/columnar
   storage, entity handles (index+generation), tick-stamped change detection,
   entity relationships, CBOR snapshots, and a benchmark suite running the
   go-ecs-benchmarks scenarios against Ark as the yardstick. Zero framework imports.
6. **`world`** — the web integration over `ecs`: command channel, tick loop,
   dirty→topic mapping, write-behind persistence helpers, world snapshot/restore.
7. **`cmd/beach`** — scaffold CLI. `beach new <app>` stamps the skeleton: Makefile,
   sqlc.yaml, Tailwind input.css with token layer, docker-compose (Postgres + Valkey),
   .env template, a working page with one SSE fragment. `beach vet` runs the
   analyzers; `beach ecs gen` runs the component codegen.
8. **`cmd/examples/boardwalk`** — the validation app: live chat + presence + an XP
   tick system, exercising every package including `world`, with an `ssesim`-style
   fan-out benchmark. This is the falsifiable claim that the framework works.

## Validation criteria

- `beach new` to styled, DB-backed, SSE-live page: under one hour, documented as a
  walkthrough.
- boardwalk sustains the manifold chat budget (5k SSE connections, single container)
  in the included benchmark — with SSE compression enabled, and per-connection
  compressor memory reported as a first-class benchmark number.
- A grep of boardwalk shows zero raw `data-*` Datastar strings, zero pgtype imports
  outside generated code, zero hardcoded colors — all enforced by `beach vet`.
- thirdcoast.systems could delete its duplicated helpers and import the-beach with a
  net-negative diff (assessed on paper as a writeup; actual migration is its own
  project).
- `ecs` imports nothing from the rest of the-beach, and its benchmark suite stays
  within small-integer factors of Ark on the go-ecs-benchmarks scenarios that matter
  here (query iteration, batch create/destroy).

## Risks

- **Building an engine.** A custom ECS is exactly the kind of project grug warns
  about. Containment: the core is deliberately small (archetype tables + queries +
  change ticks + snapshots), the non-features list in
  [architecture/07-ecs.md](../architecture/07-ecs.md#non-features) is a contract
  (no parallel scheduler, no scripting, no reflection queries, no internal event
  bus), and the benchmark suite against Ark keeps performance claims honest. If the
  custom engine stalls, the `world` facade is narrow enough to swap a third-party
  engine back in.
- **ECS overreach.** The siren call is to ECS-ify CRUD. The boundary rule in
  [architecture/08-world.md](../architecture/08-world.md#the-boundary) is written to
  be quoted back at whoever tries.
- **Framework-itis.** Grug warning: extracting too early, abstracting what manifold
  only did once. Countermeasure: nothing enters the-beach that doesn't exist in
  manifold13.tv or fall directly out of the ECS design. New ideas earn their way in
  through an app first.
- **LISTEN/NOTIFY ceiling.** Known limits (8KB payloads, commit-serialized NOTIFY under
  high write rates, dedicated connection per listener). Acceptable single-node; the
  `hub` topic seam is where NATS slots in if multi-node ever happens.
