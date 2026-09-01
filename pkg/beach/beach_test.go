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

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
)

// comp is a tiny templ component for tests: trusted literal markup.
func comp(html string) templ.Component {
	return templ.Raw(html)
}

// testApp builds a bare App (no DB, no hub) for handler tests, and returns it
// plus its full http.Handler.
func testApp(t *testing.T) (*App, http.Handler) {
	t.Helper()
	a := New(Config{Service: "test"})
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
		Healthz: func(context.Context) error { return io.EOF },
	})
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("failing probe status = %d, want 503", rec.Code)
	}
}

func TestStaticServesWithETag(t *testing.T) {
	_, h := testApp(t)

	// A versioned request (how the server references assets) is content-addressed
	// and may be cached immutably for a year.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/css/app.css?v=abc", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("static status = %d, want 200", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag on static asset")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("versioned Cache-Control = %q, want immutable", cc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("Content-Type = %q, want text/css", ct)
	}

	// A bare request (how the browser fetches ES module sub-imports) must
	// revalidate, not be pinned immutable, so a deploy is picked up.
	recBare := httptest.NewRecorder()
	h.ServeHTTP(recBare, httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil))
	if cc := recBare.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Fatalf("bare Cache-Control = %q, want revalidate (no-cache)", cc)
	}
	if recBare.Header().Get("ETag") == "" {
		t.Fatal("missing ETag on bare static request")
	}

	// Conditional request returns 304 regardless of versioning.
	req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
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
	url := a.AssetURL("css/app.css")
	if !strings.HasPrefix(url, "/static/css/app.css?v=") {
		t.Fatalf("asset url = %q, want versioned /static path", url)
	}
}

func TestSecurityHeadersAndCSP(t *testing.T) {
	a := New(Config{
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

func TestTokenAuthedRejectsMissingToken(t *testing.T) {
	// A resolver is configured, but the request carries no Authorization header.
	a := New(Config{
		Service: "test",
		Tokens: func(context.Context, string) (*Principal, error) {
			return &Principal{UserID: 1}, nil
		},
	})
	h := a.Handler()
	a.Page("/api", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>secret</main>")}, nil
	}, a.TokenAuthed())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("guard let a token-less request through")
	}
}

func TestTokenAuthedRejectsInvalidToken(t *testing.T) {
	a := New(Config{
		Service: "test",
		Tokens: func(context.Context, string) (*Principal, error) {
			return nil, auth.ErrInvalidToken
		},
	})
	h := a.Handler()
	a.Page("/api", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>secret</main>")}, nil
	}, a.TokenAuthed())

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer bogus.token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want 401", rec.Code)
	}
}

func TestTokenAuthedNoResolverRejects(t *testing.T) {
	// No Tokens resolver configured: every token request is rejected (safe fail).
	a, h := testApp(t)
	a.Page("/api", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>secret</main>")}, nil
	}, a.TokenAuthed())

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer anything.here")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-resolver status = %d, want 401", rec.Code)
	}
}

func TestTokenAuthedPassesAndAttachesPrincipal(t *testing.T) {
	want := &Principal{UserID: 42, Username: "svc", Permissions: []string{"fleet:read"}}
	a := New(Config{
		Service: "test",
		Tokens: func(_ context.Context, raw string) (*Principal, error) {
			if raw != "pfx.secret" {
				return nil, auth.ErrInvalidToken
			}
			return want, nil
		},
	})
	h := a.Handler()
	var seen *Principal
	a.Page("/api", func(c *Ctx) (View, error) {
		seen, _ = c.Principal()
		return View{Page: comp("<main>ok</main>")}, nil
	}, a.TokenAuthed())

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer pfx.secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token status = %d, want 200", rec.Code)
	}
	if seen != want {
		t.Fatalf("handler saw principal %+v, want the resolved token principal", seen)
	}
}

func TestTokenAuthedComposesWithCan(t *testing.T) {
	// TokenAuthed attaches a principal Can(...) can then enforce on the same route.
	a := New(Config{
		Service: "test",
		Tokens: func(context.Context, string) (*Principal, error) {
			return &Principal{UserID: 7, Permissions: []string{"fleet:read"}}, nil
		},
	})
	h := a.Handler()
	a.Page("/api", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>secret</main>")}, nil
	}, a.TokenAuthed(), a.Can("fleet:write"))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer pfx.secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("token principal lacking permission status = %d, want 403", rec.Code)
	}
}

func TestInScopeGuard(t *testing.T) {
	mkApp := func(p *Principal) (*App, http.Handler) {
		a := New(Config{
			Service: "test",
			Tokens:  func(context.Context, string) (*Principal, error) { return p, nil },
		})
		h := a.Handler()
		a.Page("/row", func(c *Ctx) (View, error) {
			return View{Page: comp("<main>row</main>")}, nil
		}, a.TokenAuthed(), a.InScope("cust-1"))
		return a, h
	}

	cases := []struct {
		name string
		p    *Principal
		want int
	}{
		{"in scope", &Principal{UserID: 1, Scope: "cust-1"}, http.StatusOK},
		{"wrong scope", &Principal{UserID: 1, Scope: "cust-2"}, http.StatusForbidden},
		{"unscoped", &Principal{UserID: 1, Scope: ""}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, h := mkApp(tc.p)
			req := httptest.NewRequest(http.MethodGet, "/row", nil)
			req.Header.Set("Authorization", "Bearer pfx.secret")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%s: status = %d, want %d", tc.name, rec.Code, tc.want)
			}
		})
	}
}

func TestInScopeRejectsAnonymous(t *testing.T) {
	// InScope on a route with no principal in context (anonymous) is a 403.
	a, h := testApp(t)
	a.Page("/row", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>row</main>")}, nil
	}, a.InScope("cust-1"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/row", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous in-scope status = %d, want 403", rec.Code)
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
		wantOK bool
	}{
		{"Bearer abc.def", "abc.def", true},
		{"bearer abc.def", "abc.def", true}, // scheme is case-insensitive
		{"Bearer   spaced  ", "spaced", true},
		{"Basic abc", "", false},
		{"Bearer ", "", false},
		{"Bearer", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if tt.header != "" {
			req.Header.Set("Authorization", tt.header)
		}
		got, ok := bearerToken(req)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("bearerToken(%q) = (%q, %v), want (%q, %v)", tt.header, got, ok, tt.want, tt.wantOK)
		}
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
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
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
