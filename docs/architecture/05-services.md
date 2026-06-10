# Architecture 5 — `cache` & `session`

[← docs index](../README.md) · prev: [Hub](04-hub.md) · next: [UI toolkit](06-ui.md)

## cache

Two shapes, both proven in manifold:

- `cache.Snapshot[T]` — atomic-pointer immutable snapshot, lock-free reads,
  `Refresh(ctx, load)` swaps wholesale (plans cache pattern,
  `internal/db/plans_cache.go`).
- `cache.Keyed[K,V]` — RWMutex map with `LoadAll` + per-id `Invalidate` (emote cache
  pattern, `internal/chatemote/cache.go`).

Plus the paved road: `cache.InvalidateOn(ctx, pool, "channel", c)` wires a pgnotify
listener that parses an id payload and invalidates the entry — making
NOTIFY-invalidation the default instead of one-off (fixes the plans-cache manual
refresh inconsistency).

## session

Lifted from manifold's `pkg/session`: Valkey-backed, TTL, per-user session sets
for RevokeAll, `OptionalAuth` / `RequireAuth` / `RequireRole` middleware factories,
`session.UserFrom(ctx)`. The Data/AuthUser duplication collapses to one struct with an
`AuthUser()` view. Role and tier become app-defined string types satisfying small
interfaces, since `db.UserRole` belongs to the app's schema, not the framework.

Hardening adopted from Atlas's session store (the store stays Valkey; the discipline
transfers): the 32-byte `crypto/rand` token is stored as **SHA256(token)**, never
raw; each session carries its own CSRF secret (hash-validated server-side); idle TTL
slides on authenticated requests; `Rotate()` re-mints token + CSRF on privilege
change. Realm/cookie scoping is config, not Atlas's hardcoded operator/portal pair.

The route-level surface is the guards in [03-http.md](03-http.md#errors-and-guards):
`beach.Authed()` / `beach.Role(...)` / `beach.Can(...)` wrap this middleware and
populate `c.User()` and `c.Principal()` — the full identity and RBAC story is
[12-auth.md](12-auth.md).
