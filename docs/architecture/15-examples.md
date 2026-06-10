# Architecture 15 — Example apps: the validation trio

[← docs index](../README.md) · prev: [Analytics](14-analytics.md)

Three example apps under `examples/`, each shaped exactly like a consuming app
(`beach new` output + features), each chosen to stress a different slice of the
framework. Together they are the falsifiable claim that the framework works — every
package is exercised by at least one of them, and every perf gate runs against one
of them.

## `examples/boardwalk` — real-estate game (Monopoly-like)

The **`sim`/`ecs` showcase**: a multiplayer, tick-driven board game.

- The ruleset is the smallest playable slice — roll, move, buy, rent, trade. No
  auctions, mortgages, or jail lawyering in v1; the point is the framework, not
  tournament fidelity.
- Game state lives in the sim: player positions, turn order, dice, property
  ownership, rent timers — components in the `ecs.Store`, mutated by systems,
  pushed to every player via projections. The board everyone watches is one hub
  topic; your hand/cash is your `user:<id>` topic.
- **The persistence lanes get their canonical demo**: buying a property or trading
  is a synchronous `pg.InTx` commit (money — the [boundary rule](08-sim.md#the-boundary))
  reflected into the sim via `ApplyCommitted`; positions and turn state are
  write-behind; the cash ledger is the append-only journal.
- Game lobby/accounts via `auth`; spectator mode is an unauthenticated stream.
- Validates: `ecs`, `sim`, hub topics, command channel + `Ask` (roll dice →
  result), projections, all three persistence lanes.

## `examples/driftbottle` — stranger chat (Omegle-style, text only)

The **`hub`/SSE showcase**, and the host of the fan-out benchmark.

- Anonymous by design: no accounts — the `auth` principal's `anonymous` kind does
  real work for once. A matchmaking queue pairs two strangers onto a private
  ephemeral topic (`pair:<id>`); disconnect or "next" tears it down and re-queues.
- Everything is ephemeral: no chat persistence, sessions are the only state —
  which makes it the purest possible test of the hub, SSE compression, presence,
  and reconnect/`?since=` behavior with nothing else in the way.
- **Hosts the 5k-connection benchmark** (single container, SSE gzip on,
  per-connection memory reported) and the moderation hooks demo (rate-limit per
  session, word filters).
- Validates: hub at scale, `StreamFunc`, SSE compression budget, matchmaking on
  the command channel, anonymous principals, the 14KB rule (the whole app should
  land well under it).

## `examples/pantry` — at-home grocery ERP

The **CRUD/`apigen`/`auth`/analytics showcase**: boring in the good way, on purpose.

- Household inventory: items, storage locations, quantities, expiry dates,
  shopping lists, purchase history. Almost every route is **generated** — the
  `@api`/`@page`/`@fragment`/`@requires` annotations on sqlc queries
  ([13-apigen.md](13-apigen.md)) produce the CRUD surface; hand-written handlers
  should be the rare exception, and that ratio is the measure of apigen's worth.
- `auth` with real roles (household admin vs member), the full login flow,
  the authz analyzer in fatal mode.
- **The `ch` + `chart` showcase**: purchases and consumption events batch into
  ClickHouse; dashboards render spend-over-time (line), category breakdown
  (stacked bar), price-history sparklines per item, expiry calendar heatmap,
  and a waste gauge — every widget a `ui.Defer` section feeding `chart`
  components from `ch` queries ([14-analytics.md](14-analytics.md)).
- Validates: apigen end-to-end, auth/RBAC, `ch`, `chart`, `i18n` (pantry is the
  most text-heavy app, so it carries the translation catalog), deferred dashboard
  sections.

## Coverage matrix

| Package | boardwalk | driftbottle | pantry |
| --- | :-: | :-: | :-: |
| beach (root) / datastar / ui | ● | ● | ● |
| hub | ● | **●●** | ○ |
| ecs / sim | **●●** | ○ | — |
| auth / passwords | ● | ● (anonymous) | **●●** |
| apigen | ○ | — | **●●** |
| ch / chart | ○ | — | **●●** |
| i18n | ○ | — | **●●** |
| cache / session / pg / config | ● | ● | ● |

●● = the showcase, ● = exercised, ○ = incidental, — = unused (deliberately: not
every app needs every package, and the examples should prove that too).
