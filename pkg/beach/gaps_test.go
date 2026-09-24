package beach

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/Beach/pkg/session"
)

// --- Gap 1: Config.Middleware injection + ordering ---

func TestConfigMiddlewareWrapsHandler(t *testing.T) {
	var hit bool
	a := New(Config{
		Service: "test",
		Middleware: []func(http.Handler) http.Handler{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hit = true
					w.Header().Set("X-MW", "1")
					next.ServeHTTP(w, r)
				})
			},
		},
	})
	h := a.Handler()
	a.Page("/", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>ok</main>")}, nil
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !hit {
		t.Fatal("app middleware was not invoked")
	}
	if rec.Header().Get("X-MW") != "1" {
		t.Fatal("app middleware did not run on the request path")
	}
}

func TestConfigMiddlewareOrderingOutermostFirst(t *testing.T) {
	var order []string
	mw := func(tag string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, tag)
				next.ServeHTTP(w, r)
			})
		}
	}
	a := New(Config{
		Service:    "test",
		Middleware: []func(http.Handler) http.Handler{mw("first"), mw("second")},
	})
	h := a.Handler()
	a.Page("/", func(c *Ctx) (View, error) {
		order = append(order, "handler")
		return View{Page: comp("<main>ok</main>")}, nil
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	got := strings.Join(order, ",")
	if got != "first,second,handler" {
		t.Fatalf("middleware order = %q, want first,second,handler (outermost-first)", got)
	}
}

func TestConfigMiddlewareRunsBeforeFixedStack(t *testing.T) {
	// An app middleware wraps outside the fixed stack: it sees the request before
	// the security-headers stage sets its headers, so it can read/replace them. We
	// assert it runs at all and that the fixed stack still applies underneath.
	a := New(Config{
		Service: "test",
		Middleware: []func(http.Handler) http.Handler{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r)
					// After the inner stack runs, the CSP header is present.
					if w.Header().Get("Content-Security-Policy") == "" {
						t.Error("fixed secure-headers stack did not run under app middleware")
					}
				})
			},
		},
	})
	h := a.Handler()
	a.Page("/", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>ok</main>")}, nil
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestConfigMiddlewareNilEntryIgnored(t *testing.T) {
	a := New(Config{
		Service:    "test",
		Middleware: []func(http.Handler) http.Handler{nil},
	})
	h := a.Handler()
	a.Page("/", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>ok</main>")}, nil
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("nil middleware entry broke the chain: status = %d", rec.Code)
	}
}

// --- Gap 2: Patch.Redirect / Patch.Script emission into the SSE stream ---

func datastarPost(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.String()
}

func TestActionPatchRedirectEmitsCSPSafeNavigation(t *testing.T) {
	a, h := testApp(t)
	a.Action("/login", func(c *Ctx) (Patches, error) {
		return Patches{{Redirect: "/dashboard"}}, nil
	})
	body := datastarPost(t, h, "/login")
	if !strings.Contains(body, "/dashboard") {
		t.Fatalf("redirect patch body = %q, want the target url", body)
	}
	if !strings.Contains(body, "window.location") {
		t.Fatalf("redirect patch body = %q, want a window.location assignment", body)
	}
	// The navigation must ride a data-init expression, not an inline <script>:
	// the framework's own script-src CSP blocks inline scripts, which is what
	// broke the SDK's Redirect helper in a real browser.
	if strings.Contains(body, "<script") {
		t.Fatalf("redirect patch body = %q, must not emit an inline script under the strict CSP", body)
	}
}

func TestActionPatchScriptEmitsScriptElement(t *testing.T) {
	a, h := testApp(t)
	a.Action("/run", func(c *Ctx) (Patches, error) {
		return Patches{{Script: "console.log('hi')"}}, nil
	})
	body := datastarPost(t, h, "/run")
	if !strings.Contains(body, "console.log('hi')") {
		t.Fatalf("script patch body = %q, want the script contents", body)
	}
	if !strings.Contains(body, "<script") {
		t.Fatalf("script patch body = %q, want a <script> element", body)
	}
}

func TestActionPatchFragmentThenRedirect(t *testing.T) {
	// A single Patch can carry both a fragment and a redirect; the fragment flushes
	// first, the navigation last.
	a, h := testApp(t)
	a.Action("/save", func(c *Ctx) (Patches, error) {
		return Patches{{
			Fragment: comp("<div id=\"flash\">saved</div>"),
			Target:   "flash",
			Redirect: "/next",
		}}, nil
	})
	body := datastarPost(t, h, "/save")
	if !strings.Contains(body, "saved") {
		t.Fatalf("combined patch missing fragment: %q", body)
	}
	if !strings.Contains(body, "/next") {
		t.Fatalf("combined patch missing redirect: %q", body)
	}
	if strings.Index(body, "saved") > strings.Index(body, "/next") {
		t.Fatal("fragment must flush before the redirect navigation")
	}
}

func TestCtxSetCookieBeforeRedirect(t *testing.T) {
	// The login escape: set a session cookie in the action body, then redirect via a
	// Patch — both land on the same response, no Raw handler needed.
	a, h := testApp(t)
	a.Action("/login", func(c *Ctx) (Patches, error) {
		c.SetCookie(&http.Cookie{Name: "sess", Value: "abc", Path: "/", HttpOnly: true})
		return Patches{{Redirect: "/home"}}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if sc := rec.Header().Get("Set-Cookie"); !strings.Contains(sc, "sess=abc") {
		t.Fatalf("Set-Cookie = %q, want the session cookie on the redirect response", sc)
	}
	if !strings.Contains(rec.Body.String(), "/home") {
		t.Fatalf("redirect missing from body: %q", rec.Body.String())
	}
}

// --- Gap 3: DB-less anonymous principal resolution (no Postgres) ---

func TestAnonymousPrincipalResolvedWithoutDB(t *testing.T) {
	a := New(Config{
		Service:           "test",
		AnonymousSessions: session.NewAnonStore(session.AnonConfig{}),
	})
	h := a.Handler()

	var p *Principal
	var anonID string
	var hasAnon bool
	a.Page("/", func(c *Ctx) (View, error) {
		p, _ = c.Principal()
		anonID, hasAnon = c.AnonID()
		return View{Page: comp("<main>ok</main>")}, nil
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if p == nil {
		t.Fatal("anonymous app must resolve a non-nil principal")
	}
	if !p.IsAnonymous() {
		t.Fatal("principal should report IsAnonymous()")
	}
	if !hasAnon || anonID == "" || p.AnonID != anonID {
		t.Fatalf("anon id mismatch: ctx=%q principal=%q", anonID, p.AnonID)
	}
	// The anonymous principal holds no privileges.
	if p.Can("anything:read") || p.HasRole("admin") {
		t.Fatal("anonymous principal must hold no permissions or roles")
	}
	// The id cookie was minted on the response.
	if sc := rec.Header().Get("Set-Cookie"); !strings.Contains(sc, "beach_session=") {
		t.Fatalf("anonymous id cookie not set: %q", sc)
	}
}

func TestAnonymousSessionStableAcrossRequests(t *testing.T) {
	store := session.NewAnonStore(session.AnonConfig{CookieName: "anon"})
	a := New(Config{Service: "test", AnonymousSessions: store})
	h := a.Handler()

	var seen string
	a.Page("/", func(c *Ctx) (View, error) {
		id, _ := c.AnonID()
		seen = id
		return View{Page: comp("<main>ok</main>")}, nil
	})

	// First request mints an id and sets the cookie.
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/", nil))
	first := seen
	if first == "" {
		t.Fatal("first request did not mint an anonymous id")
	}

	// Second request carrying the cookie reuses the same id.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Cookie", "anon="+first)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if seen != first {
		t.Fatalf("anonymous id changed across requests: %q then %q", first, seen)
	}
}

func TestAnonymousGuardStillRejects(t *testing.T) {
	// An anonymous principal is unprivileged: Authed() rejects it (no session user).
	a := New(Config{
		Service:           "test",
		AnonymousSessions: session.NewAnonStore(session.AnonConfig{}),
	})
	h := a.Handler()
	a.Page("/private", func(c *Ctx) (View, error) {
		return View{Page: comp("<main>secret</main>")}, nil
	}, a.Authed())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/private", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous behind Authed status = %d, want 401", rec.Code)
	}
}

func TestSessionsTakePrecedenceOverAnonymous(t *testing.T) {
	// When a DB-backed store is configured, the anonymous path is not wired: the DB
	// store's OptionalAuth runs and an anonymous (no-cookie) request stays principal-
	// less, exactly as before.
	dbStore := &session.Store{}
	a := New(Config{
		Service:           "test",
		Sessions:          dbStore,
		AnonymousSessions: session.NewAnonStore(session.AnonConfig{}),
	})
	h := a.Handler()
	var p *Principal
	a.Page("/", func(c *Ctx) (View, error) {
		p, _ = c.Principal()
		return View{Page: comp("<main>ok</main>")}, nil
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if p != nil {
		t.Fatalf("DB-backed app must not mint an anonymous principal, got %+v", p)
	}
	// No anonymous cookie should be set when Sessions wins.
	if sc := rec.Header().Get("Set-Cookie"); strings.Contains(sc, "beach_session=") {
		t.Fatalf("anonymous cookie leaked under a DB store: %q", sc)
	}
}
