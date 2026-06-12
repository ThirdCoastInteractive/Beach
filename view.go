package beach

import (
	"github.com/a-h/templ"
)

// View is what a PageFunc returns: a description that knows how to render both
// ways. On a normal navigation the framework writes Page (the full document);
// on a Datastar request it patches Fragment into Target. A separate fragment
// route has nothing to attach to — the dual-purpose branch is the framework's,
// not the handler's, which is the whole point of the type.
type View struct {
	// Page is the full HTML document rendered on navigation (a normal GET). It is
	// subject to the 14KB rule; heavier content ships as a deferred section.
	Page templ.Component

	// Fragment is the component patched in on a Datastar request. When nil, the
	// framework patches Page instead (a page whose fragment is the whole document
	// — fine for small pages, though most set an explicit Fragment + Target).
	Fragment templ.Component

	// Target is the DOM id the Fragment patches into (without the leading '#').
	// Empty means "patch by the fragment's own element id" (Datastar's default
	// outer-morph), which is the common case when the fragment carries its id.
	Target string

	// Mode overrides the patch mode for the Datastar branch (replace, inner,
	// append, ...). Zero value uses the framework default (outer morph).
	Mode PatchMode

	// Status overrides the HTTP status for the navigation branch (e.g. 404 for a
	// not-found page rendered as a real page). Zero means 200.
	Status int

	// ViewTransition, on the Datastar branch, applies the patch inside the browser
	// View Transitions API (document.startViewTransition), so CSS
	// ::view-transition-* rules animate the change. No effect on navigation.
	ViewTransition bool
}

// PatchMode selects how a fragment is merged into the DOM. It mirrors the
// Datastar element-patch modes; the zero value is the default outer morph.
type PatchMode string

const (
	// PatchOuter morphs the target element itself (default).
	PatchOuter PatchMode = "outer"
	// PatchInner replaces the target's inner HTML.
	PatchInner PatchMode = "inner"
	// PatchReplace hard-replaces the target element.
	PatchReplace PatchMode = "replace"
	// PatchPrepend inserts the fragment as the target's first children.
	PatchPrepend PatchMode = "prepend"
	// PatchAppend inserts the fragment as the target's last children.
	PatchAppend PatchMode = "append"
	// PatchBefore inserts the fragment before the target.
	PatchBefore PatchMode = "before"
	// PatchAfter inserts the fragment after the target.
	PatchAfter PatchMode = "after"
	// PatchRemove removes the target element.
	PatchRemove PatchMode = "remove"
)

// Patch is a single fragment-to-target update. It is the unit an ActionFunc
// returns (as Patches) and the unit a StreamFunc's catch-up pushes. A Patch with
// a nil Fragment and PatchRemove mode removes the Target.
type Patch struct {
	// Fragment is the component to render and patch in.
	Fragment templ.Component

	// Target is the DOM id to patch into (without '#'). Empty patches by the
	// fragment's own element id.
	Target string

	// Mode selects the merge mode; zero is the default outer morph.
	Mode PatchMode

	// Signals, when non-nil, JSON-marshals to a data-signals patch sent alongside
	// the element patch (e.g. to clear a form's input signals after a submit).
	Signals any

	// ViewTransition applies this patch inside the browser View Transitions API
	// so CSS ::view-transition-* rules animate the change.
	ViewTransition bool
}

// Patches is the return type of an ActionFunc: an ordered list of DOM patches
// the framework streams to the client over a one-shot SSE response. Returning an
// empty Patches is valid (a mutation with no visible change).
type Patches []Patch
