# Architecture 12 — `auth` + `passwords`: identity, argon2id, RBAC

[← docs index](../README.md) · prev: [Decisions log](11-decisions.md) · next: [API codegen](13-apigen.md)

Lifted from Atlas (`Y:\Atlas`), where this is already built and production-shaped.
Two packages: `passwords` (argon2id hashing, zero deps) and `auth` (principal model,
login flow, guards backing). Sessions stay Valkey
([05-services.md](05-services.md#session)) but adopt Atlas's hardening.

## `passwords` — argon2id, lifted verbatim

From `Y:\Atlas\pkg\passwords\password.go`, via `github.com/alexedwards/argon2id`:

```go
type Password string                       // stores the PHC-encoded hash

var params = &argon2id.Params{
    Memory:      128 * 1024,               // 128 MB
    Iterations:  4,
    Parallelism: 4,
    SaltLength:  32,                       // 256-bit salt
    KeyLength:   64,                       // 512-bit key
}

func NewPassword(plaintext string) (Password, error)   // length-validated (8..512), hash
func (p Password) Compare(plaintext string) (bool, error) // constant-time
func IsArgonEncoded(input string) bool                 // "$argon2id$" prefix check
```

PHC string encoding (`$argon2id$v=19$m=...`), `sql.Scanner`/`driver.Valuer` +
pgx text scanner/valuer interfaces, tests included. Parameters are deliberately
heavy — login is infrequent. One addition over Atlas: **rehash-on-upgrade** —
`Compare` reports when the stored hash used weaker params so the login handler can
transparently re-hash (Atlas has `must_rotate_password` plumbing but no auto-rehash;
the framework closes that).

## The login flow

Framework-provided handler helpers encode Atlas's hardened flow; the app owns the
templ form and routes:

1. **Pretoken** — GET /login embeds a 10-minute HMAC-SHA256 pretoken
   (purpose-gated, realm-coherent) as a hidden field; POST verifies it. Kills form
   replays for free (`Y:\Atlas\internal\session\pretoken.go`, lifted).
2. **Timing-safe lookup** — unknown username runs a dummy argon2id compare so user
   enumeration can't be timed (lifted from `Y:\Atlas\api\auth.go`).
3. **Lockout** — `failed_login_count` on the credentials row; threshold and duration
   are config (Atlas hardcodes 5 attempts / 15 min). Local state, no distributed
   coordination; this is account protection, not DDoS mitigation.
4. **Session mint** — 32 bytes `crypto/rand`; the store keeps **SHA256(token)**,
   never the raw value; per-session CSRF secret; sliding idle TTL; `Rotate()` on
   privilege change. (Atlas hardening grafted onto the Valkey store.)
5. **Open-redirect guard** — post-login `?return=` must be a relative `/` path.

Logout clears cookies and deletes the session (idempotent, 204). Password-reset
schema ships in the skeleton (`password_reset_tokens`: hashed single-use tokens,
expiry, audit columns); the email/flow handlers are app-side.

## `auth` — the principal model

Every request resolves to a `Principal` (lifted from `Y:\Atlas\pkg\principal`):

```go
type Principal struct {
    Kind        Kind          // user | workload | anonymous
    ID          ID
    Mechanism   Mechanism     // session_local | ... (extensible)
    Permissions []string      // resolved: "devices:read", "racks:write:power_draw"
    Roles       []ID          // audit trail
    SessionID   *ID
    Attestation Attestation   // who/issuer/issued_at — forensics without re-query
}
```

- **Resolver multiplexer**: an ordered chain of resolvers, each returning
  `(principal, recognized, error)`. v1 ships the session resolver; OAuth2/mTLS slots
  exist (Atlas has the stubs) but are not built — same posture as the NATS seam.
- **RBAC schema** in the skeleton: `roles`, `permissions`
  (`resource:action[:field]`), `role_permissions` (allow/deny + scope),
  `principal_groups`, `principal_role_bindings`. Resolved permission strings are
  cached on the session (60-min TTL, configurable) — no O(roles) walk per request.
- **Guard integration**: alongside `beach.Authed()` / `beach.Role(...)`
  ([03-http.md](03-http.md#errors-and-guards)) comes
  **`beach.Can("devices:write")`** — a permission guard backed by the principal.
  Inside handlers, `c.Principal()` exposes `HasPermission`/`HasAnyOf`/`HasAllOf`.
- **Audit hook**: `AuditEmitter` interface called once per request after resolution;
  ships as a slog no-op, apps swap in a real writer.

## What stays app-side

Login/logout/reset page templates and routes, OIDC/JIT user provisioning policy,
role seeding, the email sender. The skeleton stamps the `users` +
`user_credentials_local` + sessions-adjacent + RBAC migrations and a working
login/logout pair so the whole flow is proven on first `make up`.

## Security rules (framework doctrine, from Atlas CLAUDE.md)

Never return `password_hash` (or any secret column) in a `RETURNING` clause or
SELECT * — explicit column lists only; the adapted authz analyzer
([10-tooling.md](10-tooling.md)) greps for violations. Mutating queries carry
`-- @requires <permission>` annotations and hand-written mutation handlers must
call a principal check — both statically verified.
