package beach

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/Beach/pkg/theme"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"
)

// TestThemeHrefMatchesTheRoute holds the two halves of the same path together.
//
// The kit cannot import the HTTP layer, so the page shell spells the URL out and
// the router spells it out again. Two literals that must agree and cannot see
// each other is exactly the shape that silently drifts — and the failure would
// be a 404 on the stylesheet, which renders a page with no colours at all.
func TestThemeHrefMatchesTheRoute(t *testing.T) {
	if driftwood.ThemeHref != ThemePath {
		t.Errorf("the page links %q but the app serves %q", driftwood.ThemeHref, ThemePath)
	}
}

func TestThemeRouteServesTheTokens(t *testing.T) {
	a := New(Config{})
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ThemePath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", ThemePath, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}
	body := rec.Body.String()
	for _, name := range theme.TokenNames() {
		if !strings.Contains(body, name+":") {
			t.Errorf("served theme is missing %s", name)
		}
	}
	// Both schemes, or a visitor whose OS asks for light gets the dark one.
	for _, want := range []string{
		"@media (prefers-color-scheme: light)",
		`:root[data-theme="light"]`,
		`:root[data-theme="dark"]`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("served theme is missing %q", want)
		}
	}
}

// TestThemeRevalidates checks the conditional request path. The palette can
// change while a visitor is on the site, so it revalidates rather than caching
// hard — which is only worth doing if the 304 actually happens.
func TestThemeRevalidates(t *testing.T) {
	a := New(Config{})
	h := a.Handler()

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, ThemePath, nil))
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the theme response")
	}

	req := httptest.NewRequest(http.MethodGet, ThemePath, nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	h.ServeHTTP(second, req)
	if second.Code != http.StatusNotModified {
		t.Errorf("conditional GET = %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried %d bytes of body", second.Body.Len())
	}
}

// TestThemeSourceOverridesThePreset is the runtime-theming path: an app that
// stores its palette gets it served, without rebuilding the framework.
func TestThemeSourceOverridesThePreset(t *testing.T) {
	custom, err := theme.BuildPreset("redtide")
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{ThemeSource: func(context.Context) (theme.Theme, error) {
		return custom, nil
	}})
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ThemePath, nil))

	want := custom.Dark.Tokens()["--color-accent"]
	if !strings.Contains(rec.Body.String(), want) {
		t.Errorf("served theme does not carry the source's accent %s", want)
	}
}

// TestABrokenThemeSourceStillServesColour is the failure posture. A site whose
// theme row is malformed should keep working in the framework's own colours; a
// page with no palette at all is a far worse outcome than a page in the wrong
// one, and it is the one an app author is least able to debug.
func TestABrokenThemeSourceStillServesColour(t *testing.T) {
	a := New(Config{ThemeSource: func(context.Context) (theme.Theme, error) {
		return theme.Theme{}, errors.New("no such theme row")
	}})
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ThemePath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200 on the shipped preset", ThemePath, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "--color-accent:") {
		t.Error("fell back to a theme with no accent")
	}
}

// TestReloadThemeSwapsIt covers the invalidation path an app uses after writing
// a new theme — directly, or from a pg.Notify listener when another process did.
func TestReloadThemeSwapsIt(t *testing.T) {
	key := "driftwood"
	a := New(Config{ThemeSource: func(context.Context) (theme.Theme, error) {
		return theme.BuildPreset(key)
	}})
	h := a.Handler()

	before := httptest.NewRecorder()
	h.ServeHTTP(before, httptest.NewRequest(http.MethodGet, ThemePath, nil))
	beforeETag := before.Header().Get("ETag")

	key = "redtide"
	if err := a.ReloadTheme(context.Background()); err != nil {
		t.Fatal(err)
	}

	after := httptest.NewRecorder()
	h.ServeHTTP(after, httptest.NewRequest(http.MethodGet, ThemePath, nil))
	if after.Header().Get("ETag") == beforeETag {
		t.Error("the ETag did not change across a reload, so caches would keep the old palette")
	}
	want, _ := theme.BuildPreset("redtide")
	if !strings.Contains(after.Body.String(), want.Dark.Tokens()["--color-accent"]) {
		t.Error("reload did not swap the served palette")
	}
}
