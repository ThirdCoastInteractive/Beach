package ui

import "context"

// Kits travel in the request context: the App builder (beach.KitStatic /
// beach.KitFromDB) selects a kit per request and stashes it here, and handlers
// pull it out via c.Kit(), which is WithKit/KitFrom under the hood.

// ctxKey is the unexported context key under which the active kit is stored.
type ctxKey struct{}

// WithKit returns a copy of ctx carrying k as the active kit. The App builder
// calls this once per request; handlers should not.
func WithKit(ctx context.Context, k Kit) context.Context {
	return context.WithValue(ctx, ctxKey{}, k)
}

// KitFrom returns the active kit from ctx. If none was injected it falls back to
// Mandatory(), so a handler always gets a usable kit (or a loud panic at boot if
// no kit was registered at all — the mandatory-kit enforcement).
func KitFrom(ctx context.Context) Kit {
	if k, ok := ctx.Value(ctxKey{}).(Kit); ok && k != nil {
		return k
	}
	return Mandatory()
}
