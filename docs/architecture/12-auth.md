# Architecture 12 — `auth` + `passwords`: identity, argon2id, simple RBAC

[← docs index](../README.md) · prev: [Decisions log](11-decisions.md) · next: [API codegen](13-apigen.md)

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
}
```

- **Resolution is boring on purpose**: permissions are resolved once at login from
  the role tables and stored in the session. Changing a user's roles rotates their
  sessions. Every later check is a slice lookup on a struct already in memory — one
  session-backed resolver is the whole story.
- **Anonymous is the absence of a principal**: `c.User()` returns nil. driftbottle
  runs entirely on anonymous sessions; no `Kind` enum required.
- **Guards**: `beach.Authed()`, `beach.Role("admin")`, and the permission guard
  `beach.Can("pantry:write")` ([03-http.md](03-http.md#errors-and-guards)).
  Inside handlers: `c.Principal()` with `HasPermission` / `HasAnyOf`.

The RBAC schema is three tables:

```sql
roles            (id, slug)
role_permissions (role_id, permission)       -- permission is a "resource:action" string
user_roles       (user_id, role_id)
```

Permissions resolve to a flat `resource:action` list on the principal; the three tables
above are the whole schema.

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
