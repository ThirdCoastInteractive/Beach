# Architecture 5 — `cache`, `session`, `i18n`

[← docs index](../README.md) · prev: [Hub](04-hub.md) · next: [UI toolkit](06-ui.md)

## cache

Two shapes:

- `cache.Snapshot[T]` — an atomic-pointer immutable snapshot with lock-free reads;
  `Refresh(ctx, load)` builds a new snapshot and swaps it wholesale. For small,
  hot, read-everywhere data (plans, palettes, feature tables).
- `cache.Keyed[K,V]` — an RWMutex map with `LoadAll` at boot and per-id
  `Invalidate`. For collections where entries change one at a time (emotes,
  uploaded assets).

Plus the paved road: `cache.InvalidateOn(ctx, pool, "channel", c)` wires a
`pg.Listen` listener that parses an id payload and invalidates the entry —
NOTIFY-driven invalidation is the default, not a per-cache afterthought.

## session

Postgres-backed sessions with auth middleware: one `sessions` table, queried through
sqlc like everything else — no second datastore to run, back up, or reason about.
`RevokeAll` is a single `DELETE ... WHERE user_id = $1` (an index on `user_id` makes
it cheap); `OptionalAuth` / `RequireAuth` / `RequireRole` middleware factories;
`session.UserFrom(ctx)`. One identity struct with derived views — never parallel
copies of user data.

The hardening discipline: the 32-byte `crypto/rand` token is stored as
**SHA256(token)** — the row's primary key — never raw; each session carries its own
CSRF secret (hash-validated server-side); idle TTL slides on authenticated requests
(`expires_at = now() + ttl`); `Rotate()` re-mints token + CSRF on privilege change
(delete the old row, insert the new one in one `InTx`). Reads filter
`expires_at > now()`; a periodic sweep (`DELETE ... WHERE expires_at < now()`) reaps
the dead rows. Cookie name, path, and lifetimes are plain config — there is no
multi-realm machinery; no beach app has two realms.

The sliding write is throttled — a session past, say, half its TTL takes one
`UPDATE`, not one per request — so authenticated traffic doesn't turn every GET into
a write. The `sessions` migration ships in the auth skeleton alongside the
users/credentials/RBAC tables.

The route-level surface is the guards in [03-http.md](03-http.md#errors-and-guards):
`beach.Authed()` / `beach.Role(...)` / `beach.Can(...)` wrap this middleware and
populate `c.User()` and `c.Principal()` — the full identity and RBAC story is
[12-auth.md](12-auth.md).

## i18n

A real runtime with mechanical verification, kept deliberately flat:

- **Catalog**: `internal/view/i18n/catalog.json` maps each key to a reference
  label and a translator comment; `locales/<tag>.json` (e.g. `en-US.json`) maps
  keys to translations. Both embedded via `go:embed`, loaded once at boot.
- **Lookup**: `i18n.T(ctx, "pantry.items.title")` — **literal keys only**
  (lowercase, digits, dot, underscore, hyphen), enforced by the extractor.
  Locale comes from the request context. A key missing from the active locale
  falls back to the default locale; dev mode logs the gap.
- **Locale resolution**: cookie, then `Accept-Language`, then the configured
  default — set once by middleware the App builder wires when
  `beach.Config{Locales: ...}` is present. Apps that never configure it pay
  nothing: `T` resolves against the default locale and the whole feature is
  inert.
- **Tooling**: `beach i18n` extracts every `i18n.T` call from templ source and
  verifies (or `--write`s) the catalog; the `beach vet` analyzer
  ([10-tooling.md](10-tooling.md)) fails the build on non-literal keys or a stale
  catalog. One key set, mechanically verified — no orphaned translations, no
  untranslated keys discovered in production.

Flat keys with `fmt`-style argument substitution. One key set, mechanically verified,
resolved against the active locale.
