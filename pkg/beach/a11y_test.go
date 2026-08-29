package beach

// Framework-level accessibility behaviour: the two things the HTTP layer owns
// rather than the kit — announcing a change, and declaring the language it was
// rendered in. See docs/rfc/06-accessibility.md.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ThirdCoastInteractive/the-beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ui/driftwood"
)

// TestPatchAnnounceTargetsTheLiveRegion is the server half of WCAG 4.1.3.
//
// The message has to be *appended into* the region the page already shipped —
// not sent as a replacement for it, and not to some other id. A patch that
// replaced the region would take the region's identity with it and the next
// message would go nowhere; one aimed elsewhere is simply never announced.
func TestPatchAnnounceTargetsTheLiveRegion(t *testing.T) {
	a, h := testApp(t)
	a.Action("/filter", func(c *Ctx) (Patches, error) {
		return Patches{{Announce: "Twelve results."}}, nil
	})
	body := datastarPost(t, h, "/filter")

	if !strings.Contains(body, "Twelve results.") {
		t.Fatalf("announce body = %q, want the message", body)
	}
	if !strings.Contains(body, "selector #"+ToastTarget) {
		t.Fatalf("announce body = %q, want it aimed at #%s", body, ToastTarget)
	}
	if !strings.Contains(body, "mode append") {
		t.Fatalf("announce body = %q, want append — replacing the region destroys the thing being announced into", body)
	}
	// Screen-reader-only: the region is how a change reaches someone who cannot
	// see it, not a second place to put visible text.
	if !strings.Contains(body, "sr-only") {
		t.Fatalf("announce body = %q, want the message screen-reader-only", body)
	}
}

// TestToastTargetIsTheKitsRegion pins the alias. The kit renders the element, so
// the kit owns the id; beach.ToastTarget naming a different string would mean
// every announcement missed the region the shell shipped.
func TestToastTargetIsTheKitsRegion(t *testing.T) {
	if ToastTarget != driftwood.ToastTarget {
		t.Fatalf("beach.ToastTarget = %q, driftwood.ToastTarget = %q — announcements would miss the rendered region",
			ToastTarget, driftwood.ToastTarget)
	}
}

// TestConfigLocalesResolvesRequestLanguage checks the wiring that makes
// <html lang> follow the request (WCAG 3.1.1): Config.Locales installs the
// middleware, and the locale reaches the handler's context — which is the
// context the page shell renders with.
func TestConfigLocalesResolvesRequestLanguage(t *testing.T) {
	cat, err := i18n.Load(fstest.MapFS{
		"locales/en-US.json": &fstest.MapFile{Data: []byte(`{"k":"en"}`)},
		"locales/es-ES.json": &fstest.MapFile{Data: []byte(`{"k":"es"}`)},
	}, "en-US")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	seen := func(t *testing.T, cfg Config, req *http.Request) (locale, msg string) {
		t.Helper()
		a := New(cfg)
		a.Raw(http.MethodGet, "/probe", func(w http.ResponseWriter, r *http.Request) {
			locale = i18n.Locale(r.Context())
			msg = i18n.T(r.Context(), "k")
		})
		a.Handler().ServeHTTP(httptest.NewRecorder(), req)
		return locale, msg
	}

	t.Run("cookie wins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.AddCookie(&http.Cookie{Name: i18n.CookieName, Value: "es-ES"})
		req.Header.Set("Accept-Language", "en-US")
		if loc, msg := seen(t, Config{Locales: cat}, req); loc != "es-ES" || msg != "es" {
			t.Errorf("cookie locale = %q/%q, want es-ES/es", loc, msg)
		}
	})

	t.Run("accept-language when no cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.Header.Set("Accept-Language", "es-ES,en;q=0.5")
		if loc, _ := seen(t, Config{Locales: cat}, req); loc != "es-ES" {
			t.Errorf("Accept-Language locale = %q, want es-ES", loc)
		}
	})

	t.Run("inert when unconfigured", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.Header.Set("Accept-Language", "es-ES")
		// No Locales: the app is monolingual and pays nothing — no locale on the
		// context, and T falls through to the framework's embedded catalog.
		if loc, _ := seen(t, Config{}, req); loc != "" {
			t.Errorf("unconfigured app resolved a locale %q, want none", loc)
		}
	})
}

// TestErrorPageSpeaksTheRequestLanguage covers the surface most likely to be
// forgotten: a page rendered by the framework rather than by the app. An error
// page in English under lang="es-ES" makes the document's own declaration a lie.
func TestErrorPageSpeaksTheRequestLanguage(t *testing.T) {
	cat, err := i18n.Load(fstest.MapFS{
		"locales/en-US.json": &fstest.MapFile{Data: []byte(`{"framework.error.not_found":"Not found"}`)},
		"locales/es-ES.json": &fstest.MapFile{Data: []byte(`{"framework.error.not_found":"No encontrado"}`)},
	}, "en-US")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ctx := i18n.WithCatalog(i18n.WithLocale(context.Background(), "es-ES"), cat)
	if got := errorTitle(ctx, http.StatusNotFound); got != "No encontrado" {
		t.Errorf("404 title under es-ES = %q, want the Spanish string", got)
	}
	if got := errorTitle(context.Background(), http.StatusNotFound); got != "Not found" {
		t.Errorf("404 title with no locale = %q, want the framework default", got)
	}
}
