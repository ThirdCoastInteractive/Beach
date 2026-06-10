# the-beach

A hypermedia-first, server-rendered web framework for Go. Extracted from
manifold13.tv, extended with an entity component system for realtime app state.

**Stack (non-negotiable):** Echo · templ · Datastar · Tailwind · pgx · sqlc · goose ·
Postgres LISTEN/NOTIFY · SSE · Valkey · custom `ecs` engine.

Hypermedia-first. Server-driven. The first paint is the final paint. Boring in the
good way.

## Docs

Start at the [docs index](docs/README.md) — it holds the overall diagrams (realtime
spine, package dependencies) and links every sub-document:

- [RFC](docs/rfc/01-charter.md) (4 docs) — charter, extraction from manifold13.tv,
  the ECS pitch, program of projects + risks.
- [Architecture](docs/architecture/01-layout.md) (15 docs) — module layout (per
  [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)), boot spine, the
  root `beach` package + handler shapes, hub, cache/session, the `ui` toolkit
  (14KB rule, deferred sections), the `ecs` engine, the `sim`, lifecycles, tooling,
  decisions log, auth (argon2id + RBAC, from Atlas), API codegen (from Atlas),
  analytics (`chart` SSR SVG + `ch` ClickHouse), and the example trio
  (boardwalk / driftbottle / pantry).

## Branching & releases

| Branch   | Remote   | Discipline                                                        |
| -------- | -------- | ------------------------------------------------------------------ |
| `master` | `origin` | Dev. Messy WIP commits welcome — commit early, commit often.      |
| `main`   | `public` | Release. ONLY squashed summary commits of finished work.          |

Never commit directly to `main`. When a unit of work is finished on `master`:

```bash
scripts/release.sh "summary of the finished work"
```

That squash-merges `master` into `main` as one commit whose message reads like a
changelog entry, pushes `main` to the public remote, and pushes `master` to origin.

## Status

Pre-implementation. The RFC and architecture writeup are the current deliverables;
project 1 (`config` + `pg`) is the first code.
