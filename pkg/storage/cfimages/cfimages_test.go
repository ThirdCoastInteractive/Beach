package cfimages

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testClient points a Client at an httptest server standing in for the
// Cloudflare API.
func testClient(handler http.Handler) (*Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	c := New("acct-1", "tok-secret", "hash-abc")
	c.base = srv.URL
	return c, srv
}

func TestUploadSuccess(t *testing.T) {
	var gotAuth, gotFilename, gotBody string
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/accounts/acct-1/images/v1" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		b, _ := io.ReadAll(f)
		f.Close()
		gotFilename, gotBody = hdr.Filename, string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  map[string]any{"id": "2cdc28f0-017a-49c4-9ed7-87056c83901"},
			"errors":  []any{},
		})
	}))
	defer srv.Close()

	id, err := c.Upload(context.Background(), strings.NewReader("png bytes"), "cat.png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if id != "2cdc28f0-017a-49c4-9ed7-87056c83901" {
		t.Errorf("id = %q", id)
	}
	if gotAuth != "Bearer tok-secret" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotFilename != "cat.png" || gotBody != "png bytes" {
		t.Errorf("multipart file = %q %q", gotFilename, gotBody)
	}
}

func TestUploadErrorEnvelope(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors": []map[string]any{
				{"code": 5400, "message": "images.create: bad image format"},
			},
		})
	}))
	defer srv.Close()

	_, err := c.Upload(context.Background(), strings.NewReader("not an image"), "x.bin")
	if err == nil {
		t.Fatal("Upload should fail on success:false")
	}
	// The error must carry the api's code and message so logs are useful.
	for _, want := range []string{"5400", "bad image format", "400"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestUploadNonJSONResponse(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>cloudflare gateway error</html>"))
	}))
	defer srv.Close()

	_, err := c.Upload(context.Background(), strings.NewReader("x"), "x.png")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("error = %v, want status 502 surfaced", err)
	}
}

func TestUploadSuccessWithoutID(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{}})
	}))
	defer srv.Close()

	if _, err := c.Upload(context.Background(), strings.NewReader("x"), "x.png"); err == nil {
		t.Error("Upload should fail when the envelope has no id")
	}
}

func TestDelete(t *testing.T) {
	var gotPath, gotMethod string
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{}})
	}))
	defer srv.Close()

	if err := c.Delete(context.Background(), "img-42"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/accounts/acct-1/images/v1/img-42" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
}

func TestDeleteError(t *testing.T) {
	c, srv := testClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors":  []map[string]any{{"code": 5404, "message": "Image not found"}},
		})
	}))
	defer srv.Close()

	err := c.Delete(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "5404") {
		t.Errorf("error = %v, want code 5404 surfaced", err)
	}
}

func TestDeliveryURL(t *testing.T) {
	c := New("acct-1", "tok", "hash-abc")
	got := c.DeliveryURL("2cdc28f0", "public")
	if got != "https://imagedelivery.net/hash-abc/2cdc28f0/public" {
		t.Errorf("DeliveryURL = %q", got)
	}
}
