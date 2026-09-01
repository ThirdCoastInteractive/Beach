// Package auth is the identity layer: the Principal, the hardened login flow,
// and a deliberately small RBAC.
//
// See docs/architecture/12-auth.md. It builds on two lower packages:
// passwords (argon2id hashing) and session (the Postgres-backed session store).
//
// RBAC is boring on purpose. Permissions are resolved once, at login, from the
// three role tables (roles, role_permissions, user_roles) and flattened onto the
// Principal as "resource:action" strings stored in the session. Every later check
// is a slice lookup on a struct already in memory. Changing a user's roles rotates
// their sessions, so a stale principal cannot outlive a privilege change.
//
// Anonymous has two shapes. The absence of a Principal — PrincipalFrom returns
// (nil, false) — is the unauthenticated request the guards reject. A DB-less app
// that wants a stable, cookie-only identity instead uses an explicit anonymous
// principal (AnonymousPrincipal): it carries an opaque id, holds no roles or
// permissions, and answers true to IsAnonymous. It satisfies no Can/Role/Scope
// check, so the guards still treat it as unprivileged.
package auth

import (
	"context"
)

// ID is the user identifier. It mirrors session.User.ID (a bigint primary key).
type ID = int64

// Principal is the resolved identity for an authenticated request. It is built
// once at login and carried in the session; handlers read it back via
// PrincipalFrom. Permissions are a flat list of "resource:action" strings.
type Principal struct {
	UserID      ID
	Username    string
	Roles       []string // role slugs
	Permissions []string // "resource:action" strings

	// Scope is the owner/customer boundary the principal acts within — a generic
	// string the framework carries and checks but never interprets. Which scopes
	// exist (a customer id, an org slug, a tenant) is an app-side decision; Beach
	// only provides the primitive and the InScope check the guard layer enforces.
	// An empty Scope is unscoped: it satisfies no specific-scope check.
	Scope string

	// AnonID, when non-empty, is the stable cookie-only identity of an anonymous
	// principal (see AnonymousPrincipal). It is opaque to Beach — apps key
	// ephemeral per-visitor state on it. A DB-backed (logged-in) principal leaves
	// it empty.
	AnonID string

	// Anonymous marks a principal minted from a cookie-only session rather than a
	// real account. An anonymous principal holds no roles or permissions, so every
	// Can/Role/Scope check denies it; handlers that simply need a well-defined,
	// stable identity read AnonID.
	Anonymous bool
}

// AnonymousPrincipal builds the cookie-only anonymous principal for a DB-less
// session: a stable opaque id, no roles, no permissions. It is the explicit
// anonymous kind the charter calls for — c.Principal() returns one (not nil) for
// an ephemeral app, so handlers have a well-defined identity without Postgres. It
// satisfies no Can/Role/Scope check.
func AnonymousPrincipal(id string) *Principal {
	return &Principal{AnonID: id, Anonymous: true}
}

// IsAnonymous reports whether the principal is the cookie-only anonymous kind. A
// nil (absent) principal is not anonymous in this sense — it is unauthenticated.
func (p *Principal) IsAnonymous() bool {
	return p != nil && p.Anonymous
}

// HasRole reports whether the principal holds the given role slug.
func (p *Principal) HasRole(role string) bool {
	if p == nil {
		return false
	}
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasPermission reports whether the principal holds the exact "resource:action"
// permission. A nil principal (anonymous) never has a permission.
func (p *Principal) HasPermission(perm string) bool {
	if p == nil {
		return false
	}
	for _, have := range p.Permissions {
		if have == perm {
			return true
		}
	}
	return false
}

// HasAnyOf reports whether the principal holds at least one of the given
// permissions. With no permissions listed it returns false.
func (p *Principal) HasAnyOf(perms ...string) bool {
	if p == nil {
		return false
	}
	for _, want := range perms {
		if p.HasPermission(want) {
			return true
		}
	}
	return false
}

// Can is the permission guard predicate: it reports whether the principal may
// perform "resource:action". It is the in-memory half of beach.Can(...); a nil
// principal can do nothing.
func (p *Principal) Can(perm string) bool {
	return p.HasPermission(perm)
}

// InScope reports whether the principal acts within the given owner/customer
// scope. It is the in-memory half of the scope guard: a nil (anonymous) or
// unscoped principal is denied, and a principal carrying a different scope is
// denied. The scope string is opaque to Beach — the app decides what a scope
// means; this only enforces an exact match.
func (p *Principal) InScope(scope string) bool {
	if p == nil || p.Scope == "" {
		return false
	}
	return p.Scope == scope
}

// --- request context ---

// ctxKey is the unexported context key for the Principal.
type ctxKey struct{}

// WithPrincipal returns a copy of ctx carrying the principal.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// PrincipalFrom returns the principal attached to ctx, and whether one is
// present. Anonymous requests carry no principal: ok is false and the principal
// is nil.
func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(*Principal)
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}
