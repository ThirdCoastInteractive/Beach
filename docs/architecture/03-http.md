# Architecture 3 — HTTP layer: the root `beach` package + `datastar`

[← docs index](../README.md) · prev: [Boot spine](02-boot-spine.md) · next: [Hub](04-hub.md)

The HTTP layer is the module root (`package beach`) per the
[layout](01-layout.md#module-layout) — the framework's front door, not a sub-package.

## App builder

Without a builder, every service's `main.go` becomes the same 150 lines of
middleware and bootstrap with slightly different CSP strings. The builder
collapses them:

```go
app := beach.New(beach.Config{
    Service:  "live",
    Release:  cfg.IsRelease(),
    CSP:      beach.CSPDefault().AllowMedia("https://customer-x.cloudflarestream.com"),
    Static:   localStatic,            // overlaid on the framework's embedded assets
    Sessions: sessions,               // nil = database-less, every principal anonymous
    Healthz:  func(ctx context.Context) error { return pool.Ping(ctx) },
})

// Routes register against typed handler shapes (below). Guards make the auth
// story visible in the route table, not buried inside handlers.
app.Page("/",                      handler.Live(pool, s))                       // public
app.Page("/wallet",                handler.Wallet(pool),           app.Authed())
app.Action("/coins/purchase/{id}", handler.PurchaseCoins(pool, s), app.Authed())
app.Stream("/stream",              handler.LiveStream(hubRef, pool), app.Authed())
app.Page("/admin/orders",          handler.Orders(pool),           app.Role("admin"))
app.Socket("/ws/state",            handler.StateSocket(s),         app.Authed())
app.Raw(http.MethodPost, "/webhooks/stripe", billing.Webhook())  // escape hatch: plain net/http

app.Start(":" + cfg.Port)                                // graceful shutdown inside
```

Stdlib `net/http` is underneath (`app.Raw` and `app.Mux()` exist for webhooks and the
genuinely weird), but the paved road is four typed handler shapes that encode the
house rules by construction.

Middleware stack, in fixed order: request logger (skips /healthz, /static),
recover, secure headers + CSP, optional session auth (runs when `Config.Sessions`
is set), visitor preferences (one cookie read). Gzip is not middleware — SSE
responses compress inside the SSE writer (below).

The framework owns two routes of its own, both underscore-prefixed where they
could collide with an app's: `GET /healthz`, and `POST /_beach/prefs`, which
writes the visitor's timing preferences and 303s back to a same-origin `Referer`.
That second one is how WCAG 2.2.2 is answered framework-wide rather than app by
app — `driftwood.LiveToggle` posts to it, and `adaptStream` drops a paused
visitor's topics whether or not the app ever renders a control. See
[RFC 06](../rfc/06-accessibility.md).

`Config.Middleware []func(http.Handler) http.Handler` injects app-supplied
`net/http` wrappers *around* that fixed stack — outermost-first, so the first entry
sees the request first and the response last, ahead of even request logging. It is
the seam for cross-cutting concerns the framework does not own (an i18n locale
resolver, a custom request id); `App.Handler()` applies it. Left nil, the handler
is exactly the fixed stack.

The static handler computes SHA256 ETags for every file at boot and serves with
`immutable` Cache-Control and 304s; `Config.Static` is overlaid on the framework's
embedded tree (`app.css`, the Datastar client, the chart modules) so one `/static`
prefix serves both. Asset URLs carry a `?v=` version: released binaries get stable
commit-based versioning, dev rebuilds get boot timestamps, and every deploy busts
caches without a manifest.

## Handlers: four typed shapes

The load-bearing rules — dual-purpose handlers branch on `IsDatastar`,
`db.New(pool)` per request, never duplicate page + fragment routes, SSE loops follow
the subscribe/catch-up shape — are the handler types themselves, not documentation:
holding one wrong is a compile error or a shape you cannot express, never a review
comment.

Handlers stay standalone factory functions taking only the deps they need — never
methods on a server struct. But they return beach types instead of raw
`http.HandlerFunc`, and take a `*beach.Ctx` carrying the request scope:
`c.User()` (set by guards), `c.InTx(fn)`, `beach.Bind[T](c)` for validated input
binding. Views are `.templ` components ([06-ui.md](06-ui.md)) the handler calls
directly — there is no kit lookup.

### `beach.PageFunc` — anything you can navigate to

The handler returns a view description; the framework does the dual-purpose branch.
You cannot forget it, and separate fragment routes have nothing to attach to:

```go
func Wallet(pool *pgxpool.Pool) beach.PageFunc {
    return func(c *beach.Ctx) (beach.View, error) {
        data, err := loadWallet(c, db.New(pool))
        if err != nil { return beach.View{}, err }
        return beach.View{
            Page:     walletPage(data),     // full document on navigation (.templ)
            Fragment: walletPanel(data),    // patched on Datastar refresh (.templ)
            Target:   "wallet-panel",
        }, nil
    }
}
```

Tabs, filters, pagination, sort reuse the page URL with query params — they never
get their own routes. The rule is structural: a `View` always knows how to render
both ways, so there is nothing for a separate fragment route to do.

`View.Page` is subject to [the 14KB rule](06-ui.md#the-14kb-rule): the compressed
first response targets the initial TCP congestion window, and anything heavier
ships as a [deferred section](06-ui.md#deferred-sections) whose `@get`
lands back on a `PageFunc` fragment.

### `beach.ActionFunc` — Datastar-only mutations

Navigation gets a 404; the `Datastar-Request` check and method discipline are
the framework's problem. The handler does the mutation and says what to patch:

```go
func PurchaseCoins(pool *pgxpool.Pool, s *sim.Sim) beach.ActionFunc {
    return func(c *beach.Ctx) (beach.Patches, error) {
        req, err := beach.Bind[PurchaseReq](c)        // bind + validate, typed
        if err != nil { return nil, err }
        var bal db.Balance
        err = c.InTx(pool, func(q *db.Queries) error { // money: commit first — see
            bal, err = purchase(c, q, req)             // 08-sim.md "Persistence"
            return err
        })
        if err != nil { return nil, err }
        s.Send(cmd.ApplyCommitted{User: c.User().ID})
        return beach.Patches{{Fragment: walletPanel(bal), Target: "wallet-panel"}}, nil
    }
}
```

An action does not have to drop to a `Raw` handler to redirect or drive the client.
A `Patch` carries two escapes that ride the normal Datastar SSE flush:
`Patch{Redirect: "/wallet"}` navigates the client (a location script) and
`Patch{Script: "..."}` runs client JS (`ExecuteScript`). Within one patch the
framework flushes in a fixed order — signals, then script, then fragment, then
redirect — so any fragment or signal updates land before the client navigates away.
Paired with `c.SetCookie(*http.Cookie)` (response headers flush when the SSE stream
opens, after the handler body runs), an action can set a session cookie *and*
redirect on the same response — the post-login bounce without a `Raw` handler.

### `beach.StreamFunc` — SSE subscriptions

Hand-written SSE loops run 200 lines once you handle everything correctly —
subscribe, since-cursor catch-up, select loop, session row, cleanup. Here they
become a declaration; the framework owns the loop:

```go
func LiveStream(h *hub.Hub, pool *pgxpool.Pool) beach.StreamFunc {
    return func(c *beach.Ctx) (beach.Sub, error) {
        return beach.Sub{
            Topics: []string{"chat:" + c.Param("room"), "user:" + c.User().ID.String()},
            CatchUp: func(since string, p beach.Patcher) error {   // ?since= replay
                return replayNotifications(c, db.New(pool), since, p)
            },
        }, nil
    }
}
```

### `beach.SocketFunc` — WebSockets (not hypermedia)

The doctrine ([20-sync.md](20-sync.md)): **SSE carries hypermedia; WebSocket carries
payloads that are not hypermedia** — 60 Hz binary simulation state, upstream
controller input, telemetry. A socket never patches the DOM through the framework;
a page that also wants UI updates keeps a normal `Stream` alongside.

```go
func StateSocket(s *sim.Sim) beach.SocketFunc {
    return func(c *beach.Ctx, sock *beach.Socket) error {
        t := time.NewTicker(time.Second / 60)
        defer t.Stop()
        for {
            select {
            case <-sock.Context().Done():   // canceled on close/shutdown
                return nil
            case <-t.C:
                sock.WriteLatest(encodeState(s)) // latest-state-wins, never blocks
            }
        }
    }
}
```

The framework owns the wire: guards run **before** the upgrade (a rejection is plain
HTTP, never a half-open socket), one writer goroutine serializes frames, a read pump
keeps ping/pong keepalive working even for write-only handlers, and graceful
shutdown sends close 1001 to every live socket (hijacked connections are invisible
to `http.Server.Shutdown`, so the app closes them itself). `Write` is the bounded
ordered queue (acks, control); `WriteLatest` is the depth-1 coalescing mailbox for
state streams — a slow client skips frames instead of building a queue. Returning
nil (or the read loop's terminal error) closes 1000; a panic or unexpected error
closes 1011. `Config.Sockets` tunes size cap, keepalive, queue depth, and allowed
origins (same-origin only by default — browser WS ignores CORS, so the check is
ours). `beach.DialSocket` + `beach.Relay` cover the proxy pattern: terminate the
public socket via `App.Socket`, relay frames to a private backend worker. The
client side is `view/static/js/beach-ws.js` (reconnect with backoff, ArrayBuffer
frames, queue-while-connecting). Wire dependency: vendored `coder/websocket`
(MIT, zero transitive deps) — see [RFC 05](../rfc/05-websockets.md).

### Errors and guards

**Errors are part of the contract.** Handlers return errors, not responses:
`beach.ErrNotFound`, `beach.ErrForbidden`, and validation errors from `beach.Bind`
map to `driftwood.ErrorPage` on navigation and to a `driftwood.ErrorAlert`
toast/inline-validation patch on Datastar requests (`render.go`). One error type,
both renderings, no handler ever writes its own error HTML.

**Guards** (`app.Authed()`, `app.Role(...)`, and the permission guard
`app.Can("devices:write")` backed by the [principal model](12-auth.md)) wrap
session middleware and populate `c.User()` / `c.Principal()`; a `PageFunc` behind a
guard can assume the user exists. `app.Raw` is the documented escape hatch —
payment webhooks must mount outside auth so they execute reliably — and `beach-vet`
flags `http.HandlerFunc` registered anywhere else.

## SSE compression

The conventional wisdom says skip gzip for SSE, because gzip middleware buffers —
events would sit in the compressor's 32KB buffer instead of arriving. That's a
middleware artifact, not a protocol limit, so Beach moves compression into the
SSE path itself instead of giving it up:

- The stream writer (inside `beach.NewSSE`, under every `StreamFunc` and `ActionFunc`
  response) negotiates `Content-Encoding: gzip` via `Accept-Encoding` and
  **sync-flushes per event**: `gzip.Writer.Flush()` (zlib `Z_SYNC_FLUSH`, ~5 bytes
  overhead) then the HTTP flusher. Delivery latency is unchanged; browsers — and
  Datastar's fetch-based SSE — decompress streaming responses transparently.
- The win is outsized for our payloads: a stream of templ fragment patches repeats
  the same tags, classes, and selectors endlessly, and the compressor
  window persists across the whole stream, so every event compresses against
  everything before it. Keepalives compress to almost nothing.
- Gzip, not `deflate`: `Content-Encoding: deflate` carries the ancient
  raw-vs-zlib ambiguity and zero advantage (gzip is deflate plus a header).
  Brotli/zstd are not worth the dependency for v1.
- The honest cost is **per-connection compressor state** (~256–768KB stdlib). At the
  5k-connection budget that is gigabytes, so: `klauspost/compress` at level 1 is the
  default, Huffman-only mode (`beach.SSECompressionLight`) is the low-memory fallback
  (~40–60% savings, near-zero CPU), and `beach.SSECompressionOff` exists per app. The
  driftbottle fan-out benchmark runs with compression on — memory per connection is
  a measured budget, not a hope. `no-transform` is set on SSE responses so proxies
  (Cloudflare tunnels included) leave the encoding alone.

## datastar (typed builders)

Raw `data-*` strings in templ are a build failure via the `beach-vet` analyzer;
the typed builders are the only sanctioned way to emit Datastar attributes. Colon
event syntax (`data-on:click`) is what the builders emit; nobody types it.
