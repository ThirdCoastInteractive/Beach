# the-beach

A hypermedia-first, server-rendered web framework for Go, with an entity component
system for realtime app state.

**Stack (non-negotiable):** net/http · templ · Datastar (free core) · Tailwind v4 +
rybitten · pgx · sqlc · goose · Postgres LISTEN/NOTIFY · SSE · custom `ecs` engine ·
WebSocket via vendored `coder/websocket` for the non-hypermedia channel
([RFC 05](docs/rfc/05-websockets.md)).

Hypermedia-first. Server-driven. The first paint is the final paint. Boring in the
good way.

## Docs

Start at the [docs index](docs/README.md) — it holds the overall diagrams (realtime
spine, package dependencies) and links every sub-document:

- [RFC](docs/rfc/01-charter.md) (4 docs) — charter, scope (packages + framework
  laws), the ECS pitch, program of projects + risks.
- [Architecture](docs/architecture/01-layout.md) (19 docs) — module layout (per
  [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)), boot spine, the
  root `beach` package + handler shapes, hub, cache/session/i18n, the `ui` toolkit
  (templ views, the driftwood kit, 14KB rule, deferred sections), the component
  catalog + `ui/specimen`, the `ecs` engine, the `sim`, lifecycles, tooling,
  decisions log, auth (argon2id + RBAC), API codegen, analytics (`chart` — SSR SVG,
  22 types, client interaction layer — + `ch` ClickHouse), RYB color & gamut
  theming (`rybitten`), and the example apps (boardwalk / driftbottle / pantry /
  hush).

## Branching & releases

| Branch   | Remote   | Discipline                                                   |
| -------- | -------- | ------------------------------------------------------------ |
| `master` | `origin` | Dev. Messy WIP commits welcome — commit early, commit often. |
| `main`   | `public` | Release. ONLY squashed summary commits of finished work.     |

Never commit directly to `main`. When a unit of work is finished on `master`:

```bash
scripts/release.sh "summary of the finished work"
```

That squash-merges `master` into `main` as one commit whose message reads like a
changelog entry, pushes `main` to the public remote, and pushes `master` to origin.

## Status & workflow

All six [roadmap](docs/ROADMAP.md) blocks are built; the framework, the four
example apps, and the tooling compile, test, and serve (see the
[docs index](docs/README.md) status line).

Views are `.templ` components compiled by `templ generate` (a Go tool — see the
`tool` directive in go.mod). Run `make gen` after editing any `.templ` or CSS
file; it runs `go tool templ generate` and rebuilds `app.css`. Generated
`*_templ.go` files are committed, so plain `go build` works on a fresh clone.
`make build` / `make vet` / `make test` all depend on `gen`.
