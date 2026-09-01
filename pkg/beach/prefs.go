package beach

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/ThirdCoastInteractive/Beach/pkg/prefs"
)

// The HTTP half of visitor timing preferences: the cookie, the middleware that
// resolves it onto every request, and the route that writes it. The preference
// itself, and the readers views use, live in [prefs] — the component kit has to
// read it and cannot import this package.

// Prefs re-exports the preference type so a handler names one beach.Prefs rather
// than reaching across packages for a two-field struct.
type Prefs = prefs.Prefs

// PrefsCookie is the cookie carrying a visitor's timing preferences.
const PrefsCookie = prefs.Cookie

// PrefsPath is the framework-owned route that writes that cookie. It is
// registered by New alongside /healthz.
const PrefsPath = prefs.Path

// PrefsFrom reads the visitor's preferences from ctx, defaulting to everything
// on.
func PrefsFrom(ctx context.Context) Prefs { return prefs.From(ctx) }

// LiveUpdates reports whether this request's visitor still wants server-pushed
// updates. It is the reader a StreamFunc uses when it wants to branch itself;
// the framework already applies it to every stream, so most handlers never need
// to call it.
func LiveUpdates(ctx context.Context) bool { return prefs.LiveUpdates(ctx) }

// AutoDismiss reports whether notifications may still expire on a timer.
func AutoDismiss(ctx context.Context) bool { return prefs.AutoDismiss(ctx) }

// ColorScheme reports the visitor's light/dark choice, or prefs.SchemeAuto when
// they have expressed none and the operating system should decide.
func ColorScheme(ctx context.Context) prefs.Scheme { return prefs.ColorScheme(ctx) }

// prefsMiddleware resolves the preference cookie onto every request's context.
// It sits in the fixed stack: the cost is one cookie read, and making it opt-in
// would mean an app that forgets it silently loses two Level A criteria.
func (a *App) prefsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := prefs.Default()
		if ck, err := r.Cookie(PrefsCookie); err == nil {
			p = prefs.Parse(ck.Value)
		}
		next.ServeHTTP(w, r.WithContext(prefs.With(r.Context(), p)))
	})
}

// handlePrefs writes the preference cookie and sends the visitor back where they
// came from.
//
// It is a plain form post and a redirect, not a Datastar action, and that is the
// load-bearing choice. Pausing has to actually stop the stream, and the only way
// to be certain an already-open SSE connection is gone is to replace the page
// holding it: a navigation closes it, and the page that comes back does not
// contain the element that would open another. Doing it in place would depend on
// the client library aborting an in-flight request when its element is swapped —
// which it appears to do, but "appears to" is not a basis for a Level A
// mechanism. The locale switcher already works this way, so it is one idiom and
// not two.
func (a *App) handlePrefs(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	p := prefs.Default()
	if ck, err := r.Cookie(PrefsCookie); err == nil {
		p = prefs.Parse(ck.Value)
	}
	// Each field is read only when present, so one control can flip one
	// preference without resetting the other.
	if v := r.PostForm.Get("live"); v != "" {
		p.LiveUpdates = v == "on"
	}
	if v := r.PostForm.Get("toast"); v != "" {
		p.AutoDismiss = v == "on"
	}
	// The scheme is tri-state, so "auto" is a value a control can post rather
	// than an absence it has to express by omitting the field.
	if v := r.PostForm.Get("scheme"); v != "" {
		if sc := prefs.Scheme(v); sc.Valid() {
			p.Scheme = sc
		} else if v == "auto" {
			p.Scheme = prefs.SchemeAuto
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     PrefsCookie,
		Value:    p.String(),
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: false, // carries no identity; a page may read its own setting
		SameSite: http.SameSiteLaxMode,
		Secure:   a.cfg.Release,
	})
	http.Redirect(w, r, prefsReturn(r), http.StatusSeeOther)
}

// prefsReturn picks where to send the visitor after a preference change: back to
// the page they were on. The Referer is attacker-influenced, so only its path
// and query are used, and only when it is same-origin — echoing an absolute URL
// would turn this route into an open redirect.
func prefsReturn(r *http.Request) string {
	ref := r.Header.Get("Referer")
	if ref == "" {
		return "/"
	}
	u, err := url.Parse(ref)
	if err != nil || !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	if u.Host != "" && u.Host != r.Host {
		return "/"
	}
	out := u.Path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}
