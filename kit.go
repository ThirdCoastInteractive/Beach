package beach

import "github.com/ThirdCoastInteractive/the-beach/ui"

// KitSelector resolves the UI kit injected into a request context. The App
// builder calls it once at construction; handlers read the result via c.Kit().
// Built-in selectors: KitStatic (a single named kit) and KitNoop (the toolkit's
// no-op fallback, handy in tests). A KitFromDB-style per-request selector is a
// future extension; v1 resolves one kit for the whole app.
type KitSelector func() ui.Kit

// KitStatic selects the registered kit named name (registered via a kit
// package's init() + a blank import). It panics at build if no such kit is
// registered — a deploy that names a missing kit fails loudly at boot, never as
// a blank page.
func KitStatic(name string) KitSelector {
	return func() ui.Kit {
		k, ok := ui.Lookup(name)
		if !ok {
			panic("beach: KitStatic(" + name + "): no kit registered under that name " +
				"— add a blank import of the kit package")
		}
		return k
	}
}

// KitInstance selects a kit value directly, bypassing the registry. Useful when
// a service constructs its kit programmatically rather than via init().
func KitInstance(k ui.Kit) KitSelector {
	return func() ui.Kit { return k }
}

// KitNoop selects the toolkit's built-in no-op kit. It renders the skeleton
// markup against Stellar tokens and is meant for tests and bring-up, not real
// apps.
func KitNoop() KitSelector {
	return func() ui.Kit { return ui.NoopKit() }
}
