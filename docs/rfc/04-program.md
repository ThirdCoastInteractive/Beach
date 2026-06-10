# RFC 4 — Program of projects, validation, risks

[← docs index](../README.md) · prev: [The ECS pitch](03-ecs.md)

## Program of projects

Each project is independently shippable and lands complete — code, tests, docs, and
example usage. Order is dependency order, not priority tiers.

1. **`config` + `pg`** — the boot spine. Env loader, pool wrapper, advisory-lock
   migrations, `InTx`, and `Listen`/`Notify` (LISTEN with backoff, NOTIFY helper —
   absorbed into `pg` rather than a separate package).
2. **`beach` (module root) + `datastar`** — App builder (middleware stack, healthz,
   static cache, merged FS, CSP presets, graceful shutdown), the typed handler shapes
   (`PageFunc`/`ActionFunc`/`StreamFunc`, `beach.Ctx`, guards, the error contract),
   `IsDatastar`/`NewSSE` underneath with flush-per-event SSE gzip, typed attribute
   builders, vet analyzers (`lint-datastar`, `lint-pgtype`, `lint-uuid`,
   naked-`echo.HandlerFunc`).
3. **`hub` + `cache` + `session` + `i18n`** — topic registry with per-connection
   buffered fan-out and drop-on-full, ticker producers, snapshot caches with
   NOTIFY invalidation, Valkey sessions with auth middleware, and the flat-key
   i18n runtime with its extractor.
4. **`passwords` + `auth`** — lifted from Atlas and slimmed: argon2id hashing
   (verbatim + rehash-on-upgrade), the hardened login flow (pretoken, timing-safe
   verify, lockout-as-config), session hardening grafted onto the Valkey store,
   a three-table RBAC with `resource:action` permissions resolved at login,
   `beach.Can()` guard, skeleton users/credentials/RBAC migrations, and the
   adapted authz analyzer.
5. **`ui`** — the toolkit: `Kit` interface, registry, context injection, icon API,
   semantic token contract, the performance laws (no pop-in, no blocking, the 14KB
   rule, `ui.Defer` lazy sections), and `driftwood`: a minimal reference kit proving
   the interface (RetroChrome stays in manifold; porting it is manifold's business).
6. **`chart` + `ch`** — the analytics pair: Atlas's 22-type SSR SVG chart library
   lifted near-verbatim (geometry all at once; live-update wiring and empty/error
   states land per type as the examples render them), and the ClickHouse package
   built fresh to Atlas's Telemetry RFC (client, goose migrations, generic
   `Batcher` ingest, query→chart helpers, retention as config). Optional
   everywhere: no DSN, no ClickHouse, app still boots.
7. **`ecs`** — the standalone engine: component schema + codegen, archetype/columnar
   `Store`, entity handles (index+generation), tick-stamped change detection,
   entity relationships, CBOR snapshots, and a benchmark suite running the
   go-ecs-benchmarks scenarios against Ark as the yardstick. Zero framework imports.
7. **`sim`** — the web integration over `ecs`: command channel, tick loop,
   projections (dirty components → topics + views), write-behind persistence
   helpers, store snapshot/restore.
8. **`cmd/beach` + `cmd/beach-apigen`** — scaffold CLI and the API generator.
   `beach new <app>` stamps the skeleton: Makefile, sqlc.yaml (with the apigen
   plugin registered), Tailwind input.css with token layer, docker-compose
   (Postgres + Valkey, ClickHouse commented out), .env template, auth migrations +
   a working login, a working page with one SSE fragment. `beach vet` runs the
   analyzers (manifold's + Atlas's lint family); `beach ecs gen` runs the component
   codegen; `beach-apigen` ports Atlas's annotation grammar to beach handler shapes
   ([architecture/13-apigen.md](../architecture/13-apigen.md)).
9. **The example trio** ([architecture/15-examples.md](../architecture/15-examples.md))
   — three apps, each shaped exactly like a consuming app, together the falsifiable
   claim that the framework works:
   - `examples/boardwalk` — Monopoly-style real-estate game: the `sim`/`ecs`
     showcase, all three persistence lanes (trades transact, positions
     write-behind, cash ledger journals).
   - `examples/driftbottle` — Omegle-style text stranger chat: anonymous
     principals, ephemeral pair topics, matchmaking; hosts the 5k SSE fan-out
     benchmark.
   - `examples/pantry` — at-home grocery ERP: apigen-generated CRUD, auth/RBAC in
     fatal-lint mode, ClickHouse-fed `chart` dashboards behind `ui.Defer`.

## Validation criteria

- `beach new` to styled, DB-backed, SSE-live page: under one hour, documented as a
  walkthrough.
- driftbottle sustains the manifold chat budget (5k SSE connections, single
  container) in the included benchmark — with SSE compression enabled, and
  per-connection compressor memory reported as a first-class benchmark number.
- A grep of all three example apps shows zero raw `data-*` Datastar strings, zero
  pgtype imports outside generated code, zero hardcoded colors — all enforced by
  `beach vet`.
- Every example app's page responses stay ≤14KB compressed (the
  [14KB rule](../architecture/06-ui.md#the-14kb-rule)), tracked in the perf-history
  gate; pantry's dashboards demonstrably lazy-load via `ui.Defer` with zero layout
  shift.
- thirdcoast.systems could delete its duplicated helpers and import the-beach with a
  net-negative diff (assessed on paper as a writeup; actual migration is its own
  project).
- `ecs` imports nothing from the rest of the-beach, and its benchmark suite stays
  within small-integer factors of Ark on the go-ecs-benchmarks scenarios that matter
  here (query iteration, batch create/destroy).
- pantry's login is argon2id end to end with two real roles; the authz analyzer
  passes in fatal mode (every mutation carries `@requires`, every hand-written
  mutation handler checks the principal, no secret columns in RETURNING).
- The majority of pantry's CRUD routes are apigen-generated, not hand-written —
  the hand-written-handler count is the published measure of the generator's worth.
- boardwalk's economy survives a mid-game process kill: committed trades and the
  ledger intact (synchronous lane), positions recovered from write-behind +
  snapshot.

## Risks

- **Building an engine.** A custom ECS is exactly the kind of project grug warns
  about. Containment: the core is deliberately small (archetype tables + queries +
  change ticks + snapshots), the non-features list in
  [architecture/07-ecs.md](../architecture/07-ecs.md#non-features) is a contract
  (no parallel scheduler, no scripting, no reflection queries, no internal event
  bus), and the benchmark suite against Ark keeps performance claims honest. If the
  custom engine stalls, the `sim` facade is narrow enough to swap a third-party
  engine back in.
- **ECS overreach.** The siren call is to ECS-ify CRUD. The boundary rule in
  [architecture/08-sim.md](../architecture/08-sim.md#the-boundary) is written to
  be quoted back at whoever tries.
- **Framework-itis.** Grug warning: extracting too early, abstracting what manifold
  only did once. Countermeasure: nothing enters the-beach that doesn't exist in
  manifold13.tv or fall directly out of the ECS design. New ideas earn their way in
  through an app first.
- **LISTEN/NOTIFY ceiling.** Known limits (8KB payloads, commit-serialized NOTIFY under
  high write rates, dedicated connection per listener). Acceptable single-node; the
  `hub` topic seam is where NATS slots in if multi-node ever happens.
