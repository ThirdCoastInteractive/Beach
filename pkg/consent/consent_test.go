package consent

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseEncodeRoundTrip(t *testing.T) {
	cases := map[string]struct {
		allow   map[string]bool
		version int
		raw     string
	}{
		"granted analytics": {
			allow:   map[string]bool{"analytics": true},
			version: 1,
			raw:     "v1.analytics=1",
		},
		"denied analytics": {
			allow:   map[string]bool{"analytics": false},
			version: 1,
			raw:     "v1.analytics=0",
		},
		"mixed categories sorted": {
			allow:   map[string]bool{"marketing": false, "analytics": true},
			version: 1,
			raw:     "v1.analytics=1.marketing=0",
		},
		"necessary only": {
			allow:   map[string]bool{},
			version: 1,
			raw:     "v1",
		},
		"nil allow is necessary only": {
			allow:   nil,
			version: 1,
			raw:     "v1",
		},
		"version 2": {
			allow:   map[string]bool{"analytics": true},
			version: 2,
			raw:     "v2.analytics=1",
		},
		"hyphen and underscore slugs": {
			allow:   map[string]bool{"error-logs": true, "session_replay": false},
			version: 1,
			raw:     "v1.error-logs=1.session_replay=0",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gotRaw := Encode(tc.allow, tc.version)
			if gotRaw != tc.raw {
				t.Fatalf("Encode = %q, want %q", gotRaw, tc.raw)
			}
			got := Parse(gotRaw, tc.version)
			if !got.Set {
				t.Fatalf("Parse(%q) unset", gotRaw)
			}
			if Encode(got.Allow, tc.version) != tc.raw {
				t.Fatalf("round-trip Encode = %q, want %q (Allow=%v)", Encode(got.Allow, tc.version), tc.raw, got.Allow)
			}
			if tc.allow != nil && !maps.Equal(got.Allow, tc.allow) {
				t.Fatalf("Allow = %v, want %v", got.Allow, tc.allow)
			}
		})
	}

	t.Run("trimmed granted", func(t *testing.T) {
		got := Parse(" v1.analytics=1 ", 1)
		if !got.Set || !got.Allow["analytics"] {
			t.Fatalf("trimmed granted: %#v", got)
		}
	})
}

func TestParseUnset(t *testing.T) {
	cases := map[string]string{
		"empty":              "",
		"wrong version":      "v0.analytics=1",
		"newer version":      "v2.analytics=1",
		"bad value":          "v1.analytics=yes",
		"garbage":            "garbage",
		"leading space junk": " v1.a=yes ",
		"leading zeros":      "v01.analytics=1",
		"no version":         "v.analytics=1",
		"trailing dot":       "v1.analytics=1.",
		"missing value":      "v1.analytics=",
		"uppercase slug":     "v1.Analytics=1",
		"duplicate slug":     "v1.analytics=1.analytics=0",
		"extra suffix":       "v1.analytics=10",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got := Parse(raw, 1)
			if got.Set {
				t.Fatalf("Parse(%q) = %#v, want unset", raw, got)
			}
		})
	}
}

func TestAllowsHonorsCookieAndBrowserSignals(t *testing.T) {
	cfg := Config{}
	cases := map[string]struct {
		cookie   string
		gpc, dnt string
		want     bool
	}{
		"granted cookie":   {cookie: Encode(map[string]bool{"analytics": true}, 1), want: true},
		"denied cookie":    {cookie: Encode(map[string]bool{"analytics": false}, 1)},
		"unset":            {},
		"missing category": {cookie: Encode(map[string]bool{"marketing": true}, 1)},
		"gpc wins over allow": {
			cookie: Encode(map[string]bool{"analytics": true}, 1),
			gpc:    "1",
		},
		"dnt wins over allow": {
			cookie: Encode(map[string]bool{"analytics": true}, 1),
			dnt:    "1",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: defaultCookieName, Value: tc.cookie})
			}
			if tc.gpc != "" {
				r.Header.Set("Sec-GPC", tc.gpc)
			}
			if tc.dnt != "" {
				r.Header.Set("DNT", tc.dnt)
			}
			if got := Allows(r, cfg, "analytics"); got != tc.want {
				t.Fatalf("Allows = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNeedsPrompt(t *testing.T) {
	cfg := Config{}
	cases := map[string]struct {
		cookie   string
		gpc, dnt bool
		nilReq   bool
		want     bool
	}{
		"unset prompts":          {want: true},
		"recorded no skips":      {cookie: Encode(map[string]bool{"analytics": false}, 1)},
		"recorded yes skips":     {cookie: Encode(map[string]bool{"analytics": true}, 1)},
		"gpc skips":              {gpc: true},
		"dnt skips":              {dnt: true},
		"nil request prompts":    {nilReq: true, want: true},
		"missing cookie prompts": {want: true},
		"gpc with cookie skips":  {cookie: Encode(map[string]bool{"analytics": true}, 1), gpc: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var r *http.Request
			if !tc.nilReq {
				r = httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
				if tc.cookie != "" {
					r.AddCookie(&http.Cookie{Name: defaultCookieName, Value: tc.cookie})
				}
				if tc.gpc {
					r.Header.Set("Sec-GPC", "1")
				}
				if tc.dnt {
					r.Header.Set("DNT", "1")
				}
			}
			if got := NeedsPrompt(r, cfg); got != tc.want {
				t.Fatalf("NeedsPrompt = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("wrong cookie name is unset", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		r.AddCookie(&http.Cookie{Name: "other", Value: Encode(map[string]bool{"analytics": true}, 1)})
		if !NeedsPrompt(r, Config{CookieName: "app_consent"}) {
			t.Fatal("missing named cookie should prompt")
		}
	})

	t.Run("version bump treats old cookie as unset", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		r.AddCookie(&http.Cookie{Name: defaultCookieName, Value: Encode(map[string]bool{"analytics": true}, 1)})
		if !NeedsPrompt(r, Config{Version: 2}) {
			t.Fatal("old version should prompt after bump")
		}
		if Allows(r, Config{Version: 2}, "analytics") {
			t.Fatal("old version must not allow after bump")
		}
	})
}

func TestDomainForHost(t *testing.T) {
	cases := map[string]struct {
		configured, host, want string
	}{
		"configured apex":             {".example.com", "example.com", ".example.com"},
		"www of configured":           {".example.com", "www.example.com", ".example.com"},
		"subdomain with port":         {".example.com", "app.example.com:443", ".example.com"},
		"unrelated host uses eTLD+1":  {".example.com", "show.other.com", ".other.com"},
		"empty configured two labels": {"", "podcast.example.org", ".example.org"},
		"localhost":                   {"", "localhost:9351", ""},
		"loopback v4":                 {"", "127.0.0.1:8080", ""},
		"loopback v6":                 {"", "[::1]:8080", ""},
		"private ip":                  {"", "192.168.1.10", ""},
		"configured without dot":      {"example.com", "www.example.com", "example.com"},
		"single label":                {"", "intranet", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := DomainForHost(tc.configured, tc.host); got != tc.want {
				t.Fatalf("DomainForHost(%q, %q) = %q, want %q", tc.configured, tc.host, got, tc.want)
			}
		})
	}
}

func TestCookieFlags(t *testing.T) {
	c := Cookie(Config{}, map[string]bool{"analytics": true}, ".example.com", true)
	if c.Name != defaultCookieName || c.Value != "v1.analytics=1" {
		t.Fatalf("name/value: %#v", c)
	}
	if c.Path != "/" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags: %#v", c)
	}
	if c.Domain != ".example.com" {
		t.Fatalf("domain %q", c.Domain)
	}
	if c.MaxAge != int(defaultMaxAge.Seconds()) {
		t.Fatalf("max-age %d, want %d", c.MaxAge, int(defaultMaxAge.Seconds()))
	}

	custom := Cookie(Config{CookieName: "app_consent", Version: 3, MaxAge: 24 * time.Hour}, nil, "", false)
	if custom.Name != "app_consent" || custom.Value != "v3" || custom.Secure || custom.MaxAge != int((24*time.Hour).Seconds()) {
		t.Fatalf("custom cookie: %#v", custom)
	}

	exp := ExpireCookie("app_consent", ".example.com", true)
	if exp.Name != "app_consent" || exp.MaxAge != -1 || exp.Value != "" || !exp.HttpOnly || exp.Path != "/" || !exp.Secure || exp.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expire: %#v", exp)
	}
}

func TestFromRequestNilAndMissing(t *testing.T) {
	if got := FromRequest(nil, Config{}); got.Set {
		t.Fatalf("nil request: %#v", got)
	}
	r := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	if got := FromRequest(r, Config{}); got.Set {
		t.Fatalf("missing cookie: %#v", got)
	}
	if BrowserOptOut(nil) {
		t.Fatal("nil request is not an opt-out")
	}
	if Allows(nil, Config{}, "analytics") {
		t.Fatal("nil request must not allow")
	}
}
