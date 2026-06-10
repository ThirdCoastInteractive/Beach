# RFC 1 — Charter: problem, goal, non-goals

[← docs index](../README.md) · next: [What gets extracted](02-extraction.md)

A reusable, hypermedia-first, SSR web framework for Go. Extracted from manifold13.tv,
hardened, and extended with an entity component system for realtime app state.

Module: `github.com/ThirdCoastInteractive/the-beach`

## Problem

Four repos now carry the same stack — Go + Echo + templ + Datastar + Tailwind + sqlc +
pgx + goose + Postgres LISTEN/NOTIFY + SSE — and the same conventions (typed Datastar
builders, chrome component registry, semantic-token theming, `make gen/vet/up`,
no-pop-in rendering, advisory-lock migrations, snapshot caches with NOTIFY
invalidation):

- `manifold13.tv` — where the patterns were proven at production scale (5k-connection
  chat benchmarks, SSE fan-out budgets, live 24/7 platform).
- `thirdcoast.systems` — deliberately duplicated the patterns ("Lifted in spirit, not
  in code"): the retrochrome token layer, the Makefile shape, the Datastar typed-builder
  pattern, the Postgres + Valkey + per-service Dockerfile pattern.
- `Atlas` — rebuilt the stack a third time and evolved it: annotation-driven API
  codegen from SQL (`atlas-apigen`), a custom lint family (datastar, head/CSP,
  vendoring, authz coverage), and argon2id auth with a full principal/RBAC model.
  Each rebuild improves the patterns — and strands the improvements in one repo.
- Every future Third Coast property starts this dance again.

Duplicate-and-trim was the right call for repo number two. For repo number three and
beyond, the shared skeleton should be a module you import, not a vibe you re-type.

Separately: manifold's domain (viewers, XP, items, inventory, crafting, trading,
economy, drops, presence, show state) is genuinely game-shaped. The current
architecture handles it with per-feature services, caches, and SSE hubs. An entity
component system gives that hot, tick-mutated, cross-cutting state one fast home —
and, paired with change detection, makes "re-render and push only what changed" fall
out for free.

## Goal

One Go module that provides:

1. The proven web/data/realtime skeleton from manifold13.tv as importable packages.
2. An ECS sim module for hot realtime state, wired into the same SSE/Datastar
   rendering pipeline.
3. A scaffold (`beach new`) that stamps out a new app with the Makefile, sqlc.yaml,
   Tailwind input, lint guards, and docker-compose shape already correct.

A new property should go from `git init` to a styled, SSE-live, DB-backed page in under
an hour, and inherit every house rule (no pop-in, no blocking, ≤14KB first responses,
typed builders only, no pgtype leakage, vendored assets) as enforced tooling rather
than tribal knowledge.

## Non-goals

- Not a general-purpose framework for other people's stacks. It is opinionated to the
  point of rudeness: Echo, templ, Datastar, Tailwind, pgx, sqlc, goose, Postgres,
  Valkey. No adapters, no `interface{} Driver` seams.
- Not an ORM. sqlc stays. Plain SQL stays.
- No SPA mode, no client-side state management. The UI is hypermedia or bust — JSON
  is never how the frontend talks to the backend. Machine APIs (integrations, MCP
  servers — the Atlas case) are in scope but only as **generated** surfaces: the
  [apigen annotations](../architecture/13-apigen.md) emit OpenAPI + JSON handling
  from the same SQL; nobody hand-writes a JSON layer.
- No multi-node story in v1. LISTEN/NOTIFY is the cross-process bell; the seam where
  NATS would slot in later is identified but not built.
- manifold13.tv is NOT migrated onto the framework as part of this work. It is the
  reference quarry. Migration is its own program, decided after the framework proves
  itself on a greenfield app.

## Philosophy

Grug-brained throughout: complexity very bad. The framework exists to trap already-won
complexity behind narrow interfaces, not to invent new complexity. Hypermedia-first,
server-driven, boring in the good way. The first paint is the final paint. No external
CDNs. Make targets only. A task is not complete until every verification step passes.
