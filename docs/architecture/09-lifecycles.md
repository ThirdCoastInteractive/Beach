# Architecture 9 — Lifecycles, end to end

[← docs index](../README.md) · prev: [World](08-world.md) · next: [Tooling & testing](10-tooling.md)

**Page request:** route → guards (session, role) → chrome middleware → `PageFunc` →
`db.New(pool)` → sqlc → `View.Page` rendered (final state, no pop-in) → gzip →
browser. A Datastar refresh of the same URL takes the same path but the framework
patches `View.Fragment` into `View.Target` instead.

**Datastar action:** `@post('/thing')` → guards → `ActionFunc` (framework already
verified `Datastar-Request` + CSRF) → `web.Bind` → mutation via `c.InTx` and/or
`w.Send` → returned `Patches` written over SSE — smallest fragment, actor's view →
(world/hub later pushes the same change to everyone else subscribed).

```mermaid
sequenceDiagram
    participant B as Browser (Datastar)
    participant H as handler
    participant PG as Postgres
    participant W as world (ECS)
    participant HUB as hub
    participant S as other subscribers

    B->>H: @post('/craft')  [Datastar-Request header]
    H->>PG: pg.InTx — commit items/trade
    PG-->>H: committed
    H->>W: Send(ApplyCommitted{...})
    H-->>B: SSE patch — smallest fragment (actor's view)
    Note over W: next tick: drain commands,<br/>run systems, collect dirty set
    W->>HUB: publish fragments, rendered once per topic
    HUB-->>B: datastar-patch-elements
    HUB-->>S: datastar-patch-elements
```

**SSE stream:** `@get('/stream')` on load → guards → `StreamFunc` returns a `Sub`
(topics + catch-up) → framework runs the loop: `?since=` catch-up render, subscribe,
block on the topic channels, write patches until disconnect → device/session row
closed, reaper sweeps stale rows (manifold's 30s reaper pattern).

**External write:** admin app commits → trigger NOTIFYs id → listener re-queries →
world command or direct hub publish → fragments patch on every subscribed client.
