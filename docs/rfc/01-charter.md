# RFC 1 — Charter: problem, goal, non-goals

[← docs index](../README.md) · next: [Scope](02-scope.md)

A reusable, hypermedia-first, SSR web framework for Go, with an entity component
system for realtime app state.

Module: `github.com/ThirdCoastInteractive/Beach`

## Problem

The stack — Go + net/http + templ + Datastar + Tailwind v4 + `rybitten`
+ sqlc + pgx + goose + Postgres
LISTEN/NOTIFY + SSE — is proven in production at real scale: live platforms
sustaining thousands of concurrent SSE connections, full design-system theming,
annotation-driven code generation, hardened authentication. But until now it has
lived as convention and copy-paste: every new property re-types the same typed
Datastar builders, the same component registry, the same semantic-token theming,
the same `make gen/vet/up` workflow, the same no-pop-in rendering discipline, the
same advisory-lock migrations and snapshot caches. Each rebuild improves the
patterns — and strands the improvements in one repo.

The shared skeleton should be a module you import, not a vibe you re-type.

Separately: much of what these applications manage is genuinely game-shaped state —
viewers, XP, items, inventories, crafting, trading, economies, presence, live show
state. Per-feature services, caches, and SSE hubs can carry that load, but an
entity component system gives hot, tick-mutated, cross-cutting state one fast home —
and, paired with change detection, makes "re-render and push only what changed"
fall out for free.

## Goal

One Go module that provides:

1. The proven web/data/realtime skeleton as importable packages.
2. An ECS sim module for hot realtime state, wired into the same SSE/Datastar
   rendering pipeline.
3. A scaffold (`beach new`) that stamps out a new app with the Makefile, sqlc.yaml,
   Stellar config *[shipped as `views.templ` + a templ-tool go.mod instead]*, lint
   guards, and docker-compose shape already correct.

A new property should go from `git init` to a styled, SSE-live, DB-backed page in under
an hour, and inherit every house rule (no pop-in, no blocking, ≤14KB first responses,
typed builders only, no pgtype leakage, vendored assets) as enforced tooling rather
than tribal knowledge.

## Non-goals

- Not a general-purpose framework for other people's stacks. It is opinionated to the
  point of rudeness: net/http, templ, Datastar, Stellar *[now Tailwind v4 +
  `rybitten`]*, pgx, sqlc, goose, Postgres.
  No adapters, no `interface{} Driver` seams.
- Not an ORM. sqlc stays. Plain SQL stays.
- The UI is hypermedia or bust: server-rendered HTML over the wire, Datastar for
  interaction, state on the server. That rules out an SPA mode and a JSON/machine-API
  layer — Beach renders pages, not endpoints. An app that genuinely needs a machine
  API writes its own RFC.
- No multi-node story in v1. LISTEN/NOTIFY is the cross-process bell; the seam where
  NATS would slot in later is identified but not built.
- No migration of existing production services onto the framework as part of this
  work. The framework proves itself on greenfield apps first; migrations are their
  own programs, decided afterward.
- *[Updated 2026-06-12: no licensed asset remains (free Datastar core, locally
  generated CSS). Updated 2026-09-01: the module path is
  `github.com/ThirdCoastInteractive/Beach`.]*

## Philosophy

Grug-brained throughout: complexity very bad. The framework exists to trap already-won
complexity behind narrow interfaces, not to invent new complexity. Hypermedia-first,
server-driven, boring in the good way. The first paint is the final paint. HTML and CSS
first — the only JavaScript is Datastar, reserved for talking to the server; local
interactivity (disclosure, dialogs, tabs, toggles) is markup and CSS, never script. No
external CDNs. Make targets only. A task is not complete until every verification step
passes.
