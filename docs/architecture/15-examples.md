# Architecture 15 — Example apps: the validation set

[← docs index](../README.md) · prev: [Analytics](14-analytics.md) · next: [Component catalog](16-components.md)

Four example apps under `cmd/examples/`, each shaped exactly like a consuming
app (`beach new` output + features), each chosen to stress a different slice of
the framework. Together they are the falsifiable claim that the framework works —
every package is exercised by at least one of them.

This doc is the **full architecture of each app**: how it boots, where state
lives, how a request becomes an SSE patch, and — just as important — what each
app deliberately does *not* do. All four are built and runnable.

| App | Showcase | State home | Persistence |
| --- | --- | --- | --- |
| boardwalk | `sim`/`ecs` realtime | `ecs.Store` in a sim loop | Postgres snapshot + ClickHouse firehose |
| driftbottle | `hub`/SSE fan-out | in-process maps (hot path) | Postgres transcript archive + ClickHouse firehose |
| pantry | `apigen`/`auth`/analytics | Postgres | Postgres + ClickHouse |
| booking-manager | `mailer`/`sms` add-ons, Postgres-only stack | Postgres | Postgres |

Every app requires the **Beach stack** — Postgres and/or ClickHouse — and **fails
to start** if its configured stack is missing. There is no degraded in-memory
mode: a misconfigured process exits with the list of missing env vars rather than
serving a crippled version. Where a live in-memory model is the *showcase*
(boardwalk's `ecs`/`sim`, driftbottle's matchmaking/feed) it stays in memory on
the hot path, backed by a required Postgres + ClickHouse off the hot path.

A rule every app obeys, learned the hard way: **a rendered page is not a working
feature.** Each is verified in a real browser (chrome-devtools/playwright)
clicking the actual flows, not just compiling and serving a 200.

Two things every app shares: views are `.templ` components (`views.templ`,
compiled by `make gen`, generated output committed), and
each mounts the framework's [specimen](16-components.md#the-specimen) at
`/specimen` — the component/token/chart gallery rides the examples instead of a
dedicated demo binary.

**Layout.** Each app is split by concern (see [01-layout.md](01-layout.md#what-a-consuming-app-looks-like)):
a thin `main.go` + `config.go` at the root, and everything else under app-local
`internal/<concern>` packages — `internal/web` for the app/server struct +
handlers + `.templ` views, plus sibling packages for the decouplable concerns
(`internal/store`, `internal/analytics`, and per app `internal/game`,
`internal/chat`, `internal/notify`,
`internal/mail`, `internal/auth`). The file names called out below (`views.templ`,
`game.go`, `board.go`, …) live inside the relevant `internal/<concern>` package
rather than the app root.

---

## `cmd/examples/boardwalk` — real-estate game (Monopoly-like)

The **`sim`/`ecs` showcase**: a multiplayer, tick-driven board game where the
entire live slice lives in an `ecs.Store` owned by a single-writer `sim` loop and
is fanned to every watcher over SSE. The live model stays in memory — that is the
point, it isolates the sim/hub story — but the app now requires the Beach stack
and **fails to boot without it**: Postgres holds a periodic CBOR snapshot of the
whole store (the [snapshot lane](08-sim.md#persistence), restored on boot so a
restart resumes the game) and ClickHouse holds the event firehose behind `/stats`.

### Boot & wiring (`main.go`)

```go
conf := loadConfig()                 // config.MustLoad: POSTGRES_DSN + CLICKHOUSE_DSN required
pool := pg.MustPool(ctx, conf.DSN); pg.Migrate(ctx, pool, migrationsFS)
conn, _ := ch.Connect(ctx, conf.CHDSN); ch.Migrate(ctx, conn, chMigrations())
events := ch.NewBatcher[event](conn, "boardwalk_events", ch.Batch{})

h := hub.New()
blob, _ := snap.load(ctx)            // restore the saved store if present
s := sim.New(sim.Config{TickRate: 20, ProjectRate: 20, Hub: h, Store: restoredFrom(blob)})
game := NewGame(s, events)
app := beach.New(beach.Config{Service: "boardwalk", Hub: h})
go game.Run(ctx)
go saveLoop(ctx, game, snap)         // periodic snapshot upsert to Postgres
```

| Method | Path | Shape | Role |
| --- | --- | --- | --- |
| GET | `/` | PageFunc | full board page + `data-init` to open the three streams |
| POST | `/join` | ActionFunc | claim a seat, set `bw_seat` cookie, re-open the hand stream |
| POST | `/roll` | ActionFunc | roll command → toast feedback |
| GET | `/board` | StreamFunc | subscribe every connection to topic `board` |
| GET | `/hand` | StreamFunc | subscribe a seated player to `user:<seat>`; spectators to nothing |
| GET | `/race` | StreamFunc | subscribe every connection to the shared cash-race topic |
| GET | `/specimen` | PageFunc | the framework [specimen](16-components.md#the-specimen) |

The whole "session" is one cookie, `bw_seat`, holding a seat index 0–3. No cookie
= spectator (`seatFrom` returns −1). No auth package, no login — the demo is
deliberately public.

### State model — everything is an entity

The live game is components in the `ecs.Store`, never plain Go fields:

```go
type Player    struct { Name, Token string; Spec bool }   // Spec = seat still open
type Position  struct { Square int }
type Cash      struct { Amount int }
type TurnOrder struct { Seat int }
type Ownership struct { Tile int; Owner ecs.Entity }       // zero Owner = bank
type Board     struct { Turn, LastN, LastD1, LastD2 int; Log []string; Joined int; Over bool }
```

At boot `NewGame` creates one singleton `Board` entity, four seat entities
(`Spec: true`), and one `Ownership` entity per buyable tile. The board itself
(`board.go`) is **static data** — a 20-square ring of `tile{Name, Kind, Price,
Rent, Group}` with kinds GO / property / tax / chance / jail (just visiting) /
free parking. Live ownership is the `Ownership` components keyed by tile index;
the ring is read-only.

### Commands & the single-writer boundary

Mutation happens in exactly one place: inside a command's `Apply(w *sim.World)`,
run on the loop goroutine. Handlers never touch the store directly — they `Ask`:

```go
// handler side
res := game.Roll(ctx, seat)        // wraps a rollCmd in sim.Ask, waits for the reply

// loop side, inside rollCmd.Apply(w):
//   validate it's this seat's turn → roll 2× w.Intn(6)+1 → move (from+steps)%20
//   → resolve tile (buy if affordable & unowned / pay rent / tax / chance) → next seat
```

`joinCmd` and `rollCmd` are the only two commands. Dice come from the **sim's
deterministic PRNG** (`w.Intn`), so a game is reproducible from its seed. Reads
that must be consistent (catch-up renders) go through `game.Snapshot(ctx)`, which
uses `sim.AskFunc` to run a read-only closure on the loop.

### Hub topics & projections

Two projections turn store mutations into pushed HTML, and a hub ticker drives
the cash race:

| Producer | Fires on | Topic | Fragment |
| --- | --- | --- | --- |
| Board projection | `Changed[Board]` | `board` (everyone) | `boardFragment` → `#bw-board` |
| Cash projection | `Changed[Cash]` | `user:<seat>` (one player) | `handFragment` → `#bw-hand` |
| Race ticker | every second (`hub.Ticker`) | `race` (everyone) | `raceFragment` → `#bw-race` |

The **"Cash race" card** is the [server-animated chart](14-analytics.md) pattern
live: each second the ticker snapshots the game, `chart.LayoutBarRace` re-ranks
the bars, and the rendered `BarRaceSVG` fragment is published — CSS transitions
on the bar geometry tween between frames, the ticker dedupes unchanged frames,
and no animation code runs on the client.

The shared board (turn banner, 20 tiles with owners/occupants, standings, a
14-line log) fans to all subscribers; your private hand (cash, your tile, your
deeds, the Roll button when it's your turn) goes only to your `user:<seat>` topic.
New posts on page load come from each stream's `CatchUp`, which renders the
current snapshot off-loop before the first projection arrives. Joining patches a
tiny `#bw-reopen` div whose `data-init` re-fires `@get('/hand')`, swapping the
spectator control for a real hand with no page reload (the CSP-safe pattern — no
inline script).

### Rendering

`views.templ` holds every view — the page, `boardFragment`, `handFragment`,
`raceFragment` — as templ components calling `driftwood` directly
(`@driftwood.Card(...)`); `views.go` keeps the small data-shaping helpers so the
templates read as plain markup. One snapshot read feeds all the fragments.

### Deliberately not there

No trade, no auction, no mortgage, no jail rules, no doubles/extra-turn, no
bankruptcy or win condition (`Board.Over` exists but is never set). Buying is
automatic on landing if you can afford it. The point is the framework, not
tournament fidelity. The persistence is deliberately the snapshot lane only — no
per-component write-behind mirror tables — because the demo's job is to show the
lane, not to be the system of record for a game.

**Validates:** `ecs`, `sim`, the command channel + `Ask`/`AskFunc`, projections,
hub topics (shared vs per-user), SSE catch-up and re-open, the server-animated
chart (`hub.Ticker` + bar race), the `sim` snapshot lane (`Store.Save`/restore to
Postgres), and a `ch` event firehose + `chart` dashboard.

---

## `cmd/examples/driftbottle` — stranger chat (Omegle-style, text only)

The **`hub`/SSE showcase**: anonymous, no accounts — the purest possible test of
the hub, matchmaking, and reconnect with nothing else *on the hot path*. The live
slice (the waiting queue, the pairings, the rolling feed) stays in process maps;
that is what makes it the designated fan-out benchmark host — no session
affinity, no per-message N-fan, **no DB on the hot path**. The app still requires
the Beach stack and fails to boot without it: Postgres archives transcripts and
ClickHouse holds the event firehose, both written strictly **off** the hot path
(a non-blocking `ch.Batcher` and a buffered persist channel drained by one
goroutine), so the benchmark claim is preserved. `/stats` charts the firehose.

### Boot & wiring (`main.go`)

```go
type server struct {
    hub      *hub.Hub
    mu       sync.Mutex
    waiting  string                       // the one session parked in the lobby, or ""
    sessions map[string]*sessionState     // every session seen
    feeds    map[string][]chatMessage     // pair topic → rolling message buffer
}
```

| Method | Path | Shape | Role |
| --- | --- | --- | --- |
| GET | `/` | PageFunc | lobby page; mints the `db_sid` cookie on first render |
| GET | `/events` | StreamFunc (`beach.Sub`) | subscribe to exactly `me:<sid>` |
| POST | `/start` | ActionFunc | enter the matchmaking queue |
| POST | `/say` | ActionFunc | send a message to your partner |
| POST | `/next` | ActionFunc | leave this partner and re-queue |
| GET | `/specimen` | PageFunc | the framework [specimen](16-components.md#the-specimen) |

All markup — lobby, waiting, chat, the notices — lives in `views.templ` as templ
components over `driftwood`.

Identity is one cookie, `db_sid`, a random 128-bit hex id. There is no account,
no `session.Store`, no login — `sid()` reads the cookie or mints a new id. This is
the `auth` story's `anonymous` case doing real work.

### Matchmaking

Two topic shapes: `me:<sid>` (per-session, the only thing a connection subscribes
to) and `pairTopic(a,b)` = `pair:` + the two ids sorted, used only to key the
rolling feed buffer.

```
enqueue(id):
  already paired/waiting → re-render current state (idempotent)
  s.waiting == ""        → park id in s.waiting, render "waiting"
  else if waiting != id and neither is the other's `last`:
      pair them, clear waiting, set mutual partner, drop the old feed buffer,
      publish chatFragment to both me:<a> and me:<b>
```

`teardown(id)` clears both `partner` fields, sets both `last` (so an immediate
Next can't rematch the same pair), drops the feed buffer, and publishes
`endedFragment` to the ex-partner — it does **not** re-queue. `/next` is just
`teardown(id)` then `enqueue(id)`.

### Hub/SSE story

One connection per session, one topic (`me:<sid>`). A message fans to exactly two
topics, never N:

```
broadcast(sender, partner, body):
  append to feeds[pairTopic] (capped at feedCap = 100)
  publish forViewer(snapshot, sender)  → me:<sender>   // sender sees "me"
  publish forViewer(snapshot, partner) → me:<partner>  // partner sees "them"
```

`forViewer` rewrites each message's `From` to "me"/"them" per viewer; fragments
are rendered to bytes **once** per publish and fanned as bytes. Reconnect is
handled by the stream's `CatchUp`, which calls `currentRoom(id)` and renders
whichever state — lobby / waiting / active chat — the session is in right now.
This single-topic, two-publish, pre-rendered-bytes shape is what makes it the
fan-out benchmark host: no session affinity, no per-message N-fan, no DB on the
hot path.

### Moderation (actually implemented)

- **Rate limit:** a sliding window per session, `msgRateBurst = 8` per
  `msgRateWindow = 10s` (`rateOK`); over the limit publishes a notice to `#notice`
  and clears the composer.
- **Word filter:** `clean()` → `filterWords()` masks a hardcoded blocklist
  (case-insensitive substring) with asterisks, and truncates at `maxMsgLen =
  1000`.

### Chat action & Datastar signals

The composer binds its textarea to the `msg` signal, so `/say` reads it with
`beach.Bind[struct{ Msg string }]` (JSON signal, **not** form value). On success
the handler returns a `Signals` patch `{"msg": ""}` to clear the input — no DOM
surgery, just a signal reset.

### Tests (`driftbottle_test.go`)

Covers the load-bearing behavior directly against the `server`: pairing two
strangers, message fan-out with correct me/them framing, next-tears-down-and-
re-queues, the rate limiter (exactly 8 succeed), the word filter, and
catch-up defaulting an unknown session to the lobby.

### Deliberately not there

No reading of the transcript archive back into the app (Postgres is a write-only
sink here — the archive is for offline/admin use, never the hot path), no
presence list beyond the pairing, no group chat, and the 5k-connection benchmark
harness itself is not checked into the tree yet — the app is *built for* it (the
architecture above is the proof), but the runnable artifact is the chat plus its
unit tests.

**Validates:** hub fan-out, `StreamFunc`/`beach.Sub`, catch-up/reconnect,
matchmaking, anonymous identity, the moderation hooks, signal-clearing patches,
off-hot-path write-behind to Postgres, and a `ch` firehose + `chart` `/stats`.

---

## `cmd/examples/pantry` — at-home grocery ERP

The **CRUD/`apigen`/`auth`/analytics showcase**: boring in the good way. It
requires the full Beach stack — Postgres (the system of record: inventory, auth,
sessions) and ClickHouse (the dashboard's activity firehose) — and **fails to
boot without both**; there is no in-memory mode.

### Boot & wiring (`main.go`)

```go
type app struct {
    store    *pgStore            // Postgres-backed; the only store
    cat      *i18n.Catalog
    authn    *auth.Authenticator
    sessions *session.Store
    pool     *pgxpool.Pool
    chConn   ch.Conn             // the activity firehose
    events   *ch.Batcher[event]
    release  bool
}
```

Boot: `config.MustLoad` (Postgres + ClickHouse DSNs required — exits with the
missing list otherwise) → `i18n.Load(localesFS, "en-US")` → `openPool` then
`migrate` (three sources: `session.Migrations`, `auth.Migrations`, the app's
`migrationsFS` — each gets its own goose version table, see
[02-boot-spine.md](02-boot-spine.md)) → `newPGStore` (idempotently seeds the
sample inventory on a fresh DB) → build the `session.Store` + `auth.Authenticator`
and `seedAdmin` (`admin` / a password, role `household-admin` with
`pantry:read|write|admin`) → `ch.Connect` + `ch.Migrate` + the events batcher.
Any failure is fatal. The finished `beach.Handler()` is wrapped in
`cat.Middleware` so every request resolves a locale into context.

| Method | Path | Shape | Guard |
| --- | --- | --- | --- |
| GET | `/`, `/items`, `/locations`, `/lists`, `/login`, `/specimen` | PageFunc | — |
| GET | `/widgets/{spend,category,expiry,waste}` | PageFunc | — (deferred) |
| POST | `/items`, `/locations` | ActionFunc | `app.Can("pantry:write")` when auth is on |
| POST | `/login` | Raw | — (plain form → 303) |
| GET | `/logout` | Raw | — |

### apigen: the annotation surface vs the running handlers

The SQL files under `internal/db/sql/queries/` carry the **full apigen spec** —
the six annotations that would generate the whole CRUD surface:

```sql
-- name: CreateItem :one
-- @api POST /items
-- @requires pantry:write
-- @notify items
-- @fragment page.ItemCard
INSERT INTO pantry_items (...) VALUES (...) RETURNING ...;
```

`@api METHOD /path` (shape from the verb), `@page`/`@fragment` (full page vs
patched partial), `@requires perm` (guard), `@notify resource` (NOTIFY topic).
Items, locations, and lists each carry the full list/get/create/update/delete/
toggle set. In the *running* binary the write paths are **hand-written
equivalents** in `internal/web` (its own comment says so) — they are the
falsifiable reference for what apigen emits: bind the signals, run the write,
patch the `@fragment`. The ratio of generated-to-hand-written is apigen's measure;
pantry keeps both visible side by side.

### Data model

`internal/store` defines `Item`, `Location`, `ShoppingList`, `ListItem` and the
`Store` (its `store.New` boot-seed inserts 6 items, 3 locations, 1 list when
`pantry_items` is empty — the idempotent analogue of the old in-memory seed). The
migration creates soft-deleting (`deleted_at`) tables `pantry_items`,
`pantry_locations`, `pantry_lists`, `pantry_list_items` plus the framework's
auth/session tables, and seeds the `household-admin` / `household-member` roles.

### Request lifecycle (create item)

1. The add-item form binds each field to a same-named Datastar signal.
2. `POST /items` → `createItem` reads them with `beach.Bind[…]` — quantity is a
   `float64` because a number input posts a JSON number; the rest are strings.
3. `a.store.addItem(...)`, then return
   `beach.Patches{{Fragment: a.itemGrid(...), Target: "item-grid"}}` — the grid
   re-renders and morphs in place.

Login is the deliberate exception: a **plain HTML form POST** that 303-redirects,
not a Datastar action. The session cookie must be set on the response *and* the
browser must navigate — and the strict CSP forbids the SDK's script-based
redirect — so a real form post is the right tool ([12-auth.md](12-auth.md)).

### Rendering

Markup lives in `views.templ` — `shellView` (topbar + 4-section sidebar,
login/logout reflecting `c.User()`) and per-page views built from direct
`driftwood` calls: `PageHeading`, `Card`, `Grid`, `Image` (responsive item
photos at a fixed `RatioPhoto`, no layout shift), the form components,
`StackedList`, `Alert`. `pages.go` keeps the handlers that shape data and pick
the component. Write forms only render when `principalCan(c, "pantry:write")`,
so the in-memory single-user mode and the authed multi-role mode share one
template.

### Analytics — deferred dashboard

`dashboardView` lays out four `.dash-widget` cards (the
[chart-toolbar contract](14-analytics.md#the-client-interaction-layer) — each
gets live Grid/Legend/Theme/Expand controls), each body a
`ui.Defer{ID, Get: "/widgets/<id>"}` placeholder with reserved height — zero
layout shift. Datastar fetches each widget on intersection and morphs it in by
id; the widget routes (a per-request query, not a cached fragment) return
`chart.Chart*Fragment`s carrying the same id. The data is
real, split honestly: the **activity** line (`ChartLineFragment`) reads the
ClickHouse firehose (items added per day, `WITH FILL`); the inventory-derived
widgets read Postgres, the system of record — **by category**
(`ChartStackedBarFragment`, one bar per location-kind), **expiry**
(`ChartCalendarFragment`, per-day `expires_at` counts), **spoilage**
(`ChartGaugeFragment`, expired ÷ total). CH is the firehose, Postgres is the
record ([14-analytics.md](14-analytics.md)).

### i18n

`a.t(ctx, key, args…)` resolves against the catalog using the locale the
middleware put in context. `catalog.json` is the key dictionary (with
descriptions); `locales/` carries `en-US` and `es-ES`. pantry is the most
text-heavy app, so it carries the translation story.

### Deliberately not there

The apigen *generator* output is not wired into the running routes (the
hand-written equivalents stand in as the reference).

**Validates:** apigen annotation surface, `auth`/RBAC + the login flow, `session`,
`pg.Migrate` multi-source, a real `ch` firehose feeding `chart` fragments + the
widget toolbar, `driftwood` (incl. media components), deferred sections, and
`i18n`.

---

## `cmd/examples/booking-manager` — seasonal rental operator

The **add-on showcase** (`mailer` + the new `sms`) and the proof of the
**Postgres-only stack**: a self-hostable manager for a handful of seasonal
rentals — cottages, cabins, getaways — for operators who don't want a monthly
SaaS fee and can run one binary plus one database themselves. No ClickHouse,
deliberately: the "and/or" in the stack rule is real, and this is the app that
proves the smaller half.

### Boot & wiring (`main.go`)

```go
conf := loadConfig()                       // config.MustLoad: DATABASE_URL required, mail/SMS optional
pool := pg.Pool(ctx, conf.DSN); migrate(…) // session + auth + booking schema, three goose sources
st, _ := store.New(ctx, pool)              // idempotent seed: 3 properties, staff, bookings
sessions := session.NewStore(…); authn := auth.NewAuthenticator(…); seedAdmin(…)

mlr    := mailer.New(mailer.Config{…})     // Mailgun > SMTP > LogMailer
txts   := sms.New(sms.Config{…})           // Twilio > LogSender
guests := notify.New(mlr, txts, logger)    // best-effort guest messaging
lock   := &locks.LogProvider{}             // the smart-lock boundary, log impl

a := web.New(st, authn, sessions, pool, guests, lock, release)
```

Both guest-messaging transports and the lock are **interfaces picked by
config**: a fresh checkout "delivers" email, texts, and door-code programming
to the terminal; production sets `MAILGUN_*`/`SMTP_*` and `TWILIO_*` and
nothing in the app changes. `pkg/sms` is deliberately the same shape as
`pkg/mailer` — one `Sender` interface, a `Config`, `New` picking the strongest
transport, a log fallback — because `mailer` *is* the add-on convention.

| Surface | Routes | Guard |
| --- | --- | --- |
| Public landing + intake | GET `/`, POST `/inquire` | — (guests use these) |
| Dashboard / inquiries / bookings / properties / inventory pages | GET | `Can("bookings:read")` |
| Property + key-code writes | POST | `Can("properties:write")` |
| Pipeline + booking writes | POST | `Can("bookings:write")` |
| Hiring / staff / shifts | GET + POST | `Can("staffing:read")` / `staffing:write` |
| Supply writes + counts | POST | `Can("inventory:write")` |
| Login/logout | Raw POST/GET | plain form → 303 |

Two roles: `owner` (everything) and `staff` (see bookings and the schedule,
punch the clock, keep counts honest). Seeded login `admin` / `password`.

### The confirmation flow (the app's spine)

A guest submits the public inquiry form → the inquiry works a pipeline
(new → quoted → won/lost) → "Book it" converts it into a pending booking after
an overlap check (`check_in < other.check_out AND check_out > other.check_in`,
cancelled stays ignored) → **Confirm** re-checks the overlap, mints a random
per-stay door code, programs it through `locks.Provider.SetCode`, stores it on
the booking, and fans the confirmation out — email via `mailer`, text via
`sms` — with the code included. Cancelling clears the code off the lock.
Refused writes (date collisions) patch a reason into the page's note slot;
lifecycle changes are real navigations (a `Redirect` patch), not in-page state.

### Around the stays

- **Bookings calendar** — a month grid built server-side (`buildCalendar`,
  unit-tested), stays chipped onto their nights colored by status; month and
  property filter ride the query string as plain links.
- **Staffing** — applicants work applied → interview → offer → hired (hire
  creates the staff row); shifts schedule against properties; the time clock
  is one toggle (`ClockToggle` closes the open entry or opens one) and the
  week's hours are summed in SQL.
- **Inventory** — per-property supplies with par levels; at-or-under par is
  "low" and feeds the dashboard's restock list; −1/+1 adjusters patch the
  list in place (the one control staff hammer during a turnover).
- **Smart locks** — `internal/locks` is the integration boundary: a
  three-method `Provider` (SetCode/ClearCode/Status). No hardware
  implementation exists yet; the `LogProvider` narrates what a real one would
  do, and the property page renders the lock's `Status` as online/battery.

### Deliberately not there

No payments, no channel-sync (Airbnb/VRBO iCal), no guest accounts, no
mail/SMS outbox (sends are best-effort and logged), no ClickHouse analytics.
One operator, a handful of properties —
that scale is the point.

**Validates:** `mailer` + `sms` (the add-on convention, exercised from an
example for the first time), the Postgres-only stack, `auth`/RBAC with two
real roles, public + guarded routes in one app, query-param filters, the
note-slot refusal pattern, and Redirect patches as real navigation.

---

## Coverage matrix

| Package                       | boardwalk |  driftbottle  | pantry | booking-manager |
| ----------------------------- | :-------: | :-----------: | :----: | :-------------: |
| beach (root) / datastar / ui  |     ●     |       ●       |   ●    |        ●        |
| hub                           |     ●     |    **●●**     |   ○    |        —        |
| ecs / sim                     |  **●●**   |       ○       |   —    |        —        |
| auth / passwords              |     —     | ● (anonymous) | **●●** |        ●        |
| apigen                        |     —     |       —       | **●●** |        —        |
| ch / chart                    |     ●     |       ●       | **●●** |        —        |
| i18n                          |     —     |       —       | **●●** |        —        |
| mailer / sms                  |     —     |       —       |   —    |     **●●**      |
| storage                       |     —     |       —       |   —    |        —        |
| cache / session / pg / config |     ●     |       ●       |   ●    |        ●        |

●● = the showcase, ● = exercised, ○ = incidental, — = unused (deliberately: not
every app needs every package, and the examples should prove that too).
