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
    Static:   beach.MergedFS(localStatic, sharedStatic),  // service assets over shared
    Kit:      beach.KitStatic("driftwood"),               // or beach.KitFromDB(q, ttl)
    Healthz:  beach.HealthzPing(pool),
})

// Routes register against typed handler shapes (below). Guards make the auth
// story visible in the route table, not buried inside handlers.
app.Page("/",                    handler.Live(pool, s))                           // public
app.Page("/wallet",              handler.Wallet(pool),        beach.Authed())
app.Action("/coins/purchase/:id", handler.PurchaseCoins(pool, s), beach.Authed())
app.Stream("/stream",            handler.LiveStream(hubRef, pool), beach.Authed())
app.Page("/admin/orders",        handler.Orders(pool),        beach.Role("admin"))
app.Raw(http.MethodPost, "/webhooks/stripe", billing.Webhook())  // escape hatch: plain echo

app.Start(":" + cfg.Port)                                // graceful shutdown inside
```

Echo remains underneath (`app.Raw` and `app.Echo()` exist for webhooks and the
genuinely weird), but the paved road is three typed handler shapes that encode the
house rules by construction.

Middleware stack, in fixed order: request logger (skips /healthz, /static),
recover, gzip (skips SSE — see below; SSE gets its own compressor), rate limiter,
secure headers + CSP, CSRF origin check, kit injection. The builder **refuses to
start** if no UI kit is registered — component lookup can never silently fall
through.

`beach.StaticCache` computes SHA256 ETags for every static file at boot and serves
with `immutable` Cache-Control and 304s. Asset URLs carry
`?v=<shortCommit>-<bootTime>`: released binaries get stable commit-based
versioning, dev rebuilds get unique timestamps, and every deploy busts caches
without a manifest.

## Handlers: three typed shapes

The load-bearing rules — dual-purpose handlers branch on `IsDatastar`,
`db.New(pool)` per request, never duplicate page + fragment routes, SSE loops follow
the subscribe/catch-up shape — are the handler types themselves, not documentation:
holding one wrong is a compile error or a shape you cannot express, never a review
comment.

Handlers stay standalone factory functions taking only the deps they need — never
methods on a server struct. But they return beach types instead of raw
`echo.HandlerFunc`, and take a `*beach.Ctx` that carries the request-scoped kit:
`c.Kit()`, `c.User()` (set by guards), `c.InTx(fn)`, `beach.Bind[T](c)` for
validated input binding.

### `beach.PageFunc` — anything you can navigate to

The handler returns a view description; the framework does the dual-purpose branch.
You cannot forget it, and separate fragment routes have nothing to attach to:

```go
func Wallet(pool *pgxpool.Pool) beach.PageFunc {
    return func(c *beach.Ctx) (beach.View, error) {
        data, err := loadWallet(c, db.New(pool))
        if err != nil { return beach.View{}, err }
        return beach.View{
            Page:     page.Wallet(c.Kit(), data),    // full document on navigation
            Fragment: ui.WalletPanel(data),          // patched on Datastar refresh
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
ships as a [deferred section](06-ui.md#deferred-sections-uidefer) whose `@get`
lands back on a `PageFunc` fragment.

### `beach.ActionFunc` — Datastar-only mutations

Navigation gets a 404; the `Datastar-Request` check, CSRF, and method discipline are
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
        return beach.Patches{{Fragment: ui.WalletPanel(bal), Target: "wallet-panel"}}, nil
    }
}
```

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

### Errors and guards

**Errors are part of the contract.** Handlers return errors, not responses:
`beach.ErrNotFound`, `beach.ErrForbidden`, and validation errors from `beach.Bind`
map to the kit's error page on navigation and to a toast/inline-validation patch on
Datastar requests. One error type, both renderings, no handler ever writes its own
error HTML.

**Guards** (`beach.Authed()`, `beach.Role(...)`, and the permission guard
`beach.Can("devices:write")` backed by the [principal model](12-auth.md)) wrap
session middleware and populate `c.User()` / `c.Principal()`; a `PageFunc` behind a
guard can assume the user exists. `app.Raw` is the documented escape hatch —
payment webhooks must mount outside auth so they execute reliably — and `beach vet`
flags `echo.HandlerFunc` registered anywhere else.

## SSE compression

The conventional wisdom says skip gzip for SSE, because gzip middleware buffers —
events would sit in the compressor's 32KB buffer instead of arriving. That's a
middleware artifact, not a protocol limit, so the-beach moves compression into the
SSE path itself instead of giving it up:

- The stream writer (inside `beach.NewSSE`, under every `StreamFunc` and `ActionFunc`
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
  default, Huffman-only mode (`beach.SSECompressionLight`) is the low-memory fallback
  (~40–60% savings, near-zero CPU), and `beach.SSECompressionOff` exists per app. The
  driftbottle fan-out benchmark runs with compression on — memory per connection is
  a measured budget, not a hope. `no-transform` is set on SSE responses so proxies
  (Cloudflare tunnels included) leave the encoding alone.

## datastar (typed builders)

Raw `data-*` strings in templ are a build failure via the `beach vet` analyzer;
the typed builders are the only sanctioned way to emit Datastar attributes. Colon
event syntax (`data-on:click`) is what the builders emit; nobody types it.
