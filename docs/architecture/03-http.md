# Architecture 3 — HTTP layer: `web` + `datastar`

[← docs index](../README.md) · prev: [Boot spine](02-boot-spine.md) · next: [Hub](04-hub.md)

## App builder

The five manifold `cmd/*/main.go` files are the same 150 lines with different CSP
strings. The builder collapses them:

```go
app := web.New(web.Config{
    Service:  "live",
    Release:  cfg.IsRelease(),
    CSP:      web.CSPDefault().AllowMedia("https://customer-x.cloudflarestream.com"),
    Static:   web.MergedFS(localStatic, sharedStatic),  // service assets over shared
    Chrome:   web.ChromeStatic("driftwood"),            // or web.ChromeFromDB(q, ttl)
    Healthz:  web.HealthzPing(pool),
})

// Routes register against typed handler shapes (below). Guards make the auth
// story visible in the route table, not buried inside handlers.
app.Page("/",                    handler.Live(pool, w))                          // public
app.Page("/wallet",              handler.Wallet(pool),       web.Authed())
app.Action("/coins/purchase/:id", handler.PurchaseCoins(pool, w), web.Authed())
app.Stream("/stream",            handler.LiveStream(hubRef, pool), web.Authed())
app.Page("/admin/orders",        handler.Orders(pool),       web.Role("admin"))
app.Raw(http.MethodPost, "/webhooks/stripe", billing.Webhook())  // escape hatch: plain echo

app.Start(":" + cfg.Port)                               // graceful shutdown inside
```

Echo remains underneath (`app.Raw` and `app.Echo()` exist for webhooks and the
genuinely weird), but the paved road is three typed handler shapes that encode the
house rules by construction.

Middleware stack (fixed order, lifted from `cmd/admin/main.go:181-204`): request
logger (skips /healthz, /static), recover, gzip (skips SSE — see below; SSE gets its
own compressor), rate limiter, secure headers + CSP, CSRF origin check, chrome
injection. The builder **refuses to start** if no chrome is registered (fixing the
nullability wart).

`web.StaticCache` is lifted as-is: boot-time SHA256 ETags, `immutable` Cache-Control,
304s. Asset versioning keeps the `?v=<shortCommit>-<bootTime>` scheme from
`internal/view/tmpl/layout/page.templ`.

## Handlers: three typed shapes

In manifold, the load-bearing rules — dual-purpose handlers branch on `IsDatastar`,
`db.New(pool)` per request, never duplicate page + fragment routes, SSE loops follow
the registry/catch-up shape — live in instruction docs and lint guards. In the-beach
they are the handler types themselves: holding one wrong is a compile error or a
shape you cannot express, not a review comment.

Handlers stay standalone factory functions taking only the deps they need — never
methods on a server struct. But they return beach types instead of raw
`echo.HandlerFunc`, and take a `*web.Ctx` that carries the request-scoped kit:
`c.Chrome()`, `c.User()` (set by guards), `c.InTx(fn)`, `web.Bind[T](c)` for
validated input binding.

### `web.PageFunc` — anything you can navigate to

The handler returns a view description; the framework does the dual-purpose branch.
You cannot forget it, and separate fragment routes have nothing to attach to:

```go
func Wallet(pool *pgxpool.Pool) web.PageFunc {
    return func(c *web.Ctx) (web.View, error) {
        data, err := loadWallet(c, db.New(pool))
        if err != nil { return web.View{}, err }
        return web.View{
            Page:     page.Wallet(c.Chrome(), data), // full document on navigation
            Fragment: ui.WalletPanel(data),          // patched on Datastar refresh
            Target:   "wallet-panel",
        }, nil
    }
}
```

Tabs, filters, pagination, sort reuse the page URL with query params — same rule as
manifold, now structural: a `View` always knows how to render both ways.

### `web.ActionFunc` — Datastar-only mutations

Navigation gets a 404; the `Datastar-Request` check, CSRF, and method discipline are
the framework's problem. The handler does the mutation and says what to patch:

```go
func PurchaseCoins(pool *pgxpool.Pool, w *world.World) web.ActionFunc {
    return func(c *web.Ctx) (web.Patches, error) {
        req, err := web.Bind[PurchaseReq](c)         // bind + validate, typed
        if err != nil { return nil, err }
        var bal db.Balance
        err = c.InTx(pool, func(q *db.Queries) error { // money: commit first — see
            bal, err = purchase(c, q, req)             // 08-world.md "Persistence"
            return err
        })
        if err != nil { return nil, err }
        w.Send(cmd.ApplyCommitted{User: c.User().ID})
        return web.Patch(ui.WalletPanel(bal)).Target("wallet-panel"), nil
    }
}
```

### `web.StreamFunc` — SSE subscriptions

Manifold's hand-written 200-line SSE loops (subscribe, since-cursor catch-up, select
loop, session row, cleanup) become a declaration; the framework owns the loop:

```go
func LiveStream(h *hub.Hub, pool *pgxpool.Pool) web.StreamFunc {
    return func(c *web.Ctx) (web.Sub, error) {
        return web.Sub{
            Topics: []string{"chat:" + c.Param("room"), "user:" + c.User().ID.String()},
            CatchUp: func(since string, p web.Patcher) error {   // ?since= replay
                return replayNotifications(c, db.New(pool), since, p)
            },
        }, nil
    }
}
```

### Errors and guards

**Errors are part of the contract.** Handlers return errors, not responses:
`web.ErrNotFound`, `web.ErrForbidden`, and validation errors from `web.Bind` map to
the chrome's error page on navigation and to a toast/inline-validation patch on
Datastar requests. One error type, both renderings, no handler ever writes its own
error HTML.

**Guards** (`web.Authed()`, `web.Role(...)`) wrap session middleware and populate
`c.User()`; a `PageFunc` behind a guard can assume the user exists. `app.Raw` is the
documented escape hatch (Stripe webhooks must mount outside auth — the manifold
pattern), and `beach vet` flags `echo.HandlerFunc` registered anywhere else.

## SSE compression

Manifold skips gzip for SSE because Echo's middleware buffers — events would sit in
the compressor's 32KB buffer instead of arriving. That's a middleware artifact, not a
protocol limit, so the-beach moves compression into the SSE path itself instead of
giving it up:

- The stream writer (inside `web.NewSSE`, under every `StreamFunc` and `ActionFunc`
  response) negotiates `Content-Encoding: gzip` via `Accept-Encoding` and
  **sync-flushes per event**: `gzip.Writer.Flush()` (zlib `Z_SYNC_FLUSH`, ~5 bytes
  overhead) then the HTTP flusher. Delivery latency is unchanged; browsers — and
  Datastar's fetch-based SSE — decompress streaming responses transparently.
- The win is outsized for our payloads: a stream of templ fragment patches repeats
  the same tags, Tailwind classes, and selectors endlessly, and the compressor
  window persists across the whole stream, so every event compresses against
  everything before it. Keepalives compress to almost nothing.
- Gzip, not `deflate`: `Content-Encoding: deflate` carries the ancient
  raw-vs-zlib ambiguity and zero advantage (gzip is deflate plus a header).
  Brotli/zstd are not worth the dependency for v1.
- The honest cost is **per-connection compressor state** (~256–768KB stdlib). At the
  5k-connection budget that is gigabytes, so: `klauspost/compress` at level 1 is the
  default, Huffman-only mode (`web.SSECompressionLight`) is the low-memory fallback
  (~40–60% savings, near-zero CPU), and `web.SSECompressionOff` exists per app. The
  boardwalk fan-out benchmark runs with compression on — memory per connection is a
  measured budget, not a hope. `no-transform` is set on SSE responses so proxies
  (Cloudflare tunnels included) leave the encoding alone.

## datastar (typed builders)

Lifted from `manifold13.tv/pkg/datastar`. Raw `data-*` strings in templ are a build
failure via the `beach vet` analyzer (manifold's `lint-datastar`, promoted from a grep
in a Makefile to a `go/analysis` pass). Colon event syntax (`data-on:click`) is what
the builders emit; nobody types it.
