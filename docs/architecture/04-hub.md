# Architecture 4 — Realtime fan-out: `hub`

[← docs index](../README.md) · prev: [HTTP layer](03-http.md) · next: [Cache & session](05-services.md)

Every realtime surface — event-driven chat, periodic dashboard refresh, per-user
notifications — runs on one concept: **topics**. There is no second mental model.

```go
type Hub struct{ ... }

sub := hub.Subscribe("chat:room1", "presence", "user:"+id.String())
defer sub.Close()
for !sse.IsClosed() {
    select {
    case <-ctx.Done():     return nil
    case ev := <-sub.C:    ev.Render(sse)             // pre-rendered patch, written per conn
    }
}
```

(Apps don't write this loop — `beach.StreamFunc` in
[03-http.md](03-http.md#beachstreamfunc--sse-subscriptions) declares the subscription
and the framework runs it. The loop is shown for what the framework does inside.)

- Per-subscription buffered channel (64), **drop on full** — slow clients reconcile
  via a `?since=` catch-up cursor on reconnect. Liveness beats completeness on the
  push path; the catch-up query is the completeness path.
- **Render once per topic, write per connection.** A publisher renders the templ
  fragment to bytes once; the hub fans the bytes out. Per-user personalization
  (your own XP bar) publishes to the `user:<id>` topic instead. This is interest
  management: topic = area of interest.
- **Compression is per connection, not per topic.** Fragments render to plaintext
  bytes once, but each connection owns a gzip stream
  ([SSE compression](03-http.md#sse-compression)) whose window spans its whole
  stream history — so the fan-out write feeds the shared bytes through each
  subscriber's compressor. That per-connection CPU/memory is the price of stream
  compression; the level knob and the driftbottle benchmark keep it inside the
  5k-connection budget.
- `hub.Ticker(topic, interval, produce)` covers periodic refresh: a producer that
  publishes on an interval; admin dashboards subscribe like everything else.
- Producers: HTTP handlers (after a mutation), `pg.Listen` listeners (external
  writers, other processes), and [`sim` projections](08-sim.md). In-process
  mutations publish directly — they never round-trip through Postgres NOTIFY.

## Patch mode: incremental feeds

By default an `Event` morphs its fragment into the DOM by the fragment's own id —
re-render the element, fan out the bytes, Datastar reconciles. That's right for a
dashboard tile or an XP bar. It's wrong for a high-volume rolling feed (driftbottle
chat): re-rendering the whole history to add one line, then morphing the entire
container on every message, scales with history length, not with the one line that
changed.

`Event` carries an optional patch mode for that case:

```go
type Event struct {
    Bytes  []byte
    Mode   PatchMode // PatchMorph (default) | PatchAppend | PatchPrepend
    Target string    // container id to insert into, no leading '#'
}
```

- **`PatchMorph`** — the zero value and the default. Outer-morph by the fragment's
  own id; `Target` is ignored. Existing publishers set nothing and are unchanged.
- **`PatchAppend`** — insert the fragment as `Target`'s last child (new line at the
  bottom).
- **`PatchPrepend`** — insert the fragment as `Target`'s first child (new line at
  the top).

`Target` is the container id the fragment inserts into. It is **required** for
append/prepend — the insert needs somewhere to land — and ignored for the default
morph, which targets the fragment's own id.

So an incremental feed renders and publishes only the single new line, with
`Mode: PatchAppend` and the container id in `Target`. The hub still renders once and
fans the shared bytes out per connection; only the merge instruction changes.

**Stream loop mapping.** `Event.DatastarMode()` reports the
[Datastar](03-http.md#datastar) element-patch mode string for the event's `Mode`:
`""` for the default morph (add no patch option, take Datastar's outer-morph
default), or `"append"` / `"prepend"` for an insert. The SSE loop in
[`beach`](03-http.md#beachstreamfunc--sse-subscriptions) reads `ev.Target` and
`ev.DatastarMode()` and threads them through the same `patchOptions` path every
patch uses — a `#`-prefixed selector for the target plus the element-patch mode —
so a feed event lands in its container as one inserted line. The zero value adds no
options, which is exactly today's outer-morph-by-id behavior; backward compatibility
is the absence of a flag, not a code branch.
