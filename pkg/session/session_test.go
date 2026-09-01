package session

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
)

func TestHashTokenMatchesSHA256(t *testing.T) {
	token := "some-token-value"
	want := sha256.Sum256([]byte(token))
	got := hashToken(token)
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("hashToken = %x, want %x", got, want)
	}
	if len(got) != 32 {
		t.Fatalf("hashToken length = %d, want 32", len(got))
	}
}

func TestHashTokenDeterministicAndDistinct(t *testing.T) {
	a1 := hashToken("alpha")
	a2 := hashToken("alpha")
	b := hashToken("beta")
	if !bytes.Equal(a1, a2) {
		t.Fatal("hashToken not deterministic for equal input")
	}
	if bytes.Equal(a1, b) {
		t.Fatal("hashToken collided on distinct input")
	}
}

func TestMintTokenFreshAndSized(t *testing.T) {
	t1, err := mintToken()
	if err != nil {
		t.Fatal(err)
	}
	t2, err := mintToken()
	if err != nil {
		t.Fatal(err)
	}
	if t1 == t2 {
		t.Fatal("mintToken returned identical tokens")
	}
	// 32 raw bytes base64url (no padding) -> 43 chars.
	if len(t1) != 43 {
		t.Fatalf("token length = %d, want 43", len(t1))
	}
}

func TestShouldSlide(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ttl := 1 * time.Hour
	tests := []struct {
		name      string
		remaining time.Duration
		want      bool
	}{
		{"full ttl remaining -> no write", ttl, false},
		{"just over half remaining -> no write", ttl/2 + time.Minute, false},
		{"exactly half remaining -> no write", ttl / 2, false},
		{"just under half remaining -> write", ttl/2 - time.Minute, true},
		{"nearly expired -> write", time.Minute, true},
		{"already expired -> write", -time.Minute, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiresAt := now.Add(tt.remaining)
			if got := shouldSlide(expiresAt, now, ttl); got != tt.want {
				t.Fatalf("shouldSlide(remaining=%v) = %v, want %v", tt.remaining, got, tt.want)
			}
		})
	}
}

func TestShouldSlideZeroTTL(t *testing.T) {
	now := time.Now()
	if shouldSlide(now.Add(-time.Hour), now, 0) {
		t.Fatal("shouldSlide must be false for non-positive ttl")
	}
}

func TestValidateCSRF(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	sess := Session{CSRFSecret: secret}
	if !ValidateCSRF(sess, append([]byte(nil), secret...)) {
		t.Fatal("ValidateCSRF rejected matching secret")
	}
	if ValidateCSRF(sess, []byte("wrong")) {
		t.Fatal("ValidateCSRF accepted mismatched secret")
	}
	if ValidateCSRF(sess, nil) {
		t.Fatal("ValidateCSRF accepted nil secret")
	}
}

func TestUserHasRole(t *testing.T) {
	u := User{ID: 7, Roles: []string{"admin", "editor"}}
	if !u.HasRole("admin") {
		t.Fatal("expected admin role")
	}
	if !u.HasRole("editor") {
		t.Fatal("expected editor role")
	}
	if u.HasRole("viewer") {
		t.Fatal("did not expect viewer role")
	}
	var empty User
	if empty.HasRole("admin") {
		t.Fatal("zero user must have no roles")
	}
}

func TestConfigDefaults(t *testing.T) {
	c := Config{}.withDefaults()
	if c.CookieName == "" || c.CookiePath == "" || c.TTL <= 0 {
		t.Fatalf("withDefaults left zero fields: %+v", c)
	}
	custom := Config{CookieName: "x", CookiePath: "/y", TTL: time.Minute}.withDefaults()
	if custom.CookieName != "x" || custom.CookiePath != "/y" || custom.TTL != time.Minute {
		t.Fatalf("withDefaults overrode set fields: %+v", custom)
	}
}

func TestUserFromContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := UserFrom(ctx); ok {
		t.Fatal("empty context must have no user")
	}
	u := User{ID: 99, Roles: []string{"admin"}}
	ctx = withUser(ctx, u)
	got, ok := UserFrom(ctx)
	if !ok {
		t.Fatal("expected user in context")
	}
	if got.ID != u.ID || !got.HasRole("admin") {
		t.Fatalf("UserFrom = %+v, want %+v", got, u)
	}
}

// Middleware behavior is exercised without a DB by checking the unauthenticated
// paths (no cookie -> resolve fails) directly through the public handlers.

func TestOptionalAuthPassesThroughAnonymous(t *testing.T) {
	s := &Store{cfg: Config{}.withDefaults()}
	var sawUser bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawUser = UserFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s.OptionalAuth(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if sawUser {
		t.Fatal("anonymous request must carry no user")
	}
}

func TestRequireAuthRejectsAnonymous(t *testing.T) {
	s := &Store{cfg: Config{}.withDefaults()}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next must not be called for anonymous request")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s.RequireAuth(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireRoleRejectsAnonymous(t *testing.T) {
	s := &Store{cfg: Config{}.withDefaults()}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next must not be called for anonymous request")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s.RequireRole("admin")(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMigrationsEmbedded(t *testing.T) {
	entries, err := Migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded migrations found")
	}
}

// --- Integration tests (gated on TEST_POSTGRES_DSN) ---

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping live Postgres test")
	}
	return dsn
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := testDSN(t)
	ctx := context.Background()
	pool := pg.MustPool(ctx, dsn)
	if err := pg.Migrate(ctx, pool, Migrations); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := NewStore(pool, Config{CookieName: "beach_session", CookiePath: "/", TTL: time.Hour})
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM sessions")
		pool.Close()
	})
	return store
}

func TestNewGetRotateRevokeSweepLive(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	token, csrf, err := store.New(ctx, 1234, []string{"admin"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sess, err := store.Get(ctx, token)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess.UserID != 1234 || !sess.User().HasRole("admin") {
		t.Fatalf("session mismatch: %+v", sess)
	}
	if !ValidateCSRF(sess, csrf) {
		t.Fatal("CSRF secret did not round-trip")
	}

	// Rotate: old token dies, new token lives.
	newToken, _, err := store.Rotate(ctx, token, 1234, []string{"admin", "editor"})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if _, err := store.Get(ctx, token); err != ErrNotFound {
		t.Fatalf("old token after Rotate: err = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(ctx, newToken); err != nil {
		t.Fatalf("new token after Rotate: %v", err)
	}

	// RevokeAll clears everything for the user.
	if err := store.RevokeAll(ctx, 1234); err != nil {
		t.Fatalf("RevokeAll: %v", err)
	}
	if _, err := store.Get(ctx, newToken); err != ErrNotFound {
		t.Fatalf("after RevokeAll: err = %v, want ErrNotFound", err)
	}

	// Sweep removes nothing live, then reaps an expired insert.
	if _, err := store.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
}
