package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/the-beach/passwords"
	"github.com/ThirdCoastInteractive/the-beach/session"
)

func TestHasPermission(t *testing.T) {
	p := &Principal{Permissions: []string{"pantry:read", "pantry:write"}}
	tests := []struct {
		perm string
		want bool
	}{
		{"pantry:read", true},
		{"pantry:write", true},
		{"pantry:delete", false},
		{"", false},
		{"pantry", false},
	}
	for _, tt := range tests {
		if got := p.HasPermission(tt.perm); got != tt.want {
			t.Errorf("HasPermission(%q) = %v, want %v", tt.perm, got, tt.want)
		}
	}
}

func TestHasPermissionNil(t *testing.T) {
	var p *Principal // anonymous
	if p.HasPermission("pantry:read") {
		t.Fatal("nil principal must not hold any permission")
	}
	if p.Can("pantry:read") {
		t.Fatal("nil principal Can must be false")
	}
	if p.HasAnyOf("a:b", "c:d") {
		t.Fatal("nil principal HasAnyOf must be false")
	}
	if p.HasRole("admin") {
		t.Fatal("nil principal HasRole must be false")
	}
}

func TestCanGuard(t *testing.T) {
	p := &Principal{Permissions: []string{"orders:read"}}
	if !p.Can("orders:read") {
		t.Error("Can(orders:read) should be true")
	}
	if p.Can("orders:write") {
		t.Error("Can(orders:write) should be false")
	}
}

func TestHasAnyOf(t *testing.T) {
	p := &Principal{Permissions: []string{"a:read"}}
	tests := []struct {
		name  string
		perms []string
		want  bool
	}{
		{"first matches", []string{"a:read", "b:read"}, true},
		{"second matches", []string{"b:read", "a:read"}, true},
		{"none match", []string{"b:read", "c:read"}, false},
		{"empty list", nil, false},
	}
	for _, tt := range tests {
		if got := p.HasAnyOf(tt.perms...); got != tt.want {
			t.Errorf("%s: HasAnyOf(%v) = %v, want %v", tt.name, tt.perms, got, tt.want)
		}
	}
}

func TestHasRole(t *testing.T) {
	p := &Principal{Roles: []string{"admin", "editor"}}
	if !p.HasRole("admin") {
		t.Error("should have admin")
	}
	if !p.HasRole("editor") {
		t.Error("should have editor")
	}
	if p.HasRole("viewer") {
		t.Error("should not have viewer")
	}
}

func TestPrincipalContextRoundTrip(t *testing.T) {
	ctx := context.Background()

	// Anonymous: nothing attached.
	if p, ok := PrincipalFrom(ctx); ok || p != nil {
		t.Fatal("empty context must be anonymous")
	}

	// A nil principal explicitly stored is still anonymous.
	if p, ok := PrincipalFrom(WithPrincipal(ctx, nil)); ok || p != nil {
		t.Fatal("nil principal must read back as anonymous")
	}

	want := &Principal{UserID: 7, Username: "alice", Permissions: []string{"x:y"}}
	got, ok := PrincipalFrom(WithPrincipal(ctx, want))
	if !ok {
		t.Fatal("principal should be present")
	}
	if got != want {
		t.Fatalf("principal round-trip mismatch: got %+v", got)
	}
}

func TestPretokenRoundTrip(t *testing.T) {
	secret := []byte("framework-signing-key")
	now := time.Unix(1_700_000_000, 0)

	pt := MintPretoken(secret, now)
	if !VerifyPretoken(secret, pt.Value, now) {
		t.Fatal("fresh pretoken should verify at mint time")
	}
	// Within the window.
	if !VerifyPretoken(secret, pt.Value, now.Add(9*time.Minute)) {
		t.Error("pretoken should verify inside the 10m window")
	}
	// Expired.
	if VerifyPretoken(secret, pt.Value, now.Add(11*time.Minute)) {
		t.Error("pretoken should be rejected after expiry")
	}
	// Wrong secret.
	if VerifyPretoken([]byte("other-key"), pt.Value, now) {
		t.Error("pretoken signed by a different key must be rejected")
	}
}

func TestPretokenMalformed(t *testing.T) {
	secret := []byte("k")
	now := time.Unix(1_700_000_000, 0)
	bad := []string{
		"",
		"no-dot",
		"notanumber.AAAA",
		"1700000000.!!!not-base64!!!",
		"1700000000.",
	}
	for _, v := range bad {
		if VerifyPretoken(secret, v, now) {
			t.Errorf("malformed pretoken %q must not verify", v)
		}
	}
}

func TestNextLockout(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cfg := LockoutConfig{Threshold: 5, Duration: 15 * time.Minute}

	if got := nextLockout(4, cfg, now); got != nil {
		t.Errorf("below threshold should not lock, got %v", got)
	}
	got := nextLockout(5, cfg, now)
	if got == nil {
		t.Fatal("at threshold should lock")
	}
	if !got.Equal(now.Add(15 * time.Minute)) {
		t.Errorf("lockout expiry = %v, want %v", got, now.Add(15*time.Minute))
	}
	if got := nextLockout(9, cfg, now); got == nil {
		t.Error("above threshold should still lock")
	}
}

func TestLockoutConfigDefaults(t *testing.T) {
	c := LockoutConfig{}.withDefaults()
	if c.Threshold != 5 {
		t.Errorf("default threshold = %d, want 5", c.Threshold)
	}
	if c.Duration != 15*time.Minute {
		t.Errorf("default duration = %v, want 15m", c.Duration)
	}
}

// --- DB-gated integration test (skips offline) ---

func TestLoginIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN unset; skipping live-DB login test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	hash, err := passwords.Hash("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	// Minimal fixtures. Assumes the auth + session migrations have been applied.
	var uid ID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username) VALUES ($1) RETURNING id`,
		"test_login_user").Scan(&uid); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, uid) })

	if _, err := pool.Exec(ctx,
		`INSERT INTO user_credentials_local (user_id, password_hash) VALUES ($1, $2)`,
		uid, hash); err != nil {
		t.Fatalf("insert credentials: %v", err)
	}

	var roleID ID
	if err := pool.QueryRow(ctx,
		`INSERT INTO roles (slug) VALUES ('test_role') RETURNING id`).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID) })
	if _, err := pool.Exec(ctx,
		`INSERT INTO role_permissions (role_id, permission) VALUES ($1, 'pantry:write')`, roleID); err != nil {
		t.Fatalf("insert role_permission: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, uid, roleID); err != nil {
		t.Fatalf("insert user_role: %v", err)
	}

	store := session.NewStore(pool, session.Config{})
	a := NewAuthenticator(pool, store, LockoutConfig{})

	// Wrong password.
	if _, _, _, err := a.Login(ctx, "test_login_user", "wrong"); err != ErrInvalidCredentials {
		t.Errorf("wrong password: got %v, want ErrInvalidCredentials", err)
	}
	// Unknown user.
	if _, _, _, err := a.Login(ctx, "nobody", "whatever"); err != ErrInvalidCredentials {
		t.Errorf("unknown user: got %v, want ErrInvalidCredentials", err)
	}
	// Success.
	token, csrf, p, err := a.Login(ctx, "test_login_user", "correct-horse-battery")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" || len(csrf) == 0 {
		t.Error("expected non-empty token and csrf")
	}
	if p == nil || !p.Can("pantry:write") || !p.HasRole("test_role") {
		t.Errorf("resolved principal wrong: %+v", p)
	}
}
