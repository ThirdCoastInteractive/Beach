# the-beach — documentation index

A reusable, hypermedia-first, SSR web framework for Go. Extracted from manifold13.tv,
hardened, and extended with a custom entity component system for realtime app state.
Stack: Echo · templ · Datastar · Tailwind · pgx · sqlc · goose · Postgres
LISTEN/NOTIFY · SSE · Valkey · custom `ecs` engine. The module root is the framework's
front door: `import beach "github.com/ThirdCoastInteractive/the-beach"`.

**Status:** pre-implementation. The RFC and architecture docs below are the current
deliverables; project 1 (`config` + `pg` + `pgnotify`) is the first code.

## The realtime spine

How a beach app hangs together — handlers, the sim, the hub, and Postgres:

```mermaid
flowchart LR
    B["Browser<br/>(templ SSR + Datastar)"]
    subgraph proc["the-beach app process"]
        H["handlers<br/>(beach + sqlc + templ)"]
        SIM["sim (ECS)<br/>single goroutine, fixed ticks"]
        HUB["hub<br/>topic fan-out"]
        PN["pgnotify listener"]
    end
    PG[("Postgres<br/>system of record")]
    EXT["other writers<br/>admin tools / cron / other nodes"]

    B -- "pages + Datastar actions" --> H
    H -- "commands (chan)" --> SIM
    H -- "sqlc / pg.InTx" --> PG
    H -- "publish" --> HUB
    SIM -- "write-behind batches" --> PG
    SIM -- "projections,<br/>rendered once per topic" --> HUB
    HUB -- "compressed SSE<br/>datastar-patch-elements" --> B
    EXT -- "SQL writes" --> PG
    PG -- "NOTIFY (ids only)" --> PN
    PN -- "reload commands" --> SIM
```

## Package dependencies

Arrows point at what a package imports. The boot spine stands alone; `ecs` imports
nothing and is importable by anything, the-beach or not; the root `beach` package is
what an app touches first:

```mermaid
flowchart BT
    config["config"]
    pg["pg"]
    pgnotify["pgnotify"]
    datastar["datastar"]
    ui["ui + driftwood kit"]
    chart["chart<br/>SSR SVG, 22 types"]
    ch["ch<br/>ClickHouse client + batcher"]
    beach["beach (module root)<br/>App, Ctx, handler shapes"]
    hub["hub"]
    cache["cache"]
    session["session"]
    passwords["passwords<br/>argon2id, zero deps"]
    auth["auth<br/>principal, guards, login"]
    ecs["ecs<br/>standalone, zero deps"]
    sim["sim"]
    app(["your app<br/>handlers / tmpl / internal/db"])
    other(["other contexts<br/>games / sims / tools"])

    ui --> datastar
    chart --> datastar
    beach --> ui
    beach --> datastar
    beach --> config
    beach --> session
    beach --> hub
    beach --> auth
    auth --> passwords
    auth --> session
    cache --> pgnotify
    sim --> ecs
    sim --> hub
    sim --> pg
    app --> beach
    app --> hub
    app --> cache
    app --> session
    app --> auth
    app --> sim
    app --> chart
    app --> ch
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
| [4 — Program & risks](rfc/04-program.md)     | The 10 shippable projects, validation criteria, risks                 |

## Architecture — the how

| Doc                                                       | Covers                                                                 |
| --------------------------------------------------------- | ----------------------------------------------------------------------- |
| [1 — Layout](architecture/01-layout.md)                   | Stack contract, module layout per go.dev/doc/modules/layout            |
| [2 — Boot spine](architecture/02-boot-spine.md)           | `config`, `pg` (advisory-lock migrate, InTx), `pgnotify`               |
| [3 — HTTP layer](architecture/03-http.md)                 | Root `beach` package: App builder, handler shapes, guards, SSE gzip    |
| [4 — Hub](architecture/04-hub.md)                         | Topic fan-out, render-once-per-topic, tickers                          |
| [5 — Cache & session](architecture/05-services.md)        | Snapshot/keyed caches, NOTIFY invalidation, Valkey sessions            |
| [6 — UI toolkit](architecture/06-ui.md)                   | Kits, tokens, no pop-in, no blocking, the 14KB rule, deferred sections |
| [7 — ECS engine](architecture/07-ecs.md)                  | The custom standalone engine: schema, Store, change ticks              |
| [8 — Sim](architecture/08-sim.md)                         | The boundary, tick loop, projections, persistence lanes                |
| [9 — Lifecycles](architecture/09-lifecycles.md)           | Page / action / stream / external write, end to end                    |
| [10 — Tooling & testing](architecture/10-tooling.md)      | `beach new` / `beach vet` (+ Atlas lint family) / `beach ecs gen`      |
| [11 — Decisions log](architecture/11-decisions.md)        | Every load-bearing choice and why                                      |
| [12 — Auth](architecture/12-auth.md)                      | argon2id passwords, login flow, principal/RBAC, guards (from Atlas)    |
| [13 — API codegen](architecture/13-apigen.md)             | `beach-apigen`: SQL annotations → routes, handlers, NOTIFY, OpenAPI    |
| [14 — Analytics](architecture/14-analytics.md)            | `chart` (SSR SVG, lifted from Atlas) + `ch` (ClickHouse, built to Atlas's RFC) |
| [15 — Example apps](architecture/15-examples.md)          | boardwalk (Monopoly/sim), driftbottle (stranger chat/hub), pantry (ERP/apigen+analytics) |

## Reading paths

- **Auditing the plan?** RFC 1 → 2 → 3 → 4, then the decisions log.
- **Auditing the ECS?** RFC 3 → Architecture 7 → 8 → 9.
- **Auditing the frontend story?** Architecture 3 → 6 → 9.
- **Auditing auth & codegen (the Atlas adoptions)?** Architecture 12 → 13 → 10.
- **Building project 1?** Architecture 1 → 2, RFC 4 for scope.
