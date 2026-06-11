package ui

import (
	"fmt"
	"sort"
	"sync"
)

// The registry lets a kit register itself in init() so that an app pulls it in
// with a side-effect import:
//
//	import _ "github.com/.../ui/driftwood"
//
// driftwood's init() calls ui.Register(driftwoodKit). The app then names the
// mandatory kit (the one a page renders against by default) via SetMandatory,
// and Mandatory() panics if none was ever registered — a deploy that ships no
// kit is a build-time-shaped error surfaced loudly at boot, never a blank page.

var (
	regMu     sync.RWMutex
	registry  = map[string]Kit{}
	mandatory string // name of the mandatory kit, "" until set
)

// Register records a kit under its Name. A kit calls this from init(). Two kits
// with the same name is a programming error and panics. The first kit registered
// becomes the mandatory kit unless SetMandatory overrides it, so the common
// single-kit app needs no extra wiring.
func Register(k Kit) {
	if k == nil {
		panic("ui: Register(nil)")
	}
	name := k.Name()
	if name == "" {
		panic("ui: Register: kit has empty Name()")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("ui: Register: kit %q already registered", name))
	}
	registry[name] = k
	if mandatory == "" {
		mandatory = name
	}
}

// SetMandatory names which registered kit is mandatory — the one Mandatory()
// returns and the App builder injects when a request carries no explicit kit.
// It panics if no kit with that name is registered.
func SetMandatory(name string) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := registry[name]; !ok {
		panic(fmt.Sprintf("ui: SetMandatory: no kit named %q registered", name))
	}
	mandatory = name
}

// Mandatory returns the mandatory kit. It panics if no kit was ever registered:
// an app that forgot its side-effect import fails loudly at startup rather than
// rendering kit-less pages. This is the mandatory-kit enforcement.
func Mandatory() Kit {
	regMu.RLock()
	defer regMu.RUnlock()
	if mandatory == "" {
		panic("ui: no kit registered — add a blank import of a kit package, " +
			"e.g. import _ \"github.com/ThirdCoastInteractive/the-beach/ui/driftwood\"")
	}
	return registry[mandatory]
}

// Lookup returns the kit registered under name and whether it was found.
func Lookup(name string) (Kit, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	k, ok := registry[name]
	return k, ok
}

// Registered returns the names of all registered kits, sorted. It is for
// diagnostics (e.g. beach vet / a kit-picker).
func Registered() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// reset clears registry state. Test-only; not exported.
func reset() {
	regMu.Lock()
	defer regMu.Unlock()
	registry = map[string]Kit{}
	mandatory = ""
}
