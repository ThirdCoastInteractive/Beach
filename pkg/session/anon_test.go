package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnonConfigDefaults(t *testing.T) {
	c := AnonConfig{}.withDefaults()
	if c.CookieName == "" || c.CookiePath == "" || c.MaxAge <= 0 {
		t.Fatalf("withDefaults left zero fields: %+v", c)
	}
	custom := AnonConfig{CookieName: "x", CookiePath: "/y", MaxAge: 60}.withDefaults()
	if custom.CookieName != "x" || custom.CookiePath != "/y" || custom.MaxAge != 60 {
		t.Fatalf("withDefaults overrode set fields: %+v", custom)
	}
}

func TestAnonFromContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := AnonFrom(ctx); ok {
		t.Fatal("empty context must carry no anonymous id")
	}
	got, ok := AnonFrom(withAnonID(ctx, "anon-123"))
	if !ok || got != "anon-123" {
		t.Fatalf("AnonFrom round-trip = (%q, %v), want (anon-123, true)", got, ok)
	}
}

func TestAnonStoreEnsureMintsIDAndCookie(t *testing.T) {
	s := NewAnonStore(AnonConfig{CookieName: "anon"})
	var got string
	var ok bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = AnonFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	s.Ensure(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !ok || got == "" {
		t.Fatal("Ensure must mint and attach an anonymous id")
	}
	sc := rec.Header().Get("Set-Cookie")
	if sc == "" {
		t.Fatal("Ensure must set the id cookie")
	}
	// HttpOnly is part of the hardening; the page script must not read identity.
	if !cookieHas(sc, "HttpOnly") {
		t.Fatalf("anonymous cookie not HttpOnly: %q", sc)
	}
	if !cookieHas(sc, "anon="+got) {
		t.Fatalf("cookie value != attached id: cookie=%q id=%q", sc, got)
	}
}

func TestAnonStoreEnsureReusesCookieID(t *testing.T) {
	s := NewAnonStore(AnonConfig{CookieName: "anon"})
	var got string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = AnonFrom(r.Context())
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "anon", Value: "existing-id"})
	rec := httptest.NewRecorder()
	s.Ensure(next).ServeHTTP(rec, req)

	if got != "existing-id" {
		t.Fatalf("Ensure minted a new id instead of reusing the cookie: %q", got)
	}
}

func TestAnonStoreSecureFlag(t *testing.T) {
	s := NewAnonStore(AnonConfig{CookieName: "anon", Secure: true})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	rec := httptest.NewRecorder()
	s.Ensure(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !cookieHas(rec.Header().Get("Set-Cookie"), "Secure") {
		t.Fatalf("Secure config not reflected in cookie: %q", rec.Header().Get("Set-Cookie"))
	}
}

// cookieHas reports whether the Set-Cookie header contains the given attribute or
// name=value token (case-sensitive on the token, attribute names are exact).
func cookieHas(setCookie, token string) bool {
	for _, part := range splitCookie(setCookie) {
		if part == token {
			return true
		}
	}
	return false
}

// splitCookie splits a Set-Cookie value on "; " into its tokens.
func splitCookie(s string) []string {
	var out []string
	start := 0
	for i := 0; i+1 < len(s); i++ {
		if s[i] == ';' && s[i+1] == ' ' {
			out = append(out, s[start:i])
			start = i + 2
		}
	}
	out = append(out, s[start:])
	return out
}
