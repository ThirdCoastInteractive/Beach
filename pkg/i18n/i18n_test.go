package i18n

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// testCat builds a small in-memory catalog: en-US (default) and fr-FR, with a
// key present only in en-US to exercise fallback.
func testCat(t *testing.T) *Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"catalog.json": {Data: []byte(`{
			"greeting": {"label": "Hello", "comment": "greeting"},
			"cart.count": {"label": "%d items", "comment": "cart"},
			"only.en": {"label": "English only", "comment": ""}
		}`)},
		"locales/en-US.json": {Data: []byte(`{
			"greeting": "Hello",
			"cart.count": "%d items in cart",
			"only.en": "English only"
		}`)},
		"locales/fr-FR.json": {Data: []byte(`{
			"greeting": "Bonjour",
			"cart.count": "%d articles dans le panier"
		}`)},
	}
	c, err := Load(fsys, "en-US")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func TestLookup(t *testing.T) {
	c := testCat(t)
	tests := []struct {
		name   string
		locale string
		key    string
		want   string
		wantOK bool
	}{
		{"exact en", "en-US", "greeting", "Hello", true},
		{"exact fr", "fr-FR", "greeting", "Bonjour", true},
		{"fallback to default", "fr-FR", "only.en", "English only", true},
		{"missing everywhere", "en-US", "no.such.key", "no.such.key", false},
		{"empty locale uses default", "", "greeting", "Hello", true},
		{"case-insensitive tag", "EN-us", "greeting", "Hello", true},
		{"primary language match", "fr", "greeting", "Bonjour", true},
		{"unknown locale falls back", "de-DE", "greeting", "Hello", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := c.Lookup(tt.locale, tt.key)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("Lookup(%q,%q) = (%q,%v), want (%q,%v)",
					tt.locale, tt.key, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestTSubstitution(t *testing.T) {
	c := testCat(t)
	tests := []struct {
		name   string
		locale string
		key    string
		args   []any
		want   string
	}{
		{"no args", "en-US", "greeting", nil, "Hello"},
		{"fmt arg en", "en-US", "cart.count", []any{3}, "3 items in cart"},
		{"fmt arg fr", "fr-FR", "cart.count", []any{5}, "5 articles dans le panier"},
		{"missing key verbatim", "en-US", "x.y", nil, "x.y"},
		{"default when no ctx locale", "", "greeting", nil, "Hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.locale != "" {
				ctx = WithLocale(ctx, tt.locale)
			}
			got := c.T(ctx, tt.key, tt.args...)
			if got != tt.want {
				t.Errorf("T(%q,%q,%v) = %q, want %q", tt.locale, tt.key, tt.args, got, tt.want)
			}
		})
	}
}

// TestInertWhenUnconfigured verifies the package-level T works with no
// configuration: it resolves against the embedded default catalog, and unknown
// keys come back verbatim rather than panicking.
func TestInertWhenUnconfigured(t *testing.T) {
	// Embedded default has framework.* keys; an app key is unknown -> verbatim.
	if got := T(context.Background(), "app.unknown.key"); got != "app.unknown.key" {
		t.Errorf("default T unknown key = %q, want verbatim", got)
	}
	if got := T(context.Background(), "framework.error.not_found"); got != "Not found" {
		t.Errorf("default T known key = %q, want %q", got, "Not found")
	}
}

// TestSetDefault verifies that SetDefault makes the package-level T resolve an
// app catalog's strings, and that restoring nil falls back to the framework's
// embedded catalog.
func TestSetDefault(t *testing.T) {
	// Before SetDefault, an app key is unknown to the framework catalog.
	if got := T(context.Background(), "greeting"); got != "greeting" {
		t.Fatalf("pre-SetDefault T(greeting) = %q, want verbatim", got)
	}

	app := testCat(t)
	t.Cleanup(func() { SetDefault(nil) })
	SetDefault(app)

	// Now the package-level T resolves the app catalog.
	if got := T(context.Background(), "greeting"); got != "Hello" {
		t.Errorf("post-SetDefault T(greeting) = %q, want Hello", got)
	}
	ctx := WithLocale(context.Background(), "fr-FR")
	if got := T(ctx, "greeting"); got != "Bonjour" {
		t.Errorf("post-SetDefault T(fr greeting) = %q, want Bonjour", got)
	}

	// Restoring nil falls back to the framework's embedded catalog.
	SetDefault(nil)
	if got := T(context.Background(), "framework.error.not_found"); got != "Not found" {
		t.Errorf("after reset T(framework key) = %q, want %q", got, "Not found")
	}
	if got := T(context.Background(), "greeting"); got != "greeting" {
		t.Errorf("after reset T(app key) = %q, want verbatim", got)
	}
}

// TestCatalogFromContextWins verifies a catalog carried on the request context
// (as Middleware sets it) takes precedence over the package default for the
// package-level T.
func TestCatalogFromContextWins(t *testing.T) {
	// Default stays the framework catalog; a context catalog must still win.
	app := testCat(t)
	ctx := WithCatalog(context.Background(), app)
	if got := T(ctx, "greeting"); got != "Hello" {
		t.Errorf("ctx-catalog T(greeting) = %q, want Hello", got)
	}
	// Combined with a locale, the context catalog resolves the right language.
	ctx = WithLocale(ctx, "fr-FR")
	if got := T(ctx, "greeting"); got != "Bonjour" {
		t.Errorf("ctx-catalog T(fr greeting) = %q, want Bonjour", got)
	}
}

// TestMiddlewareCarriesCatalog verifies Middleware puts its catalog on the
// request context so the package-level T resolves app strings without an app
// threading its own *Catalog.
func TestMiddlewareCarriesCatalog(t *testing.T) {
	c := testCat(t)
	var seen string
	h := c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = T(r.Context(), "greeting") // package-level T, no threaded catalog
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "fr-FR"})
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen != "Bonjour" {
		t.Errorf("package-level T under middleware = %q, want Bonjour", seen)
	}
}

func TestWithLocaleRoundTrip(t *testing.T) {
	ctx := WithLocale(context.Background(), "fr-FR")
	if got := Locale(ctx); got != "fr-FR" {
		t.Errorf("Locale = %q, want fr-FR", got)
	}
	if got := Locale(context.Background()); got != "" {
		t.Errorf("Locale on bare ctx = %q, want empty", got)
	}
}

func TestLocales(t *testing.T) {
	c := testCat(t)
	got := c.Locales()
	want := []string{"en-US", "fr-FR"}
	if len(got) != len(want) {
		t.Fatalf("Locales = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Locales = %v, want %v", got, want)
		}
	}
}

func TestLoadNoLocalesDir(t *testing.T) {
	// A catalog with no locales directory is usable; keys return verbatim.
	c, err := Load(fstest.MapFS{}, "en-US")
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if got, ok := c.Lookup("en-US", "any.key"); got != "any.key" || ok {
		t.Errorf("empty catalog Lookup = (%q,%v), want verbatim,false", got, ok)
	}
}

func TestResolve(t *testing.T) {
	c := testCat(t)
	tests := []struct {
		name   string
		cookie string
		accept string
		want   string
	}{
		{"cookie wins", "fr-FR", "en-US", "fr-FR"},
		{"cookie case-insensitive", "fr-fr", "", "fr-FR"},
		{"bad cookie ignored", "zz-ZZ", "fr-FR", "fr-FR"},
		{"accept-language", "", "fr-FR,en;q=0.8", "fr-FR"},
		{"accept q-weight", "", "en-US;q=0.5, fr-FR;q=0.9", "fr-FR"},
		{"accept primary lang", "", "fr", "fr-FR"},
		{"unmatched falls to default", "", "de-DE,ja;q=0.5", "en-US"},
		{"no signals -> default", "", "", "en-US"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: CookieName, Value: tt.cookie})
			}
			if tt.accept != "" {
				r.Header.Set("Accept-Language", tt.accept)
			}
			if got := c.Resolve(r); got != tt.want {
				t.Errorf("Resolve = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	c := testCat(t)
	var seen string
	h := c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = c.T(r.Context(), "greeting")
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "fr-FR"})
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen != "Bonjour" {
		t.Errorf("middleware-resolved T = %q, want Bonjour", seen)
	}
}
