package datastar

// SPA-style navigation primitives. A Beach app gets history-synced, view-
// transitioned content swaps without hand-rolling pushState/popstate.
//
// The model: one persistent shell with a swappable main region; a "$path" signal
// holds the current path. A nav link uses Navigate (prevent default, fetch the
// route over Datastar so the server streams the new content into the region under
// a view transition, set $path, push the URL). The shell carries PathSignal (init
// $path) and PopstateNav (re-fetch + resync on browser back/forward). Links mark
// themselves current with ActiveWhen. The server half is beach.Swap, which builds
// the View that renders the full document on a load and the region fragment on a
// Datastar nav.

import "strings"

// join appends extra statements (app-specific, e.g. closing a sidebar) to a base
// Datastar expression, separated by ";".
func join(base string, extra []string) string {
	parts := append([]string{base}, nonEmpty(extra)...)
	return strings.Join(parts, "; ")
}

func nonEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Navigate is the click handler for a SPA navigation link. It prevents the
// browser's default navigation, fetches href over Datastar (the page handler
// streams the new content into the swap region under a view transition), sets the
// $path signal so the active link updates, and pushes the URL so back/forward
// work. extra appends app statements (e.g. "$navOpen = false").
//
//	Navigate("/topology") ->
//	  data-on:click__prevent="@get('/topology'); $path = '/topology'; globalThis.history.pushState(null, '', '/topology')"
func Navigate(href string, extra ...string) Attr {
	base := "@get('" + href + "'); $path = '" + href + "'; globalThis.history.pushState(null, '', '" + href + "')"
	return On("click__prevent", join(base, extra))
}

// PopstateNav is the window popstate handler for browser back/forward: it
// re-fetches the now-current URL (the server swaps the content) and resyncs
// $path. It does NOT push history — the entry already changed. Put it on a
// persistent element (the shell). extra appends app statements.
func PopstateNav(extra ...string) Attr {
	base := "@get(globalThis.location.pathname + globalThis.location.search); $path = globalThis.location.pathname"
	return Attr{Name: "data-on:popstate__window", Val: join(base, extra)}
}

// ActiveWhen toggles class on the element when the $path signal equals href —
// the reactive "current page" marker for a nav link (no re-render needed).
//
//	ActiveWhen("active", "/topology") -> data-class:active="$path === '/topology'"
func ActiveWhen(class, href string) Attr {
	return ClassToggle(class, "$path === '"+href+"'")
}

// PathSignal initialises the $path signal from the current location. Add it to
// the element that declares the app's signals (the shell), so deep links and
// reloads mark the right nav link active.
//
//	PathSignal() -> data-signals:path="globalThis.location.pathname"
func PathSignal() Attr {
	return Attr{Name: "data-signals:path", Val: "globalThis.location.pathname"}
}
