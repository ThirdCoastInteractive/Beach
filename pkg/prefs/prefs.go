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

// The tokens stored in the cookie. The bare ones each name something switched
// *off*, so the absence of a cookie is a first visit rather than a visitor who
// refused everything; the one key=value token carries a choice that has no
// natural "off".
const (
	offLive  = "live"
	offToast = "toast"
)

// schemeKey prefixes the colour-scheme token. It is a key=value token rather than
// a bare one because a colour scheme is not a switch: "light" and "dark" are both
// choices, and neither is the absence of one.
const schemeKey = "scheme="

// Scheme is the visitor's colour-scheme choice.
//
// Three states, not two. A visitor has chosen light, has chosen dark, or has
// chosen nothing — and the third is not a default to be resolved server-side, it
// is a deferral to the operating system that only CSS can honour. Collapsing it
// to a boolean is what produces a site that ignores the OS setting.
type Scheme string

const (
	// SchemeAuto is the zero value: no choice, so prefers-color-scheme decides
	// and the page stamps no attribute at all.
	SchemeAuto  Scheme = ""
	SchemeLight Scheme = "light"
	SchemeDark  Scheme = "dark"
)

// Valid reports whether s is a scheme the framework recognises. An unrecognised
// value from a hand-edited cookie resolves to auto rather than to a broken
// attribute selector.
func (s Scheme) Valid() bool {
	return s == SchemeAuto || s == SchemeLight || s == SchemeDark
}

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

	// Scheme is the visitor's light/dark choice, or SchemeAuto for "ask the
	// operating system".
	//
	// It rides here rather than in a cookie of its own for one reason: the page
	// shell reads it during server render and stamps it on <html>, which is what
	// makes a chosen theme survive the first paint. The usual alternative — a
	// blocking <script> in the head that reads localStorage — is a render-
	// blocking script the framework's whole posture rules out, and it flashes
	// the wrong theme on every navigation when it fails.
	Scheme Scheme
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

// ColorScheme reports this request's visitor's colour-scheme choice.
func ColorScheme(ctx context.Context) Scheme { return From(ctx).Scheme }

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
		default:
			if v, ok := strings.CutPrefix(tok, schemeKey); ok {
				if sc := Scheme(v); sc.Valid() {
					p.Scheme = sc
				}
			}
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
	if p.Scheme != SchemeAuto && p.Scheme.Valid() {
		off = append(off, schemeKey+string(p.Scheme))
	}
	return strings.Join(off, ",")
}
