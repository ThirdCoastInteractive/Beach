# Architecture 10 — Tooling, testing & performance doctrine

[← docs index](../README.md) · prev: [Lifecycles](09-lifecycles.md) · next: [Decisions log](11-decisions.md)

## `cmd/beach` and the skeleton

`beach new <app>` stamps: Makefile (`help` default; `setup` with pinned tool versions
— `@latest` drift is a documented sin; coupled `gen` = templ+tailwind+sqlc+esbuild;
`vet` = gen + analyzers + `go vet`; `up`/`logs`/`down`; `start-db`/`stop-db` with the
"never rm-db to fix migrations" warning), sqlc.yaml (pgtype purge + domain-ID
override examples), input.css (token layer + `@source` globs), docker-compose
(Postgres + Valkey), `.env` template, esbuild entries, and a `cmd/app` with one page,
one Datastar action, one SSE fragment — proving the whole pipe on first `make up`.

`beach vet` runs the analyzers: raw Datastar attribute strings, pgtype imports
outside `internal/db`, `google/uuid` outside the sanctioned package, hardcoded
hex/OKLCH in templ class attributes, raw `<i class="fa-...">` icons, naked
`echo.HandlerFunc` registered outside `app.Raw`.

`beach ecs gen` runs the component codegen from `components.beach.yaml`
([ECS engine](07-ecs.md#the-portable-core-model)).

Verification pipeline is framework doctrine, stamped into the skeleton's CLAUDE.md:
`make gen` → `make vet` → `make up` → browser check (no console errors, change
visible, no 404s). A task is not complete until every step passes.

## Testing & performance doctrine

- Integration tests over unit tests (grug sweet spot); regression test before fixing
  any bug; minimal mocking — world tests use a real world, db tests a real Postgres.
- World tests: construct → send → tick → assert. Determinism is the feature.
- Playwright e2e smoke per app, `workers=1` (SSE handshakes don't tolerate parallel
  handshakes against one container).
- boardwalk ships the fan-out benchmark (ssesim/chatperf lineage): 5k SSE connections
  on one container is the budget the hub must meet.
- Perf history as JSONL tracked in git so trends show up in diffs.
