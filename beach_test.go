package beach

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// comp is a tiny templ component for tests.
func comp(html string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, html)
		return err
	})
}

// testApp builds an App wired with the no-op kit (no DB, no hub) for handler
// tests, and returns it plus its full http.Handler.
func testApp(t *testing.T) (*App, http.Handler) {
	t.Helper()
	a := New(Config{Service: "test", Kit: KitNoop()})
	return a, a.Handler()
}

func TestHealthz(t *testing.T) {
	_, h := testApp(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("healthz body = %q, want ok", got)
	}
}

func TestHealthzProbeFails(t *testing.T) {
	a := New(Config{
		Kit:     KitNoop(),
		Healthz: func(context.Context) error { return io.EOF },
	})
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("failing probe status = %d, want 503", rec.Code)
	}
}

func TestStartRefusesWithoutKit(t *testing.T) {
	a := New(Config{}) // no kit registered, no selector
	if a.kit != nil {
		t.Fatal("expected nil kit when none registered")
	}
	if err := a.Start(":0"); err == nil {
		t.Fatal("Start should refuse without a kit")
	}
}

func TestStaticServesWithETag(t *testing.T) {
	_, h := testApp(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/css/stellar.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("static status = %d, want 200", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag on static asset")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("Cache-Control = %q, want immutable", cc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("Content-Type = %q, want text/css", ct)
	}

	// Conditional request returns 304.
	req := httptest.NewRequest(http.MethodGet, "/static/css/stellar.css", nil)
	req.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", rec2.Code)
	}
}

func TestStaticMissingIs404(t *testing.T) {
	_, h := testApp(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/nope.css", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing static status = %d, want 404", rec.Code)
	}
}

func TestAssetURLBusts(t *testing.T) {
	a, _ := testApp(t)
	url := a.AssetURL("css/stellar.css")
	if !strings.HasPrefix(url, "/static/css/stellar.css?v=") {
		t.Fatalf("asset url = %q, want versioned /static path", url)
	}
}

func TestSecurityHeadersAndCSP(t *testing.T) {
	a := New(Config{
		Kit: KitNoop(),
		CSP: CSPDefault().AllowMedia("https://example.com"),
	})
	a.Page("/", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>hi</main>")}, nil
	})
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "media-src 'self' https://example.com") {
		t.Fatalf("CSP missing widened media-src: %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}
}

func TestPageFullDocument(t *testing.T) {
	a, h := testApp(t)
	a.Page("/p", func(c *Ctx) (View, error) {
		return View{Page: comp("<main id=\"doc\">full</main>")}, nil
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("page status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "full") {
		t.Fatalf("page body = %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("page content-type = %q", ct)
	}
}

func TestPageDatastarFragment(t *testing.T) {
	a, h := testApp(t)
	a.Page("/p", func(c *Ctx) (View, error) {
		return View{
			Page:     comp("<html>full</html>"),
			Fragment: comp("<div id=\"panel\">frag</div>"),
			Target:   "panel",
		}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "frag") {
		t.Fatalf("fragment body = %q, want frag patch", body)
	}
	if strings.Contains(body, "full") {
		t.Fatalf("datastar branch leaked full document: %q", body)
	}
}

func TestActionRejectsNavigation(t *testing.T) {
	a, h := testApp(t)
	a.Action("/act", func(c *Ctx) (Patches, error) {
		return Patches{{Fragment: comp("<div id=\"x\">ok</div>"), Target: "x"}}, nil
	})
	// Plain POST without Datastar header -> 404.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/act", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nav-to-action status = %d, want 404", rec.Code)
	}
}

func TestActionDatastarPatches(t *testing.T) {
	a, h := testApp(t)
	a.Action("/act", func(c *Ctx) (Patches, error) {
		return Patches{{Fragment: comp("<div id=\"x\">done</div>"), Target: "x"}}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/act", nil)
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "done") {
		t.Fatalf("action body = %q, want patch", rec.Body.String())
	}
}

func TestPageErrorNotFound(t *testing.T) {
	a, h := testApp(t)
	a.Page("/missing", func(c *Ctx) (View, error) {
		return View{}, ErrNotFound
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("error page status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Not found") {
		t.Fatalf("error page body = %q", rec.Body.String())
	}
}

func TestAuthedGuardBlocksAnonymous(t *testing.T) {
	a, h := testApp(t)
	a.Page("/wallet", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>secret</main>")}, nil
	}, a.Authed())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/wallet", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("authed guard status = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("guard let anonymous through")
	}
}

func TestRawEscapeHatch(t *testing.T) {
	a, h := testApp(t)
	a.Raw(http.MethodPost, "/webhooks/stripe", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("raw"))
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/webhooks/stripe", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("raw status = %d, want 202", rec.Code)
	}
	if rec.Body.String() != "raw" {
		t.Fatalf("raw body = %q", rec.Body.String())
	}
}

func TestRecoverPanic(t *testing.T) {
	a := New(Config{Kit: KitNoop(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	h := a.Handler()
	a.Page("/boom", func(c *Ctx) (View, error) {
		panic("kaboom")
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("recovered panic status = %d, want 500", rec.Code)
	}
}
