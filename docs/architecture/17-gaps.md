# Architecture 17 — Gaps surfaced by the example apps

[← docs index](../README.md) · prev: [Component catalog](16-components.md) · next: [rybitten](19-rybitten.md)

The three example apps are the framework's falsifiable proof — and in building them, each
hit a rough edge worth fixing. This is the punch-list: what bit, where, and the fix. None
blocked the apps (every one has a working in-app workaround), but each is a real
sharpening of the framework API.

**Status:** the original punch-list below is now drained — every item is shipped behind the
narrow surface that already existed. The "Shipped" line on each names the API that landed.
New Atlas-driven gaps continue to flow through the four capability areas (auth, beach-apigen,
ui, ch) and spawn their own items.

## Boot / HTTP

- [x] **`pg` has no non-panicking pool constructor.** Only `MustPool` exists (panics on a bad
  DSN), so pantry added a local `openPool` to degrade to DB-less instead of crashing.
  **Fix:** add `pg.Pool(ctx, dsn) (*pgxpool.Pool, error)` alongside `MustPool`.
  **Shipped:** `pg.Pool(ctx, dsn) (*pgxpool.Pool, error)`; `MustPool` now delegates to it
  (single construction path).
- [x] **No app-middleware injection on the App builder.** `beach.Config` exposes no hook to
  add middleware, so pantry served via its own `http.Server` over `App.Handler()` wrapped
  in the i18n middleware. **Fix:** `Config.Middleware []func(http.Handler) http.Handler`
  (and/or a first-class `Config.Locales *i18n.Catalog`).
  **Shipped:** `Config.Middleware []func(http.Handler) http.Handler`, applied outermost-first
  around the fixed stack.

- [x] **No WebSocket path.** A high-rate, bidirectional, binary channel (live simulation
  state, controller input, telemetry) had no framework shape — apps would mount a foreign
  handler behind `Raw` and carry their own WS dependency. **Fix:** RFC 05.
  **Shipped:** `App.Socket` (the fourth typed shape) with framework-owned keepalive,
  `WriteLatest` coalescing, guard-before-upgrade, and shutdown close 1001; plus
  `beach.DialSocket` + `beach.Relay` for the proxy pattern and `beach-ws.js` client-side.
  Wire dependency: vendored `coder/websocket`. Demo: driftbottle `/sockets`.

## Actions / SSE surface

- [x] **An `ActionFunc` can't redirect, run a script, or set a cookie.** It returns only
  `Patches`, and `Patch` has no redirect/script/header escape — so pantry's login had to
  be a `Raw` handler (it needed to set the session cookie and redirect). **Fix:** a
  `Patch{Redirect string}` (and/or `Patch{Script string}`), or expose the
  `datastar.ServerSentEventGenerator` on `*Ctx` for the rare action that needs it.
  **Shipped:** `Patch{Redirect string}` and `Patch{Script string}` (flushed in
  signals→script→fragment→redirect order) plus `c.SetCookie(*http.Cookie)`.
- [x] **`hub` events have only one patch mode (outer-morph by id).** Incremental feeds
  (driftbottle chat) must re-render the whole rolling history and morph the container each
  message. **Fix:** carry a patch mode (append/prepend) on `hub.Event` so high-volume
  feeds append a single line instead of re-rendering.
  **Shipped:** `hub.Event.Mode` (`PatchMorph`/`PatchAppend`/`PatchPrepend`) + `Event.Target`,
  consumed by the stream loop via the existing `patchOptions` path.

## Identity

- [x] **No DB-less anonymous sessions.** `Config.Sessions`/`Config.Principals` assume a
  Postgres-backed store, so a fully-ephemeral app (driftbottle) gets `nil` `c.Principal()`
  and an empty `c.User()`. Both driftbottle and pantry minted their own cookie. The charter
  calls for an anonymous principal kind — but there's no cookie-only constructor.
  **Fix:** a cookie-only `session.Store` (or `Config.AnonymousSessions`) and a DB-less
  `auth` anonymous-principal path, so `c.Principal()` works for the anonymous kind.
  **Shipped:** `session.AnonStore` + `Config.AnonymousSessions`, and
  `auth.AnonymousPrincipal(id)` / `Principal.IsAnonymous()`, so `c.Principal()` is non-nil
  for the anonymous kind without Postgres (DB store still takes precedence when set).
- [x] **`i18n.T` can't be used for app strings.** The package-level `T` binds to the
  framework's embedded catalog (unexported), so an app must thread its own `*Catalog`
  through handlers. **Fix:** `i18n.SetDefault(*Catalog)` or resolve the catalog (not just
  the locale) from request context in the middleware.
  **Shipped:** both — `i18n.SetDefault(*Catalog)` and `i18n.WithCatalog(ctx, *Catalog)` (the
  middleware now carries the catalog), resolving context → default → framework fallback.

## Sim

- [x] **No off-loop read of the live store.** Once `Sim.Run` starts, the `ecs.Store` is owned
  by the loop goroutine, so catch-up rendering on page-nav/stream-open must round-trip a
  read command through `Ask`. boardwalk added a `Snapshot(ctx)` AskFunc that copies the
  board to a plain value — workable, but every nav costs a loop round-trip. **Fix:** a
  first-class read-snapshot (a lock-free immutable view, or a documented `Snapshot`
  command as part of the sim API).
  **Shipped:** `ecs.View` (an immutable, memory-disjoint snapshot with `ViewQuery`/`ViewGet`/
  `ViewChanged` read mirrors) captured on-loop via `Sim.Snapshot(ctx) (*ecs.View, error)`.
- [x] **Whole-surface projections need a manual trigger component.** `Changed[T]` is
  per-component, so "re-render the board on *any* mutation" needs a singleton trigger
  component touched by every mutating command (boardwalk's `touchBoard`). The coupling is
  implicit. **Fix:** a projection that fires on "any change within a component set," or a
  board-level dirty helper.
  **Shipped:** `sim.ProjectAny` + `sim.MemberOf[T]()` — a whole-surface projection that fires
  when any member component in the set changed, deduping dirty entities to one render per
  topic per pass.

## Markdown, consent, Stream

- [x] **No Markdown renderer.** Article and post bodies had nowhere to go that
  was sanitized and Stream-aware; apps would wrap goldmark + bluemonday locally.
  **Shipped:** `pkg/md` (`Render` / `RenderWith`, profiles Strict / Post / Article,
  `:::video` fences → Cloudflare Stream iframes). The editor island is
  `/static/js/md-editor.js`; the kit face is `driftwood.MarkdownEditor`.
- [x] **No first-party cookie-preference record.** A prompt that is a modal, or
  a third-party CMP, both fight the kit laws (no focus-trap banner, GPC/DNT
  win). **Shipped:** `pkg/consent` (`NeedsPrompt`, `Allows`, `Cookie`, GPC/DNT
  skip the prompt) plus `driftwood.ConsentBanner` (corner region, stays in the
  DOM), `ConsentManager` (/cookies), `ConsentLink`.
- [x] **No Cloudflare Stream client.** Video upload was either a raw tus call
  in the app or missing. **Shipped:** `pkg/storage/cfstream` (`CreateTus` returns
  Location + UID; the browser PATCHes; the API token stays on the server).

## Disposition

These were sharpenings, not redesigns — each landed as a small additive change behind the
existing surface, with the example that demanded it as the test. The cheap, high-value ones
(`pg.Pool`, `Config.Middleware`, `Patch.Redirect`, `hub` append mode, `i18n.SetDefault`) went
first; the sim read/projection ergonomics were a slightly larger design touch. The list is
drained; future framework gaps flow through the capability areas.
