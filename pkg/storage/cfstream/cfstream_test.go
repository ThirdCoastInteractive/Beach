package cfstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testClient points a Client at an httptest server standing in for the
// Cloudflare API.
func testClient(handler http.Handler) (*Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	c := New("acct-1", "tok-secret", "custcode")
	c.base = srv.URL
	return c, srv
}

func TestCreateTusSuccess(t *testing.T) {
	const location = "https://upload.videodelivery.net/tus/abc123?tusv2=true"

	var gotAuth, gotTus, gotLength, gotMeta, gotPath, gotQuery string
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotTus = r.Header.Get("Tus-Resumable")
		gotLength = r.Header.Get("Upload-Length")
		gotMeta = r.Header.Get("Upload-Metadata")
		w.Header().Set("Location", location)
		w.Header().Set("stream-media-id", "headeruid456")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	got, err := c.CreateTus(context.Background(), 26843545600, map[string]string{
		"name":               "clip",
		"maxDurationSeconds": "7200",
	})
	if err != nil {
		t.Fatalf("CreateTus: %v", err)
	}
	if gotPath != "/accounts/acct-1/stream" || gotQuery != "direct_user=true" {
		t.Errorf("request = POST %s?%s", gotPath, gotQuery)
	}
	if gotAuth != "Bearer tok-secret" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotTus != "1.0.0" {
		t.Errorf("Tus-Resumable = %q", gotTus)
	}
	if gotLength != "26843545600" {
		t.Errorf("Upload-Length = %q", gotLength)
	}
	if !strings.Contains(gotMeta, "maxDurationSeconds NzIwMA==") || !strings.Contains(gotMeta, "name Y2xpcA==") {
		t.Errorf("Upload-Metadata = %q", gotMeta)
	}
	// The URL must survive unmodified — it is the only resumable handle, and
	// the tus v2 query string is part of the resource identity.
	if got.URL != location {
		t.Errorf("URL = %q, want %q (query string must be preserved)", got.URL, location)
	}
	// stream-media-id wins over the (different) URL path segment.
	if got.UID != "headeruid456" {
		t.Errorf("UID = %q, want headeruid456 (from stream-media-id)", got.UID)
	}
}

func TestCreateTusFallsBackToLocationUID(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://upload.videodelivery.net/tus/fallback789?tusv2=true")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	got, err := c.CreateTus(context.Background(), 1024, nil)
	if err != nil {
		t.Fatalf("CreateTus: %v", err)
	}
	if got.UID != "fallback789" {
		t.Errorf("UID = %q, want fallback789 (query stripped)", got.UID)
	}
	if got.URL != "https://upload.videodelivery.net/tus/fallback789?tusv2=true" {
		t.Errorf("URL = %q, want Location verbatim", got.URL)
	}
}

func TestCreateTusMissingLocation(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("stream-media-id", "headeruid456")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	_, err := c.CreateTus(context.Background(), 1024, nil)
	if err == nil || !strings.Contains(err.Error(), "Location") {
		t.Errorf("error = %v, want missing Location", err)
	}
}

func TestCreateTusBadStatus(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"too large"}]}`))
	}))
	defer srv.Close()

	_, err := c.CreateTus(context.Background(), 1024, nil)
	if err == nil {
		t.Fatal("CreateTus should fail on non-201")
	}
	for _, want := range []string{"400", "too large"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestCreateTusZeroLength(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("CreateTus should not call the API for non-positive length")
	}))
	defer srv.Close()

	if _, err := c.CreateTus(context.Background(), 0, nil); err == nil {
		t.Fatal("CreateTus(0) succeeded, want error")
	}
	if _, err := c.CreateTus(context.Background(), -1, nil); err == nil {
		t.Fatal("CreateTus(-1) succeeded, want error")
	}
}

func TestDelete(t *testing.T) {
	var gotPath, gotMethod string
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{}})
	}))
	defer srv.Close()

	if err := c.Delete(context.Background(), "vid-42"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/accounts/acct-1/stream/vid-42" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
}

func TestDeleteError(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors":  []map[string]any{{"code": 5404, "message": "Video not found"}},
		})
	}))
	defer srv.Close()

	err := c.Delete(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "5404") {
		t.Errorf("error = %v, want code 5404 surfaced", err)
	}
}

func TestIframeURL(t *testing.T) {
	c := New("acct-1", "tok", "custcode")
	got := c.IframeURL("vid-42")
	if got != "https://customer-custcode.cloudflarestream.com/vid-42/iframe" {
		t.Errorf("IframeURL = %q", got)
	}
}

func TestThumbnailURL(t *testing.T) {
	c := New("acct-1", "tok", "custcode")
	got := c.ThumbnailURL("vid-42")
	if got != "https://videodelivery.net/vid-42/thumbnails/thumbnail.jpg" {
		t.Errorf("ThumbnailURL = %q", got)
	}
}

func TestEncodeTusMetadata(t *testing.T) {
	got := encodeTusMetadata(map[string]string{
		"episode_id": "ep-1",
		"flag":       "",
		"show_id":    "sh-1",
	})
	want := "episode_id ZXAtMQ==,flag,show_id c2gtMQ=="
	if got != want {
		t.Errorf("encodeTusMetadata() = %q, want %q", got, want)
	}
	if got := encodeTusMetadata(nil); got != "" {
		t.Errorf("encodeTusMetadata(nil) = %q, want empty", got)
	}
}

func TestUIDFromTusLocation(t *testing.T) {
	cases := map[string]struct {
		location string
		want     string
	}{
		"tus v2 location with query string": {
			location: "https://upload.videodelivery.net/tus/8bc0ca451ccfc67aff2aef43?tusv2=true",
			want:     "8bc0ca451ccfc67aff2aef43",
		},
		"tus v1 api location": {
			location: "https://api.cloudflare.com/client/v4/accounts/acct123/stream/a10e9830b7dfa03e6d578b36",
			want:     "a10e9830b7dfa03e6d578b36",
		},
		"multiple query params": {
			location: "https://upload.videodelivery.net/tus/abc123?tusv2=true&foo=bar",
			want:     "abc123",
		},
		"trailing slash": {
			location: "https://upload.videodelivery.net/tus/abc123/",
			want:     "abc123",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := uidFromTusLocation(tc.location); got != tc.want {
				t.Errorf("uidFromTusLocation(%q) = %q, want %q", tc.location, got, tc.want)
			}
		})
	}
}
