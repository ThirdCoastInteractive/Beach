# the-beach — documentation index

A reusable, hypermedia-first, SSR web framework for Go. Extracted from manifold13.tv,
hardened, and extended with a custom entity component system for realtime app state.
Stack: Echo · templ · Datastar · Tailwind · pgx · sqlc · goose · Postgres
LISTEN/NOTIFY · SSE · Valkey · custom `ecs` engine.

**Status:** pre-implementation. The RFC and architecture docs below are the current
deliverables; project 1 (`config` + `pg` + `pgnotify`) is the first code.

## The realtime spine

How a beach app hangs together — handlers, the ECS world, the hub, and Postgres:

```mermaid
flowchart LR
    B["Browser<br/>(templ SSR + Datastar)"]
    subgraph proc["the-beach app process"]
        H["handlers<br/>(echo + sqlc + templ)"]
        W["world (ECS)<br/>single goroutine, fixed ticks"]
        HUB["hub<br/>topic fan-out"]
        PN["pgnotify listener"]
    end
    PG[("Postgres<br/>system of record")]
    EXT["other writers<br/>admin tools / cron / other nodes"]

    B -- "pages + Datastar actions" --> H
    H -- "commands (chan)" --> W
    H -- "sqlc / pg.InTx" --> PG
    H -- "publish" --> HUB
    W -- "write-behind batches" --> PG
    W -- "dirty fragments,<br/>rendered once per topic" --> HUB
    HUB -- "SSE datastar-patch-elements" --> B
    EXT -- "SQL writes" --> PG
    PG -- "NOTIFY (ids only)" --> PN
    PN -- "reload commands" --> W
```

## Package dependencies

Arrows point at what a package imports. The boot spine and `session` stand alone;
`ecs` imports nothing and is importable by anything, the-beach or not:

```mermaid
flowchart BT
    config["config"]
    pg["pg"]
    pgnotify["pgnotify"]
    datastar["datastar"]
    chrome["chrome + driftwood"]
    web["web"]
    hub["hub"]
    cache["cache"]
    session["session"]
    ecs["ecs<br/>standalone, zero deps"]
    world["world"]
    app(["your app<br/>handlers / tmpl / internal/db"])
    other(["other contexts<br/>games / sims / tools"])

    chrome --> datastar
    web --> chrome
    web --> datastar
    web --> config
    cache --> pgnotify
    hub --> datastar
    world --> ecs
    world --> hub
    world --> pg
    app --> web
    app --> hub
    app --> cache
    app --> session
    app --> world
    app --> pg
    app --> pgnotify
    app --> config
    other --> ecs
```

## RFC — the why and the what

| Doc                                          | Covers                                                                |
| -------------------------------------------- | --------------------------------------------------------------------- |
| [1 — Charter](rfc/01-charter.md)             | Problem, goal, non-goals, philosophy                                  |
| [2 — Extraction](rfc/02-extraction.md)       | What gets lifted from manifold13.tv, what warts get fixed             |
| [3 — The ECS pitch](rfc/03-ecs.md)           | Why a custom engine, the design pillars, the hybrid boundary          |
| [4 — Program & risks](rfc/04-program.md)     | The 8 shippable projects, validation criteria, risks                  |

## Architecture — the how

| Doc                                                       | Covers                                                              |
| --------------------------------------------------------- | -------------------------------------------------------------------- |
| [1 — Layout](architecture/01-layout.md)                   | Stack contract, module layout, consuming-app shape                  |
| [2 — Boot spine](architecture/02-boot-spine.md)           | `config`, `pg` (advisory-lock migrate, InTx), `pgnotify`            |
| [3 — HTTP layer](architecture/03-http.md)                 | App builder, `PageFunc`/`ActionFunc`/`StreamFunc`, guards, datastar |
| [4 — Hub](architecture/04-hub.md)                         | Topic fan-out, render-once-per-topic, tickers                       |
| [5 — Cache & session](architecture/05-services.md)        | Snapshot/keyed caches, NOTIFY invalidation, Valkey sessions         |
| [6 — Chrome](architecture/06-chrome.md)                   | Component interface, semantic tokens, the no-pop-in law             |
| [7 — ECS engine](architecture/07-ecs.md)                  | The custom standalone engine: schema, storage, change ticks         |
| [8 — World](architecture/08-world.md)                     | The boundary, tick loop, dirty→fragments, persistence lanes         |
| [9 — Lifecycles](architecture/09-lifecycles.md)           | Page / action / stream / external write, end to end                 |
| [10 — Tooling & testing](architecture/10-tooling.md)      | `beach new` / `beach vet` / `beach ecs gen`, test doctrine          |
| [11 — Decisions log](architecture/11-decisions.md)        | Every load-bearing choice and why                                   |

## Reading paths

- **Auditing the plan?** RFC 1 → 2 → 3 → 4, then the decisions log.
- **Auditing the ECS?** RFC 3 → Architecture 7 → 8 → 9.
- **Building project 1?** Architecture 1 → 2, RFC 4 for scope.
