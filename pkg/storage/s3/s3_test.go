package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/storage"
)

// testStore builds a store with fake credentials. New never dials, and the
// tests below stick to calls that fail on key validation or compute locally
// (public URLs, presigning), so no network is touched.
func testStore(t *testing.T, publicBase string) *S3 {
	t.Helper()
	s, err := New(Config{
		Endpoint:        "accountid.r2.cloudflarestorage.com",
		Bucket:          "files",
		AccessKeyID:     "AKIA_TEST",
		SecretAccessKey: "secret",
		PublicBaseURL:   publicBase,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewValidatesConfig(t *testing.T) {
	for name, cfg := range map[string]Config{
		"missing endpoint": {Bucket: "b", AccessKeyID: "k", SecretAccessKey: "s"},
		"plaintext scheme": {Endpoint: "http://host", Bucket: "b", AccessKeyID: "k", SecretAccessKey: "s"},
		"missing bucket":   {Endpoint: "host", AccessKeyID: "k", SecretAccessKey: "s"},
		"missing creds":    {Endpoint: "host", Bucket: "b"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%s) should fail", name)
		}
	}

	// The https scheme is tolerated (R2's dashboard shows it) and stripped.
	if _, err := New(Config{Endpoint: "https://host/", Bucket: "b", AccessKeyID: "k", SecretAccessKey: "s"}); err != nil {
		t.Errorf("New with https endpoint: %v", err)
	}
}

func TestCleanKey(t *testing.T) {
	// Any ".." segment is rejected outright, even ones path.Clean would
	// resolve safely inside the root — loud beats clever.
	for _, bad := range []string{"", "/", "..", "../escape", "a/../../b", "a/inner/../x.png"} {
		if _, err := cleanKey(bad); err == nil {
			t.Errorf("cleanKey(%q) should fail", bad)
		}
	}
	for in, want := range map[string]string{
		"avatars/42.png": "avatars/42.png",
		"/rooted.png":    "rooted.png",
		"./a//b.png":     "a/b.png",
	} {
		got, err := cleanKey(in)
		if err != nil || got != want {
			t.Errorf("cleanKey(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

// TestStoreRejectsBadKeys checks every Store method fails on traversal before
// any request could be made.
func TestStoreRejectsBadKeys(t *testing.T) {
	ctx := context.Background()
	s := testStore(t, "")
	const bad = "../../etc/passwd"

	if _, err := s.Put(ctx, bad, bytes.NewReader(nil), ""); err == nil {
		t.Error("Put should reject traversal")
	}
	if _, _, err := s.Open(ctx, bad); err == nil {
		t.Error("Open should reject traversal")
	}
	if _, err := s.Stat(ctx, bad); err == nil {
		t.Error("Stat should reject traversal")
	}
	if err := s.Delete(ctx, bad); err == nil {
		t.Error("Delete should reject traversal")
	}
	if _, err := s.URL(ctx, bad); err == nil {
		t.Error("URL should reject traversal")
	}
}

func TestPublicURL(t *testing.T) {
	ctx := context.Background()

	// Trailing slash on the base is trimmed; keys are normalized.
	s := testStore(t, "https://files.example.com/")
	if u, err := s.URL(ctx, "avatars/42.png"); err != nil || u != "https://files.example.com/avatars/42.png" {
		t.Errorf("URL = %q, %v", u, err)
	}
	if u, _ := s.URL(ctx, "./a//b.png"); u != "https://files.example.com/a/b.png" {
		t.Errorf("normalized URL = %q", u)
	}
}

func TestPresignedURL(t *testing.T) {
	s := testStore(t, "") // no public base -> presign
	raw, err := s.URL(context.Background(), "avatars/42.png")
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	if u.Scheme != "https" {
		t.Errorf("scheme = %q, want https", u.Scheme)
	}
	if !strings.HasSuffix(u.Path, "/files/avatars/42.png") {
		t.Errorf("path = %q, want bucket/key suffix", u.Path)
	}
	q := u.Query()
	if q.Get("X-Amz-Signature") == "" {
		t.Error("missing X-Amz-Signature")
	}
	if q.Get("X-Amz-Expires") != "3600" {
		t.Errorf("X-Amz-Expires = %q, want 3600 (one hour)", q.Get("X-Amz-Expires"))
	}
}

// TestR2Integration runs a full round trip against a real bucket. Set
// R2_TEST_DSN to enable it:
//
//	R2_TEST_DSN=https://ACCESS_KEY_ID:SECRET@accountid.r2.cloudflarestorage.com/bucket
func TestR2Integration(t *testing.T) {
	dsn := os.Getenv("R2_TEST_DSN")
	if dsn == "" {
		t.Skip("R2_TEST_DSN not set")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("R2_TEST_DSN: %v", err)
	}
	secret, _ := u.User.Password()
	s, err := New(Config{
		Endpoint:        u.Host,
		Bucket:          strings.Trim(u.Path, "/"),
		AccessKeyID:     u.User.Username(),
		SecretAccessKey: secret,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	key := "beach-test/" + time.Now().UTC().Format("20060102T150405") + ".txt"
	body := []byte("the beach integration test")

	f, err := s.Put(ctx, key, bytes.NewReader(body), "text/plain")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if f.Size != int64(len(body)) || f.ContentType != "text/plain" {
		t.Errorf("Put metadata: %+v", f)
	}

	st, err := s.Stat(ctx, key)
	if err != nil || st.Size != int64(len(body)) || st.ContentType != "text/plain" {
		t.Fatalf("Stat: %+v %v", st, err)
	}

	rc, of, err := s.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Errorf("Open body mismatch: %q", got)
	}
	if of.Size != int64(len(body)) {
		t.Errorf("Open size = %d", of.Size)
	}

	if u, err := s.URL(ctx, key); err != nil || u == "" {
		t.Errorf("URL: %q %v", u, err)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Stat(ctx, key); !errors.Is(err, storage.ErrNotExist) {
		t.Errorf("after delete, Stat err = %v, want ErrNotExist", err)
	}
	// Deleting a missing key is fine.
	if err := s.Delete(ctx, key); err != nil {
		t.Errorf("Delete missing: %v", err)
	}
}
