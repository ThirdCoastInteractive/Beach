# Architecture 1 — Stack contract & module layout

[← docs index](../README.md) · next: [Boot spine](02-boot-spine.md)

File references like `manifold13.tv/internal/web/sse.go` throughout the architecture
docs point at the reference implementation in `X:\manifold-13\manifold13.tv`.

## Stack contract

Echo v4 · templ · Datastar (Go SDK `starfederation/datastar-go`) · Tailwind v4 ·
pgx/v5 + pgxpool · sqlc · goose · Postgres (LISTEN/NOTIFY) · Valkey · esbuild ·
custom `ecs` engine (no third-party ECS). Go 1.26+.

These are not pluggable. The framework's value is that every layer knows exactly what
the other layers are.

## Module layout

```
github.com/ThirdCoastInteractive/the-beach
├── config/        env-tag struct loader, MustLoad
├── pg/            pool wrapper, advisory-lock Migrate, InTx, DBTX conventions
├── pgnotify/      LISTEN with reconnect/backoff, Notify helper
├── web/           App builder, middleware, handler shapes, StaticCache, MergedFS, CSP
├── datastar/      typed data-* attribute builders (the only sanctioned way)
├── hub/           topic registry, per-connection fan-out, ticker producers
├── cache/         Snapshot[T] and Keyed[K,V] caches + NOTIFY invalidation
├── session/       Valkey-backed sessions, auth middleware (Optional/Require/Role)
├── chrome/        component interface, registry, context, icons, token contract
│   └── driftwood/ minimal reference chrome
├── ecs/           standalone engine: schema codegen, archetype storage, change ticks, snapshots
├── world/         web integration over ecs: command channel, tick loop, dirty→topic, persistence
├── cmd/
│   ├── beach/     scaffold + vet CLI (embeds skeleton/)
│   └── examples/
│       └── boardwalk/   validation app: chat + presence + XP ticks + benchmark
└── docs/
```

The package dependency diagram lives in the
[docs index](../README.md#package-dependencies). The shape in one sentence: the boot
spine (`config`, `pg`, `pgnotify`) and `session` stand alone; `ecs` imports nothing
and is importable by anything, the-beach or not; `world` is the only bridge between
`ecs` and the web packages.

## What a consuming app looks like

Manifold's shape, minus everything the framework now owns:

```
myapp/
├── cmd/<service>/         main.go (≈40 lines), handler/, tmpl/{layout,page,ui}/, static/, js/
├── internal/db/           sqlc-generated + sql/{migrations,queries}/ + sqlc.yaml
├── internal/view/css/     input.css (tokens + @source globs)
├── Makefile               gen / vet / up / logs / down (stamped by beach new)
└── docker-compose.yml     postgres + valkey + services
```
