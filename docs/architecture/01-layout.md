# Architecture 1 — Stack contract & module layout

[← docs index](../README.md) · next: [Boot spine](02-boot-spine.md)

## Stack contract

net/http (Go 1.22 ServeMux) · templ · Datastar (Go SDK `starfederation/datastar-go` + free client core) · Tailwind v4 + rybitten (CSS custom properties) ·
pgx/v5 + pgxpool · sqlc · goose · Postgres (LISTEN/NOTIFY) ·
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
├── pg/                   pool, advisory-lock Migrate, InTx, Listen/Notify
├── datastar/             typed data-* attribute builders (the only sanctioned way)
├── ui/                   UI toolkit: Icon + Defer (.templ), the performance laws
│   ├── driftwood/        the kit — exported package-level templ components
│   └── specimen/         the living showcase page (tokens, icons, components,
│                         gamut preview, chart gallery)
├── chart/                SSR SVG charts, one package: 22 types of pure-Go
│                         geometry (.go) + templ render (.templ), token-themed
├── ch/                   ClickHouse: client, goose migrations, generic Batcher
│                         ingest, query→chart helpers
├── hub/                  topic registry, per-connection fan-out, ticker producers
├── cache/                Snapshot[T] and Keyed[K,V] caches + NOTIFY invalidation
├── session/              Postgres-backed sessions (SHA256 tokens, rotation, CSRF)
├── i18n/                 flat key catalog, locale files, T(ctx, key), middleware
├── passwords/            argon2id hashing, PHC strings — zero deps
├── auth/                 principal model (RBAC), login helpers, pretoken, lockout
├── ecs/                  standalone engine: schema codegen, archetype Store,
│                         change ticks, snapshots — zero deps, zero beach imports
├── sim/                  tick-loop integration over ecs: commands, systems,
│                         projections, persistence lanes
├── internal/             private cross-package helpers (flate writer pooling,
│                         vet analyzer internals) — not importable externally
├── cmd/
│   ├── beach/            CLI: beach new / ecs gen / i18n
│   │   └── templates/    embedded app template stamped by `beach new`
│   ├── beach-vet/        the lint analyzers as a CI gate
│   └── beach-apigen/     sqlc process plugin: SQL annotations → routes, handler
│                         stubs, NOTIFY plumbing (hypermedia-only)
├── examples/             the four validation apps (15-examples.md), each shaped
│   ├── boardwalk/        exactly like a consuming app:
│   │                     Monopoly-style real-estate game — the sim/ecs showcase
│   ├── driftbottle/      Omegle-style stranger chat — the hub/SSE showcase,
│   │                     hosts the 5k fan-out benchmark
│   ├── pantry/           at-home grocery ERP — the apigen/auth/ch/chart showcase
│   └── hush/             gated anonymous board — custom layout + client-JS
│                         island, storage + Repute (18-hush.md)
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
spine (`config`, `pg`) stands alone; `ecs` imports nothing and is
importable by anything, the-beach or not; `sim` is the only bridge between `ecs` and
the web packages; the root `beach` package is what an app touches first.

## What a consuming app looks like

Each app in `examples/` is this exact shape (what `beach new` stamps):

```
myapp/
├── main.go                wiring: beach.Config, routes, handlers
├── views.templ            every view, as templ components over driftwood
├── views_templ.go         committed templ output (make gen regenerates)
├── internal/db/           sqlc-generated + sql/{migrations,queries}/ + sqlc.yaml
├── components.beach.yaml  ECS component schema (if the app uses sim)
├── Makefile               gen (templ + css + sqlc + ecs gen) / vet / up / build
├── go.mod                 with a `tool` directive for templ
└── docker-compose.yml     postgres + services
```
