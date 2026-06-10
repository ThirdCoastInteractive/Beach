# Architecture 4 — Realtime fan-out: `hub`

[← docs index](../README.md) · prev: [HTTP layer](03-http.md) · next: [Cache & session](05-services.md)

Unifies manifold's two SSE models (live's event hub, admin's `sseTickLoop`) under one
concept: **topics**.

```go
type Hub struct{ ... }                                // generalization of notify.Registry

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

- Per-subscription buffered channel (64), **drop on full** — slow clients reconcile via
  a `?since=` catch-up cursor on reconnect, exactly manifold's notification pattern
  (`cmd/live/handler/handler.go:670-701`).
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
- `hub.Ticker(topic, interval, produce)` replaces `sseTickLoop`: a producer that
  publishes on an interval; admin dashboards subscribe like everything else.
- Producers: HTTP handlers (after a mutation), `pgnotify` listeners (external writers,
  other processes), and [`sim` projections](08-sim.md). In-process mutations
  publish directly — they never round-trip through Postgres NOTIFY.
