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
("multiple packages" + "packages and commands in the same repository") with an
explicit public/private split: the **external API lives under `pkg/`**
(`pkg/beach` is the front door — the App builder, `Ctx`, and handler shapes —
alongside `pkg/chart`, `pkg/ecs`, `pkg/ui`, …), **private and tooling code lives
under `internal/`**, and binaries live in `cmd/`. (This revises the original
flat-root layout — `package beach` at the module root — adopted 2026-06-14 to
make the importable surface explicit; `pkg/beach` is still the package an app
touches first.)

```
Beach/                module github.com/ThirdCoastInteractive/Beach
├── go.mod
├── pkg/                   the external API surface
│   ├── beach/            package beach — the framework's front door: App builder,
│   │                     Ctx, PageFunc/ActionFunc/StreamFunc, guards, View/Patches/
│   │                     Sub, NewSSE (per-event gzip), StaticCache, MergedFS, CSP
│   │   └── view/         package view — the framework's embedded asset tree
│   │                     (app.css input + build output, the datastar/chart
│   │                     client JS). A package, not just a directory, so the
│   │                     design tokens declared in the stylesheet can be read
│   │                     from Go: view.Tokens() is what the contrast tests and
│   │                     the specimen's contrast table measure
│   ├── config/           env-tag struct loader, MustLoad
│   ├── pg/               Pool/MustPool, advisory-lock Migrate, InTx, Listen/Notify
│   ├── datastar/         typed data-* attribute builders (the only sanctioned way)
│   ├── ui/               UI toolkit: Icon + Defer (.templ), the house laws
│   │   ├── driftwood/    the kit — exported package-level templ components
│   │   └── specimen/     the living showcase page (tokens, icons, components,
│   │                     gamut preview, chart gallery)
│   ├── chart/            SSR SVG charts, one package: 22 types of pure-Go
│   │                     geometry (.go) + templ render (.templ), token-themed
│   ├── ch/               ClickHouse: client, goose migrations, generic Batcher
│   │                     ingest, query→chart helpers
│   ├── hub/              topic registry, per-connection fan-out, ticker producers
│   ├── cache/            Snapshot[T] and Keyed[K,V] caches + NOTIFY invalidation
│   ├── session/          Postgres-backed sessions (SHA256 tokens, rotation, CSRF)
│   ├── i18n/             flat key catalog, locale files, T(ctx, key), middleware
│   ├── prefs/            the visitor's timing preferences (pause live updates,
│   │                     stop toast timers) — a leaf package for the same reason
│   │                     i18n is: beach resolves them, driftwood reads them
│   ├── passwords/        argon2id hashing, PHC strings — zero deps
│   ├── auth/             principal model (RBAC), login helpers, pretoken, lockout
│   ├── mailer/ sms/ storage/ rybitten/   transactional mail · SMS (Twilio) · R2/Images · RYB palettes
│   ├── ecs/              standalone engine: schema codegen, archetype Store,
│   │                     change ticks, snapshots — zero deps, zero beach imports
│   └── sim/              tick-loop integration over ecs: commands, systems,
│                         projections, persistence lanes
├── internal/             private, not importable externally:
│                         deps (flate writer pooling), lint (vet analyzers),
│                         perf (benchmark/page-size harness)
├── cmd/
│   ├── beach/            CLI: beach new / ecs gen / i18n
│   │   └── templates/    embedded app template stamped by `beach new`
│   ├── beach-vet/        the lint analyzers as a CI gate
│   ├── beach-apigen/     sqlc process plugin: SQL annotations → routes, handler
│   │                     stubs, NOTIFY plumbing (hypermedia-only)
│   ├── examples/         the four validation apps (15-examples.md), each shaped
│   │   ├── boardwalk/    exactly like a consuming app:
│   │   │                 Monopoly-style real-estate game — the sim/ecs showcase
│   │   ├── driftbottle/  Omegle-style stranger chat — the hub/SSE showcase,
│   │   │                 hosts the 5k fan-out benchmark
│   │   ├── pantry/       at-home grocery ERP — the apigen/auth/ch/chart showcase
│   │   └── booking-manager/  mailer/sms, Postgres-only
└── docs/
```

Import lines read like this (the front-door package is named `beach`, so callers
alias the hyphenated module path — the `go-cmp`/`cmp` convention):

```go
import (
    beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
    "github.com/ThirdCoastInteractive/Beach/pkg/sim"
    "github.com/ThirdCoastInteractive/Beach/pkg/ui"
)
```

The package dependency diagram lives in the
[docs index](../README.md#package-dependencies). The shape in one sentence: the boot
spine (`pkg/config`, `pkg/pg`) stands alone; `pkg/ecs` imports nothing and is
importable by anything, Beach or not; `pkg/sim` is the only bridge between
`pkg/ecs` and the web packages; the `pkg/beach` package is what an app touches first.

## What a consuming app looks like

Each app in `cmd/examples/` is this exact shape (what `beach new` stamps):

```
myapp/
├── main.go                thin: load config → construct concerns → wire web → run
├── config.go              typed env loader (pkg/config)
├── internal/
│   ├── web/               the app/server struct, handlers, and .templ views — the
│   │                      HTTP glue stays cohesive in one package
│   ├── store/             domain types + Postgres access (system of record)
│   ├── analytics/         ClickHouse firehose + dashboard queries (off hot path)
│   └── db/                sqlc-generated + sql/{migrations,queries}/ + sqlc.yaml
├── components.beach.yaml  ECS component schema (if the app uses sim)
├── Makefile               gen (templ + css + sqlc + ecs gen) / vet / up / build
├── go.mod                 with a `tool` directive for templ
└── docker-compose.yml     postgres + services
```

The split is **by concern**: `main.go` only wires; the app's `app`/`server`
struct and its handler methods + `.templ` views live together in `internal/web`
(splitting handlers off their receiver would just force mass-exporting), and
genuinely decouplable concerns (data store, sim, mail, ch-analytics) each get a
sibling `internal/` package. The four `cmd/examples/` apps are this shape at
varying sizes ([15-examples.md](15-examples.md)).
