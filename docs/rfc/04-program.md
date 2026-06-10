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
   NOTIFY invalidation, Postgres sessions with auth middleware, and the flat-key
   i18n runtime with its extractor.
4. **`passwords` + `auth`** — argon2id hashing with rehash-on-upgrade, the hardened
   login flow (pretoken, timing-safe verify, lockout-as-config), session hardening
   on the Postgres `sessions` table, a three-table RBAC with `resource:action`
   permissions resolved at login, `beach.Can()` guard, skeleton
   users/credentials/sessions/RBAC migrations, and the authz analyzer.
5. **`ui`** — the toolkit: `Kit` interface and its sub-interfaces, registry, context
   injection, icon API, semantic token contract, the performance laws (no pop-in, no
   blocking, the 14KB rule, `ui.Defer` lazy sections), the component catalog — app UI,
   marketing, and commerce, Core (trio-proven) shipping first
   ([architecture/16-components.md](../architecture/16-components.md)) — and `driftwood`:
   a minimal reference kit implementing the Core catalog, with a specimen page that
   renders it.
6. **`chart` + `ch`** — the analytics pair: the 22-type SSR SVG chart library
   (geometry all at once; live-update wiring and empty/error states land per type
   as the examples render them), and the ClickHouse package (client, goose
   migrations, generic `Batcher` ingest, query→chart helpers, retention as
   config). Optional everywhere: no DSN, no ClickHouse, app still boots.
7. **`ecs`** — the standalone engine: component schema + codegen, archetype/columnar
   `Store`, entity handles (index+generation), tick-stamped change detection,
   entity relationships, CBOR snapshots, and a small benchmark guarding against
   iteration regressions. Zero framework imports.
8. **`sim`** — the web integration over `ecs`: command channel, tick loop,
   projections (dirty components → topics + views), write-behind persistence
   helpers, store snapshot/restore.
9. **`cmd/beach` + `cmd/beach-apigen`** — scaffold CLI and the API generator.
   `beach new <app>` stamps the skeleton: Makefile, sqlc.yaml (with the apigen
   plugin registered), Tailwind input.css with token layer, docker-compose
   (Postgres, ClickHouse commented out), .env template, auth migrations +
   a working login, a working page with one SSE fragment. `beach vet` runs the
   analyzers; `beach ecs gen` runs the component codegen; `beach-apigen` turns the
   six SQL annotations into beach handler shapes
   ([architecture/13-apigen.md](../architecture/13-apigen.md)).
10. **The example trio** ([architecture/15-examples.md](../architecture/15-examples.md))
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
- driftbottle sustains 5k SSE connections on a single container in the included
  benchmark — with SSE compression enabled, and per-connection compressor memory
  reported as a first-class benchmark number.
- A grep of all three example apps shows zero raw `data-*` Datastar strings, zero
  pgtype imports outside generated code, zero hardcoded colors — all enforced by
  `beach vet`.
- `driftwood` implements the full Core component catalog
  ([architecture/16-components.md](../architecture/16-components.md)), and its specimen
  page renders every one — the at-a-glance proof the kit is complete.
- Every example app's page responses stay ≤14KB compressed (the
  [14KB rule](../architecture/06-ui.md#the-14kb-rule)), tracked in the perf-history
  gate; pantry's dashboards demonstrably lazy-load via `ui.Defer` with zero layout
  shift.
- An existing production service on this stack could adopt the framework with a
  net-negative diff (assessed on paper as a writeup; actual migration is its own
  project).
- `ecs` imports nothing from the rest of the-beach — it compiles and benchmarks with
  zero framework imports, which is the modularity claim that matters. Its benchmark
  exists to catch iteration regressions, not to chase a competitor's numbers.
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
  about. Containment: the core stays four things — archetype tables, queries, change
  ticks, snapshots ([the core boundary](../architecture/07-ecs.md#the-core-boundary)
  keeps scheduling, scripting, reflection queries, and events in `sim` or out
  entirely), and the engine makes no competitive performance claim to defend — it only has
  to keep up with the live slice. If the custom engine stalls, the `sim` facade is
  narrow enough to swap a third-party engine back in.
- **ECS overreach.** The siren call is to ECS-ify CRUD. The boundary rule in
  [architecture/08-sim.md](../architecture/08-sim.md#the-boundary) is written to
  be quoted back at whoever tries.
- **Framework-itis.** Grug warning: abstracting what has only happened once.
  Countermeasure: nothing enters the-beach that isn't proven in production or
  demanded by an example app — new ideas earn their way in through an app first,
  and the coverage matrix in
  [architecture/15-examples.md](../architecture/15-examples.md) keeps that honest.
- **LISTEN/NOTIFY ceiling.** Known limits (8KB payloads, commit-serialized NOTIFY under
  high write rates, dedicated connection per listener). Acceptable single-node; the
  `hub` topic seam is where NATS slots in if multi-node ever happens.
