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

// Swap builds the View for a page that participates in SPA-style navigation (the
// client half is in pkg/datastar: Navigate / PopstateNav / ActiveWhen). On a full
// page load the framework renders page (the whole document, shell included); on a
// Datastar nav it streams fragment into the target region's inner HTML under a
// browser view transition, so the persistent shell stays put and only the main
// content slides. target is the region element's id (without '#').
//
//	return beach.Swap("main", FullPage(v), pageBody(v)), nil
func Swap(target string, page, fragment templ.Component) View {
	return View{Page: page, Fragment: fragment, Target: target, Mode: PatchInner, ViewTransition: true}
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

	// Redirect, when non-empty, navigates the client to the given URL through the
	// Datastar SSE flow (a location script). It is the escape hatch for an action
	// that must redirect — a post-login bounce — without dropping to a Raw handler.
	// Combine it with SetHeader/SetCookie on the *Ctx to set a session cookie on
	// the same response. An empty Redirect is a no-op.
	Redirect string

	// Script, when non-empty, runs the given JavaScript in the client browser
	// (Datastar ExecuteScript). It is the escape hatch for an action that must
	// drive the client directly. An empty Script is a no-op.
	Script string

	// ViewTransition applies this patch inside the browser View Transitions API
	// so CSS ::view-transition-* rules animate the change.
	ViewTransition bool

	// Announce, when non-empty, appends the message to the page's live region
	// so a screen reader reads it out. It is how a server-driven change tells
	// someone who cannot see it that anything happened — "12 results", "saved",
	// "3 items removed" (WCAG 4.1.3 Status Messages).
	//
	// It needs the region to already exist, which driftwood.Shell renders on
	// every page; a page building its own shell renders driftwood.LiveRegion
	// itself. The message is text, not markup, and is appended rather than
	// replacing what is there, so a burst of updates is read in order.
	//
	// Use it for status, not for errors: a failure already streams a toast
	// through the same region, and Alert carries role=alert when something must
	// interrupt.
	Announce string
}

// Patches is the return type of an ActionFunc: an ordered list of DOM patches
// the framework streams to the client over a one-shot SSE response. Returning an
// empty Patches is valid (a mutation with no visible change).
type Patches []Patch
