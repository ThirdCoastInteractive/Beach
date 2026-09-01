# Architecture 12 — `auth` + `passwords`: identity, argon2id, simple RBAC

[← docs index](../README.md) · prev: [Tooling & testing](10-tooling.md) · next: [API codegen](13-apigen.md)

Two packages: `passwords` (argon2id hashing, zero dependencies) and `auth` (the
principal, the login flow, and a deliberately small RBAC). Sessions are covered in
[05-services.md](05-services.md#session); auth builds on them.

## `passwords` — argon2id

Built on `github.com/alexedwards/argon2id`:

```go
type Password string                       // stores the PHC-encoded hash

var params = &argon2id.Params{
    Memory:      128 * 1024,               // 128 MB
    Iterations:  4,
    Parallelism: 4,
    SaltLength:  32,                       // 256-bit salt
    KeyLength:   64,                       // 512-bit key
}

func NewPassword(plaintext string) (Password, error)      // length-validated (8..512)
func (p Password) Compare(plaintext string) (bool, error) // constant-time
func IsArgonEncoded(input string) bool                    // "$argon2id$" prefix check
```

PHC string encoding (`$argon2id$v=19$m=...`), `sql.Scanner`/`driver.Valuer` plus
pgx text scanner/valuer interfaces, tests included. The parameters are deliberately
heavy — login is infrequent. `Compare` also reports when the stored hash used
weaker parameters, so the login handler can transparently **rehash on upgrade**:
parameter bumps roll out organically, one successful login at a time.

## The login flow

Framework-provided handler helpers encode the hardened flow; the app owns the
templ form and the routes:

1. **Pretoken** — GET /login embeds a 10-minute HMAC-SHA256 pretoken
   (purpose-gated) as a hidden field; POST verifies it. Kills form replays for
   free.
2. **Timing-safe lookup** — an unknown username runs a dummy argon2id compare so
   user enumeration can't be timed.
3. **Lockout** — `failed_login_count` on the credentials row; threshold and
   duration are config (default 5 attempts / 15 minutes). Local state, no
   distributed coordination; this is account protection, not DDoS mitigation.
4. **Session mint** — 32 bytes `crypto/rand`; the store keeps **SHA256(token)**,
   never the raw value; per-session CSRF secret; sliding idle TTL; `Rotate()` on
   privilege change ([05-services.md](05-services.md#session)).
5. **Open-redirect guard** — post-login `?return=` must be a relative `/` path.

Logout clears cookies and deletes the session (idempotent, 204). The
password-reset schema ships in the skeleton (`password_reset_tokens`: hashed
single-use tokens, expiry, audit columns); the email and flow handlers are
app-side.

## `auth` — the principal, kept small

```go
type Principal struct {
    UserID      ID
    Username    string
    Roles       []string   // role slugs
    Permissions []string   // "resource:action" strings
    Scope       string     // owner/customer boundary (opaque to Beach)
    AnonID      string     // stable cookie-only id of an anonymous principal
    Anonymous   bool       // true for the cookie-only anonymous kind
}
```

- **Resolution is boring on purpose**: permissions are resolved once at login from
  the role tables and stored in the session. Changing a user's roles rotates their
  sessions. Every later check is a slice lookup on a struct already in memory — one
  session-backed resolver is the whole story.
- **Scope is the owner/customer boundary**: `Scope` is a generic string Beach
  carries and checks but never interprets — what a scope *means* (a customer id,
  an org slug, a tenant) is an app decision. `Principal.InScope(scope)` is the
  in-memory predicate: a nil/anonymous principal, an unscoped principal (`Scope ==
  ""`), or one carrying a different scope all fail; only an exact match passes. It
  pairs with the `beach-apigen` `@scoped` static rule
  ([13-apigen.md](13-apigen.md)), which forces a scoped query to actually take its
  scope parameter at build time.
- **Anonymous has two shapes.** The default is the *absence* of a principal:
  `c.Principal()` returns nil, the unauthenticated request the guards reject.
  driftbottle runs this way; no `Kind` enum required. The second is the explicit
  cookie-only kind: a DB-less app that wants a stable identity without Postgres
  uses `auth.AnonymousPrincipal(id)` — it carries an opaque `AnonID`, holds **no**
  roles, permissions, or scope, and answers true to `IsAnonymous()`. Because it
  holds nothing, every `Can`/`Role`/`Scope` check still denies it, so guards treat
  it as unprivileged while handlers that only need a well-defined identity read
  `AnonID`. (`Config.AnonymousSessions` wires `c.Principal()` to return this kind
  instead of nil — see [05-services.md](05-services.md#session).)
- **Guards**: `beach.Authed()`, `beach.Role("admin")`, and the permission guard
  `beach.Can("pantry:write")` ([03-http.md](03-http.md#errors-and-guards)).
  `beach.TokenAuthed()` and `beach.InScope(scope)` (below) register on a route the
  same way. Inside handlers: `c.Principal()` with `HasPermission` / `HasAnyOf` /
  `InScope`.

The RBAC schema is three tables:

```sql
roles            (id, slug)
role_permissions (role_id, permission)       -- permission is a "resource:action" string
user_roles       (user_id, role_id)
```

Permissions resolve to a flat `resource:action` list on the principal; the three tables
above are the whole schema.

The `users.id` column is `bigint GENERATED BY DEFAULT AS IDENTITY` (not `GENERATED
ALWAYS`): an insert that omits `id` gets a DB-minted value — the example-app login
path — while an app that mints its own 64-bit identifier may supply `id`
explicitly. Both are first-class; neither is privileged over the other.

## API / bearer tokens

Non-interactive callers (service accounts, CLIs) authenticate with a bearer token
that resolves to the **same** `Principal` an interactive login produces — roles
and permissions resolved from the same three tables. The token is a
`<prefix>.<secret>` pair:

- `prefix` is a public, indexed lookup handle — safe to log and to show in a token
  list.
- `secret` is the bearer credential. Only `SHA256(secret)` is persisted; the raw
  token is returned **exactly once**, at mint time, and never recoverable after.
  This is the secret-column doctrine again — neither the raw secret nor the hash
  ever appears in a `RETURNING` clause or `SELECT *`.

Lookup is a single fetch by `prefix` followed by a constant-time hash compare, so
the table stays cheap and the comparison cannot be timed (an unknown prefix spends
a throwaway compare to stay timing-equivalent to a known one).

```go
func (a *Authenticator) MintToken(ctx, userID ID, name string, expiresAt *time.Time) (Token, error)
func (a *Authenticator) ResolveToken(ctx, raw string) (*Principal, error)
func (a *Authenticator) RevokeToken(ctx, prefix string) error
var ErrInvalidToken = errors.New("auth: invalid token")
```

- `MintToken` writes `SHA256(secret)` keyed by `prefix` and returns the raw
  `<prefix>.<secret>` in `Token.Raw`; `expiresAt` may be nil for a non-expiring
  token.
- `ResolveToken` verifies the secret, resolves the bearer into a `Principal`, and
  stamps `last_used_at`. Malformed, unknown, expired, **and** revoked all collapse
  to the single `ErrInvalidToken` so a caller cannot probe which tokens exist.
- `RevokeToken` is idempotent — revoking an unknown or already-revoked prefix is
  not an error.

The `api_tokens` table:

```sql
api_tokens (
    prefix       text  PRIMARY KEY,   -- public lookup handle
    token_hash   bytea NOT NULL,      -- SHA256(secret); raw secret never stored
    user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         text   NOT NULL DEFAULT '',  -- human label
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz,         -- NULL means no expiry
    revoked_at   timestamptz,         -- non-NULL once revoked
    last_used_at timestamptz
)
```

## Route guards: bearer auth and scope

Guards are ordered wrappers on the route table, so the auth story is visible at the
registration site ([03-http.md](03-http.md#errors-and-guards)). Two compose on top
of the principal, registerable on a route exactly like `beach.Can(...)`:

- `beach.TokenAuthed()` reads the `Authorization: Bearer <token>` header, resolves
  it to a principal via the configured `TokenResolver` (`Config.Tokens` — typically
  wired to `Authenticator.ResolveToken`), and attaches that principal to the
  request context. It is the bearer counterpart to the session
  `PrincipalResolver` (`Config.Principals`): with it in front, `Can(...)` and
  `InScope(...)` compose the same way they do for a logged-in user. A missing or
  invalid token — or no configured resolver — gets **401** and the handler never
  runs.
- `beach.InScope(scope)` enforces `Principal.InScope`: an anonymous, unscoped, or
  wrong-scope principal is rejected with **403** before the handler body runs.

These pair with the source-level guards on the principal (`Can`, `InScope`) and the
`beach-apigen` `@requires`/`@scoped` annotations, so the same boundary is checked at
the route, in the handler, and at generate time.

## What stays app-side

Login/logout/reset page templates and routes, user provisioning, role seeding,
the email sender. The skeleton stamps the `users` + `user_credentials_local` +
RBAC migrations and a working login/logout pair so the whole flow is proven on
first `make up`.

## Security rules (framework doctrine)

Never return `password_hash` (or any secret column) in a `RETURNING` clause or
`SELECT *` — explicit column lists only; the authz analyzer
([10-tooling.md](10-tooling.md)) greps for violations. Mutating queries carry
`-- @requires <permission>` annotations and hand-written mutation handlers must
call a principal check — both statically verified.
