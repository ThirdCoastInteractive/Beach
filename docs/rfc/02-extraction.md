# RFC 2 — What gets extracted from manifold13.tv

[← docs index](../README.md) · prev: [Charter](01-charter.md) · next: [The ECS pitch](03-ecs.md)

## What gets lifted (manifold13.tv → the-beach)

| manifold source                                                                 | the-beach package       | Notes                                                                                                           |
| ------------------------------------------------------------------------------- | ----------------------- | --------------------------------------------------------------------------------------------------------------- |
| `internal/web` (StaticCache, MergedFS, IsDatastar, NewSSE, CSRF, RequestLogger) | `beach` (module root)   | Lifted nearly verbatim; these are the crown jewels                                                              |
| `cmd/*/main.go` bootstrap sequence                                              | `beach` (App builder)   | Parameterized: middleware stack, CSP, kit resolver                                                              |
| Handler doctrine (factory functions, dual-purpose branch, SSE loop shape)       | `beach` handler shapes  | Conventions promoted into types: `PageFunc`/`ActionFunc`/`StreamFunc` + guards                                  |
| `pkg/datastar` typed builders                                                   | `datastar`              | Plus the `lint-datastar` guard as a vet analyzer                                                                |
| `internal/view/tmpl/chrome` (interface, registry, context, icons)               | `ui`                    | Reworked into the UI toolkit; implementations are kits, registration non-optional, performance laws built in   |
| `internal/view/css` three-layer token system                                    | `ui` + skeleton CSS     | Semantic tokens (`void/panel/ink/edge/accent...`) are the framework contract; each kit ships its own palette   |
| `internal/db/database.go` (pool, advisory-lock Migrate)                         | `pg`                    | Adds the missing `pg.InTx` helper                                                                               |
| `internal/db/sql/sqlc.yaml` (pgtype purge, domain IDs, enums)                   | skeleton template       | Stamped by `beach new`; `lint-pgtype` / `lint-uuid` ship as framework checks                                    |
| `internal/pgnotify`                                                             | `pg` (Listen/Notify)    | Lifted verbatim into the `pg` package: dedicated conn, exponential backoff, drop-on-slow-consumer               |
| `internal/notify/registry.go` (per-user subscription fan-out)                   | `hub`                   | Generalized from per-user to per-topic                                                                          |
| `cmd/live/handler/hub.go` + `cmd/admin/handler/sse_helpers.go`                  | `hub`                   | The two SSE mental models (event-driven hub, tick loop) unify under topics                                      |
| `internal/db/*_cache.go` (atomic snapshot + NOTIFY invalidation)                | `cache`                 | Generic `cache.Snapshot[T]` + `cache.Keyed[K,V]` with a NOTIFY-invalidation helper                              |
| `pkg/session` (Valkey store, OptionalAuth/RequireAuth/RequireRole)              | `session`               | Fix the Data/AuthUser duplication wart                                                                          |
| `internal/serviceconfig` (reflect `env:` loader)                                | `config`                | Lifted with the `A\|B` fallback syntax                                                                          |
| `Makefile` shape (`gen`/`vet`/`up`, pinned tools, FA subsetting)                | skeleton template       | Stamped by `beach new`                                                                                          |
| Bench rigs (`chatperf`, `ssesim` patterns)                                      | `examples` + docs       | SSE fan-out budget testing is a framework deliverable, not app folklore                                         |

## What gets lifted (Atlas → the-beach)

Atlas (`Y:\Atlas`) is the second quarry — same author, same stack, newer ideas.
Full designs: [architecture/12-auth.md](../architecture/12-auth.md) and
[architecture/13-apigen.md](../architecture/13-apigen.md).

| Atlas source                                                         | the-beach package    | Notes                                                                                                      |
| --------------------------------------------------------------------- | -------------------- | ------------------------------------------------------------------------------------------------------------ |
| `pkg/passwords` (argon2id, PHC, scanner/valuer, tests)               | `passwords`          | Lifted verbatim (m=128MB t=4 p=4); framework adds rehash-on-upgrade                                        |
| `pkg/principal` (RBAC core)                                          | `auth`               | Slimmed hard: three-table RBAC, `resource:action` strings, resolved at login; attestation/multiplexer/groups/scopes stay in Atlas |
| Login flow (`api/auth.go`: timing-safe verify, lockout, pretoken)    | `auth`               | Lockout thresholds, realm names, lifetimes become config                                                   |
| Session hardening (SHA256 tokens at rest, per-session CSRF, Rotate)  | `session`            | Discipline transfers; store stays Valkey, not Atlas's Postgres rows                                        |
| Users/credentials/RBAC/reset-token migrations                        | skeleton template    | Stamped by `beach new`; security rule: no secret columns in RETURNING                                      |
| `cmd/atlas-apigen` (sqlc plugin: `@api`/`@notify`/`@fragment`/...)   | `cmd/beach-apigen`   | Six hypermedia annotations adopted; emits beach handler shapes; the OpenAPI/JSON emitters stay in Atlas    |
| `atlas-i18n-extract` + catalog design                                | `i18n` + `beach i18n` | Promoted from linter to real runtime: flat-key catalog, locale JSON, literal-only `T(ctx, key)`           |
| `pkg/chart` + `chartview` (22 SSR SVG chart types, ~3.7 KLOC)        | `chart`              | Lifted near-verbatim — already token-themed, accessible, defer-native; framework adds live updates via hub/projections |
| Telemetry RFC/Spec (ClickHouse design — docs only, zero code)        | `ch`                 | Built fresh to Atlas's design: MergeTree conventions, batcher ingest, goose migrations, TTL retention as config |
| `atlas-lint-{datastar,head,stack,authz}` + `atlas-i18n-extract`      | `beach vet`          | head/stack adopted, datastar merged with manifold's, authz adapted to `@requires` + principal checks       |
| Per-entity query directories (`queries/<entity>/`)                   | skeleton template    | Replaces manifold's per-feature files; scales with the generator                                           |
| `atlas-seed`, `intel-dump`, Postgres session store                   | not taken            | Domain tools stay in Atlas; sessions stay Valkey                                                           |

## What gets fixed (known warts, named in the audits)

- **Two SSE mental models** — admin tick loops vs live hub. Unified: everything is a
  topic; a ticker is just a producer that publishes to a topic on an interval.
- **SSE responses ship uncompressed** — manifold skips gzip for SSE because Echo's
  middleware buffers events. The framework compresses in the SSE writer itself with
  a per-event sync flush instead
  ([SSE compression](../architecture/03-http.md#sse-compression)).
- **Kit (chrome) nullability** — `chrome.Lookup` failures fall through silently in
  manifold. The App builder refuses to start without a registered UI kit.
- **No first-response size budget** — manifold pages ship whatever they weigh. The
  UI toolkit adds the [14KB rule](../architecture/06-ui.md#the-14kb-rule): first
  response fits the initial TCP window; heavier sections lazy-load over Datastar via
  `ui.Defer` with exact reserved dimensions.
- **Session Data / AuthUser field duplication** — one type, derived view.
- **Inline per-service CSP strings** — `beach.CSP` builder with env-aware presets.
- **No transaction helper** — `pg.InTx(ctx, pool, func(q *db.Queries) error)`.
- **Manual cache refresh for some caches, NOTIFY for others** — `cache` package makes
  NOTIFY-invalidation the paved road.
- **`IsDatastar` checks three signals** — the `Datastar-Request` header is the only
  sanctioned check (already the documented rule; the framework makes it the only code).
- **House rules live in instruction docs and lint guards** — the dual-purpose branch,
  per-request `db.New(pool)`, no separate fragment routes, the hand-written SSE loop
  shape. These become typed handler shapes (`PageFunc` returning a `View`,
  `ActionFunc` returning `Patches`, `StreamFunc` returning a `Sub`) with guards at
  route registration — holding one wrong becomes a compile error, not a review
  comment. Raw echo remains the documented escape hatch for webhooks.
