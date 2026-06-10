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
- [Architecture](docs/architecture/01-layout.md) (11 docs) — layout, boot spine,
  HTTP layer + handler shapes, hub, cache/session, chrome, the `ecs` engine, the
  world, lifecycles, tooling, decisions log.

## Status

Pre-implementation. The RFC and architecture writeup are the current deliverables;
project 1 (`config` + `pg` + `pgnotify`) is the first code.
