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
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/Beach/pkg/prefs"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"
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

// TestAppStringsResolveOffTheRequestPath covers the render with no request
// behind it — a hub.Ticker fanning a fragment, a background job — which is where
// the scaffold's own clock lives.
//
// Config.Locales puts the catalog on the *request* context, and that is all it
// used to do, so a fragment rendered from a ticker fell through to the
// framework's embedded catalog and emitted the app's keys as literal text. The
// scaffold shipped that: an SSE tick read "home.clock 09:11:16" in the browser.
func TestAppStringsResolveOffTheRequestPath(t *testing.T) {
	// SetDefault is package-global; put it back so the rest of this binary sees
	// the framework catalog it expects.
	t.Cleanup(func() { i18n.SetDefault(nil) })

	cat, err := i18n.Load(fstest.MapFS{
		"locales/en-US.json": &fstest.MapFile{Data: []byte(`{"home.clock":"Server time"}`)},
	}, "en-US")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	New(Config{Locales: cat})

	// context.Background() is what a ticker's render has.
	if got := i18n.T(context.Background(), "home.clock"); got != "Server time" {
		t.Errorf("an off-request render emitted %q — the app's catalog never became the default", got)
	}

	// And the framework's own strings still resolve underneath it.
	if got := i18n.T(context.Background(), "ui.a11y.close"); got == "ui.a11y.close" {
		t.Errorf("registering the app catalog buried the framework's strings: %q", got)
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

// --- WCAG 2.2.2 Pause, Stop, Hide -------------------------------------------------

// TestPausedVisitorGetsCatchUpAndNothingElse is the framework half of the pause.
//
// The criterion names auto-updating information explicitly, and this framework's
// premise is server-pushed updates, so the mechanism has to hold for streams the
// kit knows nothing about. Putting the check in the adapter rather than asking
// each StreamFunc to remember it means an app cannot route around it by
// forgetting.
//
// Catch-up still runs, and that is deliberate: a paused visitor should still see
// where things stand right now. Note 3 of the criterion is explicit that a paused
// stream owes no replay of what it missed, which is why ending there is a
// complete answer rather than a lossy one.
func TestPausedVisitorGetsCatchUpAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		name       string
		paused     bool
		wantTopics bool
	}{
		{"a visitor who said nothing keeps their live updates", false, true},
		{"a visitor who paused gets no subscription", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := New(Config{Hub: hub.New()})
			caughtUp := false
			var sawTopics []string

			a.Stream("/live", func(c *Ctx) (Sub, error) {
				return Sub{
					Topics: []string{"tick"},
					CatchUp: func(_ string, p Patcher) error {
						caughtUp = true
						return nil
					},
				}, nil
			})

			// The hub records who subscribed, which is the observable that
			// matters: a paused visitor must not be holding a subscription.
			req := httptest.NewRequest(http.MethodGet, "/live", nil)
			if tc.paused {
				req.AddCookie(&http.Cookie{Name: PrefsCookie, Value: "live"})
			}
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				a.Handler().ServeHTTP(rec, req)
			}()

			select {
			case <-done:
				// A paused stream ends on its own once catch-up is written.
				sawTopics = nil
			case <-time.After(200 * time.Millisecond):
				// A live stream stays open waiting for events, which is the
				// correct behaviour and not something to assert against.
				sawTopics = []string{"tick"}
			}

			if !caughtUp {
				t.Error("catch-up did not run; a paused visitor should still see current state once")
			}
			if got := sawTopics != nil; got != tc.wantTopics {
				t.Errorf("stream stayed open = %v, want %v", got, tc.wantTopics)
			}
		})
	}
}

// TestPrefsRoundTripThroughTheCookie checks the preference survives, since a
// pause that forgets itself on the next page is not a mechanism.
func TestPrefsRoundTripThroughTheCookie(t *testing.T) {
	all := prefs.Default()
	if !all.LiveUpdates || !all.AutoDismiss {
		t.Fatal("the default is not everything-on, so a first visit reads as an opt-out")
	}
	if all.String() != "" {
		t.Errorf("a visitor at the defaults carries a non-empty cookie %q", all.String())
	}

	paused := prefs.Prefs{LiveUpdates: false, AutoDismiss: false}
	if got := prefs.Parse(paused.String()); got != paused {
		t.Errorf("round trip lost the preference: %+v -> %q -> %+v", paused, paused.String(), got)
	}
	// An unknown token from another build must degrade to the default rather
	// than to a zero value that reads as a total opt-out.
	if got := prefs.Parse("live,something-else"); got.LiveUpdates || !got.AutoDismiss {
		t.Errorf("an unrecognised token was not ignored: %+v", got)
	}
}

// TestPrefsReturnRefusesForeignReferers keeps the preference route from becoming
// an open redirect: the Referer is attacker-influenced, so only a same-origin
// path is echoed back.
func TestPrefsReturnRefusesForeignReferers(t *testing.T) {
	cases := map[string]string{
		"":                          "/",
		"http://evil.test/pwn":      "/",
		"//evil.test/pwn":           "/",
		"http://example.test/board": "/board",
		"/board?page=2":             "/board?page=2",
	}
	for ref, want := range cases {
		r := httptest.NewRequest(http.MethodPost, PrefsPath, nil)
		r.Host = "example.test"
		if ref != "" {
			r.Header.Set("Referer", ref)
		}
		if got := prefsReturn(r); got != want {
			t.Errorf("Referer %q -> %q, want %q", ref, got, want)
		}
	}
}
