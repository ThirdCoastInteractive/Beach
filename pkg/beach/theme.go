package beach

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/ThirdCoastInteractive/Beach/pkg/beach/view"
	"github.com/ThirdCoastInteractive/Beach/pkg/theme"
)

// Runtime theming.
//
// The stylesheet the framework serves carries every *rule*; this route carries
// the ~40 *values* those rules reference. That split is what the design-token
// indirection was always for, and it is what makes a theme swappable without
// rebuilding: app.css is a build artefact, and the block below is a string a
// request can produce.
//
// It is a route rather than an inline <style> in every page for two reasons. It
// caches — one fetch serves every page in the session, where inlining pays the
// bytes on every response. And it invalidates precisely: the ETag is a hash of
// the derived tokens, so a theme change busts exactly this file and nothing else.

// ThemePath is the framework-owned route serving the derived design tokens. The
// underscore prefix marks it as the framework's rather than an app's, the same
// way prefs.Path does.
const ThemePath = "/_beach/theme.css"

// ThemeSource resolves the theme for a running app. It is called at boot and on
// [App.ReloadTheme], never per request.
//
// It is a function rather than a table because the framework does not own one.
// It owns tables for auth and sessions and nothing else, and a resolver lets an
// app back its theme with a row, a file, or an environment variable — exactly as
// it already does for principals via Config.Principals.
type ThemeSource func(ctx context.Context) (theme.Theme, error)

// renderedTheme is a derived theme and the two strings a request needs, computed
// once so serving one is a write rather than a derivation.
type renderedTheme struct {
	css  string
	etag string
}

// resolveTheme derives the app's theme, in the order a configured value should
// win: an explicit theme, then a source, then the preset the framework ships.
func (a *App) resolveTheme(ctx context.Context) (theme.Theme, error) {
	switch {
	case a.cfg.Theme != nil:
		return *a.cfg.Theme, nil
	case a.cfg.ThemeSource != nil:
		return a.cfg.ThemeSource(ctx)
	default:
		return theme.BuildPreset(view.ThemePreset)
	}
}

// loadTheme derives and caches. A failure leaves whatever was already cached in
// place and returns the error: a bad row in someone's theme table should not take
// a running site's colours away, and a site that keeps its previous palette is a
// far better failure than one that renders unstyled.
func (a *App) loadTheme(ctx context.Context) error {
	t, err := a.resolveTheme(ctx)
	if err != nil {
		return err
	}
	css := t.CSS()
	sum := sha256.Sum256([]byte(css))
	a.theme.Store(&renderedTheme{
		css:  css,
		etag: `"` + hex.EncodeToString(sum[:8]) + `"`,
	})
	return nil
}

// ReloadTheme re-resolves the theme and swaps it in for subsequent requests.
//
// An app calls it after writing a new theme — from an admin screen, or from a
// pg.Notify listener when another process wrote one. The swap is atomic, so a
// request either serves the whole old theme or the whole new one, never a mix.
func (a *App) ReloadTheme(ctx context.Context) error { return a.loadTheme(ctx) }

// handleTheme serves the derived tokens.
func (a *App) handleTheme(w http.ResponseWriter, r *http.Request) {
	rt, _ := a.theme.Load().(*renderedTheme)
	if rt == nil {
		// Boot derives eagerly, so this is only reachable if that failed — in
		// which case serving the shipped preset is better than serving nothing,
		// because nothing means a page with no colours at all.
		if err := a.loadTheme(r.Context()); err != nil {
			http.Error(w, "theme unavailable", http.StatusInternalServerError)
			return
		}
		rt = a.theme.Load().(*renderedTheme)
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("ETag", rt.etag)
	// Revalidate rather than cache hard. The palette is small and can change
	// while a visitor is on the site, so a conditional request that almost
	// always answers 304 costs less than a stale theme that outlives its change.
	w.Header().Set("Cache-Control", "no-cache")
	if match := r.Header.Get("If-None-Match"); match == rt.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	io.WriteString(w, rt.css)
}
