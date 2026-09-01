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

The pool both of these lean on opens two ways. `pg.MustPool(ctx, dsn)` is the
boot-time path — a dead database means the process can't serve, so it panics.
`pg.Pool(ctx, dsn) (*pgxpool.Pool, error)` is the same constructor (new pool, then
ping) returning the error instead, for a caller that wants to report a clean boot
failure itself; `MustPool` now just wraps it. (Full pool/migration/transaction story
is [02-boot-spine.md](02-boot-spine.md).)

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

**DB-less anonymous sessions.** A fully-ephemeral app with no Postgres can still
hand every visitor a stable identity. `session.AnonStore` is the cookie-only store —
the same 32-byte `crypto/rand` token the DB store mints, but never persisted: the
cookie value *is* the session, so there is no row to look up. Its `Ensure`
middleware reads-or-mints the id, refreshes the cookie each request (the idle window
slides), and puts the id on the context for `c.AnonID()`. Wire it through
`beach.Config{AnonymousSessions: ...}`: the App carries an `auth.AnonymousPrincipal`,
so `c.Principal()` is non-nil (and `IsAnonymous()` true) for the anonymous kind
without a database — but it holds no roles or permissions, so the guards still reject
it. `Config.Sessions` takes precedence: a DB-backed app resolves real identities and
the anonymous store is ignored.

The route-level surface is the guards in [03-http.md](03-http.md#errors-and-guards):
`beach.Authed()` / `beach.Role(...)` / `beach.Can(...)` wrap this middleware and
populate `c.User()` and `c.Principal()` — the full identity and RBAC story is
[12-auth.md](12-auth.md).

## i18n

A real runtime with mechanical verification, kept deliberately flat:

- **Catalog**: `pkg/i18n/catalog.json` maps each key to a reference label and a
  translator comment; `pkg/i18n/locales/<tag>.json` (e.g. `en-US.json`) maps keys
  to translations. Both embedded via `go:embed`, loaded once at boot. The
  framework ships its own strings there: the error pages, and every accessible
  name the `driftwood` kit emits (`ui.a11y.*` — see
  [RFC 06](../rfc/06-accessibility.md)).
- **Lookup**: `i18n.T(ctx, "pantry.items.title")` — **literal keys only**
  (lowercase, digits, dot, underscore, hyphen), enforced by the extractor.
  Locale comes from the request context. A key missing from the active locale
  falls back to the default locale; dev mode logs the gap.
- **Locale resolution**: cookie, then `Accept-Language`, then the configured
  default — set once by middleware the App builder wires when
  `beach.Config{Locales: *i18n.Catalog}` is present. It wraps outermost, so
  every layer inside (app middleware, request logging, recover) sees a request
  that already knows its language. Apps that never configure it pay nothing: `T`
  resolves against the default locale and the whole feature is inert.
- **Direction**: `i18n.Dir(tag)` reports a tag's base writing direction from a
  small built-in table of right-to-left languages and scripts — no new
  dependency. The page shell writes both `lang` and `dir` from the active
  locale, because a screen reader picks its voice and pronunciation rules from
  that attribute (WCAG 3.1.1). An RTL locale gets the right direction today; the
  stylesheet has not been swept from physical to logical properties yet, which
  [RFC 06](../rfc/06-accessibility.md) records as a known limitation.
- **Catalog resolution**: the package-level `i18n.T(ctx, key)` resolves app
  strings without threading a `*Catalog`. The catalog is picked in order: one
  carried on the request context (the middleware now installs `i18n.WithCatalog`
  alongside the locale, so the active app catalog rides each request), then the
  process default registered once at boot via `i18n.SetDefault(c)`, then the
  framework's embedded catalog as the ultimate fallback. A key absent everywhere
  is returned verbatim, never fatal.
- **Tooling**: `beach i18n [--write] [--dir D] [--catalog P]` extracts every
  literal-key `i18n.T` call from Go and generated templ source and verifies (or
  `--write`s) the catalog, exiting non-zero on a missing or stale key. Run it in
  CI beside `beach-vet` — it is a standalone command, not a `beach-vet` rule.
  One key set, mechanically verified: no orphaned translations, no untranslated
  keys discovered in production. The framework's own catalog is checked the same
  way: `beach i18n --dir pkg --catalog pkg/i18n/catalog.json`.

Flat keys with `fmt`-style argument substitution. One key set, mechanically verified,
resolved against the active locale.
