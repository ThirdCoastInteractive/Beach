// Package prefs carries a visitor's answer to "may this page keep changing on
// its own?" from the request that resolved it to the view that has to obey it.
//
// Two WCAG criteria ask for a control that the framework, not each app, is best
// placed to provide:
//
//   - SC 2.2.2 Pause, Stop, Hide (A) names *auto-updating information*
//     explicitly. Server-pushed updates are this framework's whole premise, so
//     every streaming page is in scope and the mechanism to stop them has to
//     live somewhere shared.
//   - SC 2.2.1 Timing Adjustable (A) asks that a time limit be turnable off
//     *before it is encountered*. A notification that fades on a timer is a
//     time limit.
//
// It is its own package for the same reason i18n is: the HTTP layer resolves the
// preference from a cookie and the component kit has to read it, and the kit
// cannot import the HTTP layer. Nothing here knows about net/http — beach owns
// the cookie and the route, driftwood owns the control.
//
// A preference like this is not identity. No id is minted, nothing is stored
// server-side, and the cookie holds only what the visitor switched off, so a
// visitor at the defaults carries an empty value.
package prefs

import (
	"context"
	"strings"
)

// Cookie is the cookie name the preference travels in.
const Cookie = "beach-prefs"

// Path is the framework-owned route that writes the cookie. The underscore
// prefix marks it as the framework's rather than an app's.
const Path = "/_beach/prefs"

// The tokens stored in the cookie. Each names something switched *off*, so the
// absence of a cookie is a first visit rather than a visitor who refused
// everything.
const (
	offLive  = "live"
	offToast = "toast"
)

// Prefs is what a visitor has asked this page to stop doing.
//
// The zero value is deliberately *not* the default — use [Default] or [From].
// A zero Prefs reads as "turn everything off", which is the opposite of what a
// visitor who has said nothing wants, and is exactly the mistake a struct with
// two bools invites.
type Prefs struct {
	// LiveUpdates false means the visitor paused server-pushed updates. The
	// framework honours it in the stream adapter, so it holds even for an app
	// that never renders a control (SC 2.2.2).
	LiveUpdates bool

	// AutoDismiss false means notifications must not expire on a timer. This is
	// SC 2.2.1's first bullet — the limit is turned off before it is
	// encountered — and it is what makes an on-by-default toast timer
	// defensible rather than a violation.
	AutoDismiss bool
}

// Default is what a visitor who has expressed no preference gets: content
// behaves normally.
func Default() Prefs { return Prefs{LiveUpdates: true, AutoDismiss: true} }

// ctxKey is the unexported context key the resolved preference is stored under.
type ctxKey struct{}

// With returns a copy of ctx carrying p.
func With(ctx context.Context, p Prefs) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// From reads the visitor's preferences from ctx, falling back to [Default].
// A view rendered outside the middleware — a test, a background render — gets
// the defaults rather than a zero value that would read as a total opt-out.
func From(ctx context.Context) Prefs {
	if ctx == nil {
		return Default()
	}
	if p, ok := ctx.Value(ctxKey{}).(Prefs); ok {
		return p
	}
	return Default()
}

// LiveUpdates reports whether this request's visitor still wants server-pushed
// updates.
func LiveUpdates(ctx context.Context) bool { return From(ctx).LiveUpdates }

// AutoDismiss reports whether notifications may still expire on a timer.
func AutoDismiss(ctx context.Context) bool { return From(ctx).AutoDismiss }

// Parse reads a cookie value: a comma-separated list of switched-off features.
// An unrecognised token is ignored, so a cookie written by an older or newer
// build degrades to the default instead of to nonsense.
func Parse(v string) Prefs {
	p := Default()
	for _, tok := range strings.Split(v, ",") {
		switch strings.TrimSpace(tok) {
		case offLive:
			p.LiveUpdates = false
		case offToast:
			p.AutoDismiss = false
		}
	}
	return p
}

// String renders Prefs back into a cookie value, listing only what is switched
// off.
func (p Prefs) String() string {
	var off []string
	if !p.LiveUpdates {
		off = append(off, offLive)
	}
	if !p.AutoDismiss {
		off = append(off, offToast)
	}
	return strings.Join(off, ",")
}
