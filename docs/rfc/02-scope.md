# RFC 2 — Scope: what the framework provides

[← docs index](../README.md) · prev: [Charter](01-charter.md) · next: [The ECS pitch](03-ecs.md)

## The packages

| Package               | Responsibility                                                                                                                                                                                                           |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `beach` (module root) | App builder, middleware stack, the typed handler shapes (`PageFunc`/`ActionFunc`/`StreamFunc`), guards, error contract, SSE writer with per-event gzip, static cache with boot-time ETags, merged static FS, CSP presets |
| `config`              | Declarative env-var loading: `env:` struct tags, `A\|B` fallbacks, modifier flags, `MustLoad`                                                                                                                            |
| `pg`                  | Pool wrapper, advisory-lock migrations, `InTx`, and `Listen`/`Notify` with reconnect/backoff                                                                                                                             |
| `datastar`            | Typed `data-*` attribute builders — the only sanctioned way to emit Datastar attributes                                                                                                                                  |
| `ui`                  | The UI toolkit: `Kit` component interface and registry, the Stellar token system *[superseded: no Kit interface — one kit, `ui/driftwood`, direct templ components on Tailwind v4 + `rybitten` tokens]*, icon API, `Defer` lazy sections, the performance laws |
| `chart`               | Server-rendered SVG charts: 18 *[now 22]* types of pure-Go geometry plus templ components, token-themed, accessible, animated over SSE with zero client-side rendering *[a client interaction layer (tooltips/hovers/toolbar) was later added as progressive enhancement]* |
| `ch`                  | ClickHouse: native-protocol client, goose migrations, generic buffered `Batcher` ingest, typed query helpers                                                                                                             |
| `i18n`                | Flat-key translation catalog, per-locale JSON, literal-only `T(ctx, key)`, locale-resolution middleware                                                                                                                  |
| `hub`                 | Topic registry with per-connection buffered fan-out, drop-on-full, ticker producers                                                                                                                                      |
| `cache`               | `Snapshot[T]` and `Keyed[K,V]` in-process caches with NOTIFY-driven invalidation                                                                                                                                         |
| `session`             | Postgres-backed sessions: SHA256 token as PK, per-session CSRF, sliding TTL, rotation, RevokeAll — one datastore, sqlc-queried                                                                                           |
| `passwords`           | argon2id hashing (m=128MB, t=4, p=4; PHC strings; constant-time compare; rehash-on-upgrade) — zero dependencies                                                                                                          |
| `auth`                | The principal: three-table RBAC, `resource:action` permissions resolved at login, hardened login flow, `beach.Can()` guard                                                                                               |
| `ecs`                 | Standalone archetype ECS: schema-first codegen, columnar storage, tick-stamped change detection, snapshots — zero framework imports                                                                                      |
| `sim`                 | The web integration over `ecs`: single-writer tick loop, command channel, projections, persistence lanes                                                                                                                 |
| `cmd/beach`           | Scaffold and tooling CLI: `new`, `vet`, `ecs gen`, `i18n`                                                                                                                                                                |
| `cmd/beach-apigen`    | sqlc process plugin: six SQL annotations generate routes, handler stubs, and NOTIFY plumbing                                                                                                                             |

## The rules

These are framework law, enforced by the type system, the App builder, or
`beach vet` — not by review comments:

- **One SSE model.** Everything realtime is a hub topic; a dashboard ticker is just
  a producer publishing on an interval. There is no second mental model.
- **Kit registration is mandatory.** The App builder refuses to start without a
  registered UI kit — component lookup can never silently fall through.
  *[Retired with the Kit interface: components are direct Go calls now, so a
  missing component is a compile error — stronger than the rule it replaces.]*
- **One identity type.** The session's user data and the request's principal are
  one struct with derived views, never parallel copies.
- **CSP is built, not pasted.** `beach.CSP` presets with env awareness; no
  hand-maintained policy strings drifting between services.
- **`pg.InTx` is the transaction path.** No hand-rolled Begin/Rollback/Commit
  boilerplate in handlers.
- **NOTIFY-invalidation is the default** for in-process caches, not a per-cache
  afterthought.
- **HTML and CSS first.** Interactions the platform provides in markup and CSS —
  `<details>`, `<dialog>`, the `popover` attribute, `:target`/`:checked`/`:focus-within`,
  native form controls — ship with zero JavaScript. The one script dependency, Datastar,
  is reserved for server round-trips, shared signals, and SSE; local presentational state
  is never script ([architecture/06-ui.md](../architecture/06-ui.md#html-and-css-first)).
- **One Datastar detection signal.** The `Datastar-Request` header is the only
  sanctioned check, and `IsDatastar` is the only code that reads it.
- **SSE responses are compressed.** Gzip lives in the SSE writer with a per-event
  sync flush ([architecture/03-http.md](../architecture/03-http.md#sse-compression));
  skipping the gzip middleware for streams costs nothing.
- **First responses are budgeted.** Pages target ≤14KB compressed; heavier sections
  lazy-load through `ui.Defer` with exact reserved dimensions
  ([architecture/06-ui.md](../architecture/06-ui.md#the-14kb-rule)).
- **The house rules are types.** The dual-purpose page/fragment branch, per-request
  query objects, the SSE loop shape — all encoded in
  `PageFunc`/`ActionFunc`/`StreamFunc` with guards at route registration
  ([architecture/03-http.md](../architecture/03-http.md#handlers-three-typed-shapes)).
  Raw `http.HandlerFunc` survives only behind `app.Raw`, and `beach vet` enforces
  that.
