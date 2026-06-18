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
// Anonymous is the absence of a Principal: PrincipalFrom returns (nil, false) and
// the guards treat that as unauthenticated. There is no Kind enum.
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
