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

The layout follows [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)
("multiple packages" + "packages and commands in the same repository"): the **module
root is the primary entry point** (`package beach` — the App builder, `Ctx`, and
handler shapes live there, not in a `web/` sub-package), importable packages are flat
top-level directories, private helpers go in `internal/`, binaries in `cmd/`. No
`pkg/` directory — the layout doc doesn't use one and neither do we.

```
the-beach/                module github.com/ThirdCoastInteractive/the-beach
├── go.mod
├── beach.go …            package beach — the root IS the framework's front door:
│                         App builder, Ctx, PageFunc/ActionFunc/StreamFunc, guards,
│                         View/Patches/Sub, NewSSE (per-event gzip), StaticCache,
│                         MergedFS, CSP presets
├── config/               env-tag struct loader, MustLoad
├── pg/                   pool wrapper, advisory-lock Migrate, InTx
├── pgnotify/             LISTEN with reconnect/backoff, Notify helper
├── datastar/             typed data-* attribute builders (the only sanctioned way)
├── ui/                   UI toolkit: Kit interface, registry, tokens, icons,
│   │                     Defer (lazy sections), the performance laws
│   └── driftwood/        minimal reference kit
├── chart/                SSR SVG charts: 22 types, pure-Go geometry + templ
│                         components, token-themed (lifted from Atlas)
├── ch/                   ClickHouse: client, goose migrations, generic Batcher
│                         ingest, query→chart helpers (built to Atlas's RFC)
├── hub/                  topic registry, per-connection fan-out, ticker producers
├── cache/                Snapshot[T] and Keyed[K,V] caches + NOTIFY invalidation
├── session/              Valkey-backed sessions (SHA256 tokens, rotation, CSRF)
├── passwords/            argon2id hashing, PHC strings — zero deps, lifted from Atlas
├── auth/                 principal model (RBAC), login helpers, pretoken, lockout
├── ecs/                  standalone engine: schema codegen, archetype Store,
│                         change ticks, snapshots — zero deps, zero beach imports
├── sim/                  tick-loop integration over ecs: commands, systems,
│                         projections, persistence lanes
├── internal/             private cross-package helpers (flate writer pooling,
│                         vet analyzer internals) — not importable externally
├── cmd/
│   ├── beach/            CLI: beach new / vet / ecs gen
│   │   └── skeleton/     embedded app template stamped by `beach new`
│   └── beach-apigen/     sqlc process plugin: SQL annotations → routes, handler
│                         stubs, NOTIFY plumbing, optional OpenAPI (from Atlas)
├── examples/             the validation trio (15-examples.md), each shaped
│   ├── boardwalk/        exactly like a consuming app:
│   │                     Monopoly-style real-estate game — the sim/ecs showcase
│   ├── driftbottle/      Omegle-style stranger chat — the hub/SSE showcase,
│   │                     hosts the 5k fan-out benchmark
│   └── pantry/           at-home grocery ERP — the apigen/auth/ch/chart showcase
└── docs/
```

Import lines read like this (the root package is named `beach`, so callers alias the
hyphenated module path — the `go-cmp`/`cmp` convention):

```go
import (
    beach "github.com/ThirdCoastInteractive/the-beach"
    "github.com/ThirdCoastInteractive/the-beach/sim"
    "github.com/ThirdCoastInteractive/the-beach/ui"
)
```

The package dependency diagram lives in the
[docs index](../README.md#package-dependencies). The shape in one sentence: the boot
spine (`config`, `pg`, `pgnotify`) stands alone; `ecs` imports nothing and is
importable by anything, the-beach or not; `sim` is the only bridge between `ecs` and
the web packages; the root `beach` package is what an app touches first.

## What a consuming app looks like

Manifold's shape, minus everything the framework now owns. Each app in `examples/`
is this exact shape:

```
myapp/
├── cmd/<service>/         main.go (≈40 lines), handler/, tmpl/{layout,page,ui}/, static/, js/
├── internal/db/           sqlc-generated + sql/{migrations,queries}/ + sqlc.yaml
├── internal/view/css/     input.css (tokens + @source globs)
├── components.beach.yaml  ECS component schema (if the app uses sim)
├── Makefile               gen / vet / up / logs / down (stamped by beach new)
└── docker-compose.yml     postgres + valkey + services
```
