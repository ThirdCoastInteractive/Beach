# Architecture 17 — Gaps surfaced by the example apps

[← docs index](../README.md) · prev: [Component catalog](16-components.md)

The three example apps are the framework's falsifiable proof — and in building them, each
hit a rough edge worth fixing. This is the punch-list: what bit, where, and the fix. None
blocked the apps (every one has a working in-app workaround), but each is a real
sharpening of the framework API.

## Boot / HTTP

- **`pg` has no non-panicking pool constructor.** Only `MustPool` exists (panics on a bad
  DSN), so pantry added a local `openPool` to degrade to DB-less instead of crashing.
  **Fix:** add `pg.Pool(ctx, dsn) (*pgxpool.Pool, error)` alongside `MustPool`.
- **No app-middleware injection on the App builder.** `beach.Config` exposes no hook to
  add middleware, so pantry served via its own `http.Server` over `App.Handler()` wrapped
  in the i18n middleware. **Fix:** `Config.Middleware []func(http.Handler) http.Handler`
  (and/or a first-class `Config.Locales *i18n.Catalog`).

## Actions / SSE surface

- **An `ActionFunc` can't redirect, run a script, or set a cookie.** It returns only
  `Patches`, and `Patch` has no redirect/script/header escape — so pantry's login had to
  be a `Raw` handler (it needed to set the session cookie and redirect). **Fix:** a
  `Patch{Redirect string}` (and/or `Patch{Script string}`), or expose the
  `datastar.ServerSentEventGenerator` on `*Ctx` for the rare action that needs it.
- **`hub` events have only one patch mode (outer-morph by id).** Incremental feeds
  (driftbottle chat) must re-render the whole rolling history and morph the container each
  message. **Fix:** carry a patch mode (append/prepend) on `hub.Event` so high-volume
  feeds append a single line instead of re-rendering.

## Identity

- **No DB-less anonymous sessions.** `Config.Sessions`/`Config.Principals` assume a
  Postgres-backed store, so a fully-ephemeral app (driftbottle) gets `nil` `c.Principal()`
  and an empty `c.User()`. Both driftbottle and pantry minted their own cookie. The charter
  calls for an anonymous principal kind — but there's no cookie-only constructor.
  **Fix:** a cookie-only `session.Store` (or `Config.AnonymousSessions`) and a DB-less
  `auth` anonymous-principal path, so `c.Principal()` works for the anonymous kind.
- **`i18n.T` can't be used for app strings.** The package-level `T` binds to the
  framework's embedded catalog (unexported), so an app must thread its own `*Catalog`
  through handlers. **Fix:** `i18n.SetDefault(*Catalog)` or resolve the catalog (not just
  the locale) from request context in the middleware.

## Sim

- **No off-loop read of the live store.** Once `Sim.Run` starts, the `ecs.Store` is owned
  by the loop goroutine, so catch-up rendering on page-nav/stream-open must round-trip a
  read command through `Ask`. boardwalk added a `Snapshot(ctx)` AskFunc that copies the
  board to a plain value — workable, but every nav costs a loop round-trip. **Fix:** a
  first-class read-snapshot (a lock-free immutable view, or a documented `Snapshot`
  command as part of the sim API).
- **Whole-surface projections need a manual trigger component.** `Changed[T]` is
  per-component, so "re-render the board on *any* mutation" needs a singleton trigger
  component touched by every mutating command (boardwalk's `touchBoard`). The coupling is
  implicit. **Fix:** a projection that fires on "any change within a component set," or a
  board-level dirty helper.

## Disposition

These are sharpenings, not redesigns — most are a small additive change behind the
existing surface. The cheap, high-value ones (`pg.Pool`, `Config.Middleware`,
`Patch.Redirect`, `hub` append mode, `i18n.SetDefault`) are worth doing first; the sim
read/projection ergonomics are a slightly larger design touch. Each lands the same way
everything does: behind the narrow interface that already exists, with the example that
demanded it as the test.
