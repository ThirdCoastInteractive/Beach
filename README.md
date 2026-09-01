# Beach

A hypermedia-first, server-rendered web framework for Go, with an entity component
system for realtime app state.

```
git clone https://github.com/ThirdCoastInteractive/Beach
go get github.com/ThirdCoastInteractive/Beach
```

**Stack (non-negotiable):** net/http · templ · Datastar (free core) · Tailwind v4 +
rybitten · pgx · sqlc · goose · Postgres LISTEN/NOTIFY · SSE · custom `ecs` engine ·
WebSocket via vendored `coder/websocket` for the non-hypermedia channel
([RFC 05](docs/rfc/05-websockets.md)).

Hypermedia-first. Server-driven. The first paint is the final paint. Boring in the
good way.

## Docs

Start at the [docs index](docs/README.md) — it holds the overall diagrams (realtime
spine, package dependencies) and links every sub-document:

- [RFC](docs/rfc/01-charter.md) — charter, scope (packages + framework
  laws), the ECS pitch, WebSockets, accessibility.
- [Architecture](docs/architecture/01-layout.md) — module layout (per
  [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)), boot spine, the
  root `beach` package + handler shapes, hub, cache/session/i18n, the `ui` toolkit
  (templ views, the driftwood kit, 14KB rule, deferred sections), the component
  catalog + `ui/specimen`, the `ecs` engine, the `sim`, lifecycles, tooling,
  auth (argon2id + RBAC), API codegen, analytics (`chart` — SSR SVG,
  22 types, client interaction layer — + `ch` ClickHouse), RYB color & gamut
  theming (`rybitten`), and the example apps (boardwalk / driftbottle / pantry /
  booking-manager).

## Module

`github.com/ThirdCoastInteractive/Beach`. Default branch is `main`.

## Agents

This repo is meant to be used by coding agents. Start at [AGENTS.md](AGENTS.md).
Skills live under `skills/` (`beach`, `beach-ui`, `beach-a11y`, `beach-test`).
`beach new` stamps an `AGENTS.md` into a new app.

## Examples

```
make up-pantry             # CRUD / apigen / auth / charts
make up-driftbottle        # hub / SSE matchmaking
make up-boardwalk          # sim / ecs + snapshot
make up-booking-manager    # mailer / sms
```

Each example mounts the driftwood specimen at `/specimen`.

## Status

The framework, the four example apps, and the tooling compile, test, and serve
(see the [docs index](docs/README.md)).

Views are `.templ` components compiled by `templ generate` (a Go tool — see the
`tool` directive in go.mod). Run `make gen` after editing any `.templ` or CSS
file; it runs `go tool templ generate` and rebuilds `app.css`. Generated
`*_templ.go` files are committed, so plain `go build` works on a fresh clone.
`make build` / `make vet` / `make test` all depend on `gen`.
