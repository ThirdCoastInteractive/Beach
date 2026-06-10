# RFC 2 — What gets extracted from manifold13.tv

[← docs index](../README.md) · prev: [Charter](01-charter.md) · next: [The ECS pitch](03-ecs.md)

## What gets lifted (manifold13.tv → the-beach)

| manifold source                                                                 | the-beach package       | Notes                                                                                                           |
| ------------------------------------------------------------------------------- | ----------------------- | --------------------------------------------------------------------------------------------------------------- |
| `internal/web` (StaticCache, MergedFS, IsDatastar, NewSSE, CSRF, RequestLogger) | `web`                   | Lifted nearly verbatim; these are the crown jewels                                                              |
| `cmd/*/main.go` bootstrap sequence                                              | `web` (App builder)     | Parameterized: middleware stack, CSP, chrome resolver                                                           |
| Handler doctrine (factory functions, dual-purpose branch, SSE loop shape)       | `web` handler shapes    | Conventions promoted into types: `PageFunc`/`ActionFunc`/`StreamFunc` + guards                                  |
| `pkg/datastar` typed builders                                                   | `datastar`              | Plus the `lint-datastar` guard as a vet analyzer                                                                |
| `internal/view/tmpl/chrome` (interface, registry, context, icons)               | `chrome`                | Chrome becomes non-optional: apps must register one at boot                                                     |
| `internal/view/css` three-layer token system                                    | `chrome` + skeleton CSS | Semantic tokens (`void/panel/ink/edge/accent...`) are the framework contract; each chrome ships its own palette |
| `internal/db/database.go` (pool, advisory-lock Migrate)                         | `pg`                    | Adds the missing `pg.InTx` helper                                                                               |
| `internal/db/sql/sqlc.yaml` (pgtype purge, domain IDs, enums)                   | skeleton template       | Stamped by `beach new`; `lint-pgtype` / `lint-uuid` ship as framework checks                                    |
| `internal/pgnotify`                                                             | `pgnotify`              | Lifted verbatim: dedicated conn, exponential backoff, drop-on-slow-consumer                                     |
| `internal/notify/registry.go` (per-user subscription fan-out)                   | `hub`                   | Generalized from per-user to per-topic                                                                          |
| `cmd/live/handler/hub.go` + `cmd/admin/handler/sse_helpers.go`                  | `hub`                   | The two SSE mental models (event-driven hub, tick loop) unify under topics                                      |
| `internal/db/*_cache.go` (atomic snapshot + NOTIFY invalidation)                | `cache`                 | Generic `cache.Snapshot[T]` + `cache.Keyed[K,V]` with a NOTIFY-invalidation helper                              |
| `pkg/session` (Valkey store, OptionalAuth/RequireAuth/RequireRole)              | `session`               | Fix the Data/AuthUser duplication wart                                                                          |
| `internal/serviceconfig` (reflect `env:` loader)                                | `config`                | Lifted with the `A\|B` fallback syntax                                                                          |
| `Makefile` shape (`gen`/`vet`/`up`, pinned tools, FA subsetting)                | skeleton template       | Stamped by `beach new`                                                                                          |
| Bench rigs (`chatperf`, `ssesim` patterns)                                      | `cmd/examples` + docs   | SSE fan-out budget testing is a framework deliverable, not app folklore                                         |

## What gets fixed (known warts, named in the audits)

- **Two SSE mental models** — admin tick loops vs live hub. Unified: everything is a
  topic; a ticker is just a producer that publishes to a topic on an interval.
- **SSE responses ship uncompressed** — manifold skips gzip for SSE because Echo's
  middleware buffers events. The framework compresses in the SSE writer itself with
  a per-event sync flush instead
  ([SSE compression](../architecture/03-http.md#sse-compression)).
- **Chrome nullability** — `chrome.Lookup` failures fall through silently. The App
  builder refuses to start without a registered chrome.
- **Session Data / AuthUser field duplication** — one type, derived view.
- **Inline per-service CSP strings** — `web.CSP` builder with env-aware presets.
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
