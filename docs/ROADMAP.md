# Implementation roadmap

[← docs index](README.md)

Serial **blocks** (gates you pass in order), each ending in something you can run, see,
or test. Inside each block, **tracks** — subagent-sized chunks that run in parallel. No
dates; the unit is "a tangible thing works," not time.

## How to read it

- **Block** = a serial gate. Finish a block's integration before depending on it. Leaf
  tracks for later blocks *can* be pre-built early — the gate is about integration, not
  about when a leaf compiles.
- **Track** = one subagent's job. Tracks in the same block have no ordering between them
  unless an arrow says so.
- **◆ Gate** = the testable/viewable/tangible thing that proves the block is done.
- **Contract-first** is the rule that keeps parallel subagents from colliding: the first
  task in every block is to nail the thin interface the tracks code against (Kit methods,
  handler-shape signatures, the sim command API, the apigen grammar). Sign it, commit it,
  *then* fan out. Tracks build against the signed stub and integrate at the gate.

The biggest fan-outs are **Block 1** (the whole foundation, ~12 tracks), **Block 3** (one
subagent per UI sub-interface), and **Block 5** (three independent apps). Throw the most
subagents there.

---

## Block 0 — done

`config` (tested, zero deps). Stellar tokens generating to
`internal/view/static/css/stellar.css`. The plan/docs.

---

## Block 1 — Foundations · biggest fan-out

Every package that does **not** import the root `beach` package. No track here blocks
another except the few arrows below, so this is ~12 subagents at once.

*Internal ordering (small):* sign `pg`'s tiny API first (everyone else just needs its
shape); `passwords` + `session` land before `auth`.

| Track                       | What                                                                                                                                                    | Deps               |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------ |
| `pg`                        | pool (`MustPool`+Ping), advisory-lock `Migrate` (goose), `InTx`, `Listen`/`Notify` (reconnect/backoff)                                                  | pgx, goose         |
| `datastar`                  | typed `data-*` builders; `IsDatastar`; `NewSSE` writer with per-event gzip                                                                              | datastar-go        |
| `passwords`                 | argon2id, PHC strings, rehash-on-upgrade                                                                                                                | —                  |
| `session` → pg              | Postgres `sessions` table, sliding TTL, `Rotate`, `RevokeAll`, sweep                                                                                    | pg                 |
| `cache` → pg                | `Snapshot[T]`, `Keyed[K,V]`, `InvalidateOn` (NOTIFY)                                                                                                    | pg                 |
| `auth` → passwords, session | principal, login flow (pretoken/timing-safe/lockout), RBAC resolve-at-login, `Can` guard                                                                | passwords, session |
| `hub`                       | topic registry, buffered fan-out, drop-on-full, `?since=` cursor, `Ticker`                                                                              | —                  |
| `ecs`                       | the engine — see sub-fan-out                                                                                                                            | —                  |
| `chart`                     | 18 SSR SVG types — see sub-fan-out                                                                                                                      | datastar/templ     |
| `ch`                        | ClickHouse client, `Batcher`, goose-clickhouse migrate                                                                                                  | clickhouse-go      |
| `i18n`                      | catalog, `T(ctx,key)`, extractor                                                                                                                        | —                  |
| `ui-contract`               | the `Kit` interface + sub-interfaces + registry + `Defer` + token wiring to Stellar — **no components yet**, just the contract Blocks 2–3 build against | datastar           |

**`ecs` sub-fan-out** (one track each; b/c/d/e/f start once a's `Store` API is signed):
a. `Store` + entity handles (index+generation) + columnar archetype storage ·
b. component schema + `beach ecs gen` codegen · c. change ticks + `Changed[T]` ·
d. relationships + reverse index · e. CBOR snapshots · f. regression bench
(go-ecs-benchmarks scenarios).

**`chart` sub-fan-out** (one track per family, golden-file tested):
a. cartesian (line/area, sparkline, scatter, bollinger) · b. bar (h/stacked/grouped/
bullet/bar-race) · c. radial+flow (gauge×2, sankey, chord) · d. matrix+dist (heatmap,
calendar heatmap, box, streamgraph) · e. billboard + timeline.

**◆ Gate:** `go test ./...` green across all foundation packages; `ecs` bench prints
numbers; chart golden SVGs open and look right. Testable, not yet viewable.

---

## Block 2 — The spine stands up · first **viewable** page

The root `beach` package, plus just enough kit to render one page.

*Contract-first:* lock the handler shapes (`PageFunc`/`ActionFunc`/`StreamFunc`,
`beach.Ctx`, the error contract) and the App-builder surface. Then fan out.

| Track               | What                                                                                                          |
| ------------------- | ------------------------------------------------------------------------------------------------------------- |
| `beach` App builder | middleware stack, healthz, static cache + boot ETags, merged FS, CSP presets, graceful shutdown               |
| `beach` handlers    | the three shapes + guards + dual-purpose page/fragment branch + the SSE loop runner (over `hub` + `datastar`) |
| `driftwood` minimal | shell + page + button + one fragment, on Stellar tokens — enough to render the demo                           |
| demo page           | one page, one Datastar action, one ticking SSE fragment                                                       |

Deps: Block 1 (`datastar`, `hub`, `config`, `session`, `ui-contract`).

**◆ Gate:** `make up` → browser → styled page, click the action, watch the SSE fragment
update live. First-response measured ≤14KB. The "hello beach" you can **see**.

---

## Block 3 — Full kit + specimen + login · the **gallery**

The widest UI fan-out: one subagent per `Kit` sub-interface, all parallel.

*Contract-first:* confirm the Core method set per sub-interface (from
[16-components.md](architecture/16-components.md)) and the principal/guard API.

| Track                                                                           | What                                                                                                                                                                                 |
| ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| login flow                                                                      | wire `auth` into a real login/logout UI, two roles, `Can` guards, skeleton migrations                                                                                                |
| Layout / Nav / Buttons / Forms / Data / Feedback / Overlays / Media / Messaging | **nine parallel tracks** — each builds its Core components in `driftwood` against the `Kit` interface (Overlays uses native `<dialog>`/`popover`; Media uses Stellar aspect/z-index) |
| specimen                                                                        | `ui.SpecimenPage(kit)` — token sheet, icon grid, every component, all 18 charts; fills in as tracks land                                                                             |
| catalog (marketing + commerce)                                                  | parallel, lower priority; renders in the specimen too                                                                                                                                |

Deps: Block 1 (`auth`, `session`, `chart`, `ui-contract`), Block 2 (`beach`).

**◆ Gate:** log in as two roles; the specimen renders the full Core catalog + all 18
charts + the token sheet, zero hardcoded color, a11y clean. The gallery you can **see**.

---

## Block 4 — Realtime + analytics + codegen

| Track             | What                                                                                                                              | Deps         |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------- | ------------ |
| `sim`             | single-writer tick loop, command channel + `Ask`, projections (dirty→topics+views), the three persistence lanes, snapshot/restore | ecs, hub, pg |
| chart live-wiring | SSE re-layout + Datastar-into-SVG patching + token-driven CSS tweens; **bar race** is the showcase                                | chart, hub   |
| `apigen`          | sqlc process plugin: 6 annotations → `PageFunc`/`ActionFunc` + NOTIFY + `@requires` checks                                        | beach shapes |
| `ch` → chart      | `ch.Rows[T]` feeding `chart.Layout*`; the `Defer`-wrapped dashboard widget                                                        | ch, chart    |

**◆ Gate:** a tiny live sim demo (entity changes push patches; a chart animates over
SSE); `sqlc generate` emits working CRUD routes from annotated SQL.

---

## Block 5 — The three example apps · three independent tracks

Each app is its own track (each can be several subagents). The falsifiable proof.

| Track         | Showcases                                                                  |
| ------------- | -------------------------------------------------------------------------- |
| `boardwalk`   | sim/ecs; all three persistence lanes; kill-and-recover test                |
| `driftbottle` | hub/SSE; matchmaking; anonymous principals; hosts the 5k fan-out benchmark |
| `pantry`      | apigen/auth/ch/chart/i18n; generated CRUD; dashboards; item photos (media) |

**◆ Gate:** all three run; each exercises its slice; the
[coverage matrix](architecture/15-examples.md#coverage-matrix) is satisfied.

---

## Block 6 — Scaffold + guards + gates · make it repeatable

| Track                | What                                                                                                                        |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `cmd/beach` scaffold | `beach new` stamps Makefile, `stellar.config.json`, sqlc.yaml, docker-compose, `.env`, auth migrations, a working page      |
| vet analyzers        | one subagent each: `lint-datastar`, `lint-pgtype`, `lint-uuid`, authz coverage, no-custom-`<script>` (some buildable early) |
| codegen CLIs         | `beach ecs gen`, `beach i18n` extractor                                                                                     |
| perf gates           | 14KB perf-history (JSONL in git) + the 5k SSE fan-out benchmark + Playwright smokes                                         |

**◆ Gate:** `beach new myapp` → `make up` → working app; `beach vet` catches seeded
violations; perf gates run in CI.

---

## Parallelism map

```
Block 1  ████████████  ~12 package tracks + ecs(6) + chart(5)   ← throw subagents here
Block 2  ███           4 tracks, contract-gated
Block 3  ██████████    login + 9 sub-interface tracks + specimen ← and here
Block 4  ████          4 tracks
Block 5  ███           3 apps, fully independent                 ← and here
Block 6  ████          scaffold + analyzers (1 each) + gates
```

The one rule: **sign the contract, then fan out.** Every block's first commit is the
interface its tracks share; collisions happen only when subagents invent the seam
independently.
