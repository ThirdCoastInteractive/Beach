# Architecture 20 — The sync boundary: hypermedia is not a sync engine

[← docs index](../README.md) · prev: [rybitten](19-rybitten.md)

"Sync engines" — Zero, ElectricSQL, PowerSync, Convex, Jazz — are the loud answer to
"keep the screen fresh without spinners." A recurring question is whether Beach
should adopt or grow one. It should not, and the reason is a category boundary, not a
maturity gap. This doc states the boundary, what it buys us for free, the one problem it
leaves on our plate, and where it ends.

The framing here distills Siidorow's *The Limits of Generalized Sync* (Aalto, 2026), a
taxonomy of 14 sync engines across 13 projects. Its central result maps cleanly onto our
stack, and mostly tells us we already made the right call.

## Two answers to the same question

A **sync engine** keeps *client-side application state* in sync: the client holds a local
replica of the data — rows, documents, objects — and renders it itself. A **hypermedia**
platform keeps the *rendered view* in sync: the client holds HTML and morphs it; the
server is the only thing that ever knows the schema.

These are not points on a spectrum — they are mutually exclusive at the core. The moment
the client holds queryable data, the "H" is gone (the client now owns application state),
and Datastar signals + DOM morphing are the wrong renderer for it. Bolting a sync engine
onto Beach does not extend it; it converts it into a worse SPA. That is the category
error.

The taxonomy's most differentiating axis after offline capability is the **sync unit**:
rows, documents, operations, events, pages. Beach's sync unit is **the rendered
fragment** — state transfer taken to its limit, where the transferred state is already
projected to its final UI form. That single choice is why two of the taxonomy's hardest
problems never reach us: **partial replication** (the server renders only what it chooses
to send) and **schema evolution across client versions** (the client never sees a schema
to drift from).

## We already sit in the generalizable quadrant

The thesis crosses the read/write path against the online/offline boundary. Three of the
four cells generalize across applications; one resists:

| | Always-online | Offline-capable |
| --- | --- | --- |
| **Read path** | Generalizes (push deltas) | Generalizes (local replica) |
| **Write path** | **Generalizes** when the engine owns the mutation | **Resists** — auth, conflict, business logic vary by domain |

Beach has independently arrived at every choice the thesis marks "safe":

- **Server-authoritative** — the system of record is Postgres.
- **Always-online** — SSE needs a live connection; offline is a non-goal (see below).
- **Server owns the write path** — every mutation is a server transaction (`pg.InTx`,
  the sim's [sync-tx lane](08-sim.md)). There is no optimistic client write to reconcile,
  ever.
- **Read path = server pushes deltas** — an SSE patch *is* a delta. The [hub](04-hub.md)
  (topic → pre-rendered fragment → fan-out, `?since=` catch-up, drop-on-full) is a clean
  implementation of exactly the read path the thesis says commoditizes — we push HTML
  deltas instead of row deltas.

So the difference from Zero/Electric/PowerSync is one dimension: their sync unit is rows,
ours is rendered HTML. We are in their sweet spot ("real-time B2B productivity tools are
the broadest fit") on the safe side of every boundary.

## What that wins us — do not re-solve

- **No write-path generalization problem.** The thesis's single hardest limit — offline
  writes reconciled against revoked permissions, plus per-domain conflict and business
  logic — *cannot occur* in a server-serialized architecture. Keep mutations server-side;
  it is the feature.
- **Authorization stays server-side at render time.** The thesis's #1 hard limit is that
  all three sync-engine vendors pivoted to server-enforced auth. We start there. The rule
  follows directly: never *render* a fragment the current viewer is not entitled to (the
  one sharp edge this leaves is fan-out — see [the topic rule](#the-rule-topic-scope--authorization-scope)).
- **No schema on the client, no partial-replication layer.** Both fall out of the HTML
  sync unit.

## Conflict resolution: last-write-wins at the server, design conflicts out

The thesis found conflicts rare under server authority and last-write-wins dominant; the
recurring practitioner advice is "know the model explicitly and design conflicts out"
rather than build merge machinery. Zero users reported *zero* production conflicts.

Beach is server-serialized by construction, so the honest stance is:

- **Last-write-wins at the database is the default**, and that is fine. Money, items, and
  trades commit in a synchronous transaction first (the [sim is a cache of the committed
  result](08-sim.md)); the rest is LWW on `updated_at`.
- **Reach for single-owner or append-only data models** before any merge logic — they
  remove the conflict instead of resolving it.
- **Genuine multi-writer-same-field editing** (collaborative rich text) is the one case
  that justifies a CRDT, and it belongs in a **scoped island** (a Yjs-style widget), never
  the framework's sync path. Linear shipped years of product on LWW before adding CRDTs
  only for collaborative text; we hold the same line.

## The one problem we own: read-path liveness

Choosing hypermedia dissolves the write-path problems. It inherits exactly one: **read
freshness**. The thesis's most underrated finding is that staleness shows up as *unrelated
bugs* — stale data, stale permissions, a change by user A invisible to user B until reload
— in any server-authoritative app that fetches per-component and treats responses as
snapshots.

[pantry](15-examples.md) is the worked example: a list page that is
`useSWRImmutable`-equivalent, so an admin restocks an item and another operator
files a *support ticket* instead of reloading. That is a latent sync bug, not a
missing feature.

Two rules follow.

### Treat staleness per surface, not globally

Do **not** force everything through a stream. Most surfaces are fine pull-on-navigation —
a grocery list being a few seconds stale costs nothing. The criterion for adding liveness
is one question: *does a stale view here cause a support ticket or a wrong action?* Catalog
freshness → marginal, leave it pull-based. Admin permission/flag propagation → high value,
make it live. Adding a stream where staleness is harmless is the same mistake as shipping
a sync engine you don't need.

### The CRUD-liveness recipe

For sim-backed surfaces this is already solved: [projections](08-sim.md) re-render once per
topic on any change. For plain CRUD surfaces the pieces exist but the glue is hand-rolled:

1. The mutation rings the bell. apigen's [`@notify`](13-apigen.md) publishes `{table,id,op}`
   to Postgres `NOTIFY` and a hub topic; an in-process handler can `hub.Publish` directly.
2. **Re-render and fan out.** A `pg.Listen` listener (or the in-process handler) re-queries
   the changed row, renders its fragment to bytes, and publishes to the SSE topic:

   ```go
   ch, _ := pg.Listen(ctx, pool, "items")
   for payload := range ch { // payload = {table,id,op}
       id := parseID(payload)
       row, _ := q.GetItem(ctx, id)
       hub.Publish("items", hub.Event{
           Bytes:  render(page.ItemCard(row)),
           Mode:   hub.PatchMorph, // by the card's own id
       })
   }
   ```

3. The page subscribes. A [`StreamFunc`](03-http.md#beachstreamfunc--sse-subscriptions)
   returns `Sub{Topics: []string{"items"}, CatchUp: ...}`; the framework runs the loop and
   replays `?since=` on connect for completeness.

This is the read path the thesis says generalizes, in our idiom. The missing convenience —
the `id → re-render → publish` step as a first-class helper instead of a hand-written
listener — is the keystone build below.

### The rule: topic scope == authorization scope

"Render once, fan to a topic" has one sharp edge: a fragment pushed to a topic is seen by
*every* subscriber, and the render cannot be per-viewer. So **a topic's audience must equal
the fragment's authorization scope.**

- Public or workspace-shared fragments → a shared topic (`items`, `board`). Fine.
- Anything viewer-dependent → a per-principal topic, rendered per principal
  (`self:<sid>` is the pattern). Private state never rides a shared topic.

This is our (much smaller) version of the partial-replication problem: we choose topic
granularity instead of maintaining per-client materialized views, but the discipline is
mandatory — it is the one place server-side authorization can leak.

## The WebSocket lane (RFC 05)

The sync unit for UI remains the rendered fragment over SSE — that boundary is
unchanged. Some apps additionally need a high-rate, bidirectional, **binary** channel
that is not UI sync at all: 60 Hz simulation state, controller input upstream, binary
telemetry. That is [`App.Socket`](03-http.md#beachsocketfunc--websockets-not-hypermedia),
and the doctrine is one line: **SSE carries hypermedia; WebSocket carries payloads
that are not hypermedia.** A socket never patches the DOM through the framework — no
hub↔socket bridge, no Datastar over WS. If a socket consumer wants UI updates, the
page keeps a normal `Stream` alongside. The one sync idea the socket does inherit is
latest-state-wins: `WriteLatest` coalesces so a slow client skips frames instead of
building a queue — the same drop-on-full stance the hub takes.

## The offline exit condition (non-goal)

SSE, the hub, and Datastar all require a live connection. **Offline writes are the explicit
exit from hypermedia.** If an app genuinely needs to accept writes with no network, it is
not a Beach app — it needs a different architecture (or a non-hypermedia island with
its own RFC). We do not make Beach offline-capable; doing so buys the entire Chapter 5
of the thesis (write-path reconciliation, dual client/server schemas, client-generated IDs,
browser-storage immaturity) for a requirement our target apps do not have.

The evidence that this line is in the right place: even the local-first vendors retreated
to server authority, and the *only* thing that forced full offline in the study was
field apps with no connectivity. If you are not building a no-signal field tool, you are in
the documented sweet spot. The thesis calls the online-only middle ground "Sync Light"
(read-path reactivity + server-authoritative writes) and predicts it fits most B2B web
apps — hypermedia is Sync Light taken to its end, with HTML as the delta.

## Proposed helpers (not yet built)

The primitives above are complete; these are narrow combinators over them, ranked by value.
None is infrastructure — each traps already-won complexity behind a smaller interface.

1. **A CRUD live-projection helper** (keystone) — the sim's projection ergonomics for
   non-sim surfaces: bind a `NOTIFY` channel / hub topic to a `func(id) (templ.Component,
   topic, mode)` and let the framework run the `listen → coalesce by id → re-render →
   publish` loop. Turns the recipe above into a few lines and gives the topic/auth question
   one obvious home.
2. **A live-page combinator** — derive a surface's stream catch-up from the same fragment
   function its `PageFunc` already renders, so "make this surface live" stops meaning "keep
   a `PageFunc` and a `StreamFunc` render in sync by hand."
3. **A catch-up cursor utility** — a standard monotonic `?since=` cursor (event id or
   `updated_at` watermark) + a "render everything since X" helper, so every live surface
   gets read-your-writes / monotonic-reads completeness without reinventing it.
4. **`topic == auth scope` lint + per-principal topic helper** — a `beach-vet` analyzer
   that flags a shared-topic render interpolating request/user data, plus a blessed
   per-principal topic constructor so "private fragment" has an obvious right way.
5. **Broadcast-except-sender** — `hub.PublishExcept(topic, ev, subID)` so the actor, whose
   own action already patched their DOM, does not get a duplicate insert when the same
   change fans out (matters for append/prepend feeds, not morph).
