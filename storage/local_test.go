package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := NewLocal(t.TempDir(), "/files")
	if err != nil {
		t.Fatal(err)
	}

	// Put returns metadata; content type derives from the .png extension.
	body := []byte("\x89PNG fake image bytes")
	f, err := s.Put(ctx, "avatars/42.png", bytes.NewReader(body), "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if f.Key != "avatars/42.png" || f.Size != int64(len(body)) || f.ContentType != "image/png" {
		t.Fatalf("Put metadata: %+v", f)
	}

	// Open returns the same bytes.
	rc, of, err := s.Open(ctx, "avatars/42.png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Errorf("Open body mismatch")
	}
	if of.Size != int64(len(body)) {
		t.Errorf("Open size = %d", of.Size)
	}

	// Stat without body.
	if st, err := s.Stat(ctx, "avatars/42.png"); err != nil || st.Size != int64(len(body)) {
		t.Fatalf("Stat: %+v %v", st, err)
	}

	// URL points at the public prefix.
	if u, _ := s.URL(ctx, "avatars/42.png"); u != "/files/avatars/42.png" {
		t.Errorf("URL = %q", u)
	}

	// Explicit content type wins over the extension.
	cf, _ := s.Put(ctx, "blob", bytes.NewReader([]byte("x")), "application/x-thing")
	if cf.ContentType != "application/x-thing" {
		t.Errorf("explicit content type = %q", cf.ContentType)
	}

	// Delete, then it's gone.
	if err := s.Delete(ctx, "avatars/42.png"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := s.Open(ctx, "avatars/42.png"); !errors.Is(err, ErrNotExist) {
		t.Errorf("after delete, Open err = %v, want ErrNotExist", err)
	}
	// Deleting a missing key is fine.
	if err := s.Delete(ctx, "nope.png"); err != nil {
		t.Errorf("Delete missing: %v", err)
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	s, _ := NewLocal(t.TempDir(), "/files")
	for _, bad := range []string{"../escape", "../../etc/passwd", "/abs"} {
		if _, err := s.Put(context.Background(), bad, strings.NewReader("x"), ""); err == nil {
			// "/abs" cleans to "abs" (allowed); traversal must be rejected.
			if strings.Contains(bad, "..") {
				t.Errorf("Put(%q) should reject traversal", bad)
			}
		}
	}
}

func TestLocalServeHTTP(t *testing.T) {
	s, _ := NewLocal(t.TempDir(), "/files")
	_, _ = s.Put(context.Background(), "css/x.css", strings.NewReader("body{}"), "")

	h := http.StripPrefix("/files/", s)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/files/css/x.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("content-type = %q", ct)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("missing immutable cache header")
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/files/missing.css", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("missing file status = %d, want 404", rec2.Code)
	}
}
