package beach

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSPDefaultStrict(t *testing.T) {
	s := CSPDefault().String()
	if !strings.Contains(s, "default-src 'self'") {
		t.Fatalf("default CSP missing default-src self: %q", s)
	}
	if !strings.Contains(s, "object-src 'none'") {
		t.Fatalf("default CSP missing object-src none: %q", s)
	}
}

func TestCSPAllowDoesNotMutateBase(t *testing.T) {
	base := CSPDefault()
	widened := base.AllowMedia("https://cdn.example.com")
	if strings.Contains(base.String(), "cdn.example.com") {
		t.Fatal("Allow mutated the base preset")
	}
	if !strings.Contains(widened.String(), "cdn.example.com") {
		t.Fatal("widened CSP missing the new source")
	}
}

func TestCSPDeduplicates(t *testing.T) {
	c := CSPDefault().AllowImg("data:").AllowImg("data:")
	got := c.String()
	if strings.Count(got, "data:") != 1 {
		t.Fatalf("img-src data: not de-duplicated: %q", got)
	}
}

type bindReq struct {
	Email string `form:"email" json:"email"`
	Qty   int    `form:"qty" json:"qty"`
}

func (b *bindReq) Validate() error {
	if b.Email == "" {
		return Invalid("email", "required")
	}
	return nil
}

func TestBindJSON(t *testing.T) {
	a := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"email":"a@b.c","qty":3}`))
	req.Header.Set("Content-Type", "application/json")
	c := a.newCtx(httptest.NewRecorder(), req)

	got, err := Bind[bindReq](c)
	if err != nil {
		t.Fatalf("bind json: %v", err)
	}
	if got.Email != "a@b.c" || got.Qty != 3 {
		t.Fatalf("bind json got %+v", got)
	}
}

func TestBindJSONIgnoresUnknownSignals(t *testing.T) {
	a := New(Config{})
	// Datastar posts a superset of signals; binding must not hard-fail.
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"email":"a@b.c","qty":1,"extra":true}`))
	req.Header.Set("Content-Type", "application/json")
	c := a.newCtx(httptest.NewRecorder(), req)
	got, err := Bind[bindReq](c)
	if err != nil {
		t.Fatalf("bind with extra signal failed: %v", err)
	}
	if got.Email != "a@b.c" {
		t.Fatalf("bind got %+v", got)
	}
}

func TestBindForm(t *testing.T) {
	a := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("email=x@y.z&qty=7"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := a.newCtx(httptest.NewRecorder(), req)
	got, err := Bind[bindReq](c)
	if err != nil {
		t.Fatalf("bind form: %v", err)
	}
	if got.Email != "x@y.z" || got.Qty != 7 {
		t.Fatalf("bind form got %+v", got)
	}
}

func TestBindValidateFails(t *testing.T) {
	a := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"qty":1}`))
	req.Header.Set("Content-Type", "application/json")
	c := a.newCtx(httptest.NewRecorder(), req)
	_, err := Bind[bindReq](c)
	if err == nil {
		t.Fatal("expected validation error for missing email")
	}
	if statusForError(err) != http.StatusBadRequest {
		t.Fatalf("validation error status = %d, want 400", statusForError(err))
	}
}

func TestStatusForError(t *testing.T) {
	cases := map[error]int{
		ErrNotFound:       http.StatusNotFound,
		ErrForbidden:      http.StatusForbidden,
		ErrUnauthorized:   http.StatusUnauthorized,
		ErrBadRequest:     http.StatusBadRequest,
		Invalid("f", "m"): http.StatusBadRequest,
	}
	for err, want := range cases {
		if got := statusForError(err); got != want {
			t.Fatalf("statusForError(%v) = %d, want %d", err, got, want)
		}
	}
}
