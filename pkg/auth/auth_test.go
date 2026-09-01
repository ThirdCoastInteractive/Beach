package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/pkg/passwords"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
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

func TestAnonymousPrincipal(t *testing.T) {
	p := AnonymousPrincipal("anon-xyz")
	if p == nil {
		t.Fatal("AnonymousPrincipal returned nil")
	}
	if !p.IsAnonymous() {
		t.Fatal("anonymous principal must report IsAnonymous()")
	}
	if p.AnonID != "anon-xyz" {
		t.Fatalf("AnonID = %q, want anon-xyz", p.AnonID)
	}
	// No privileges: every authz check denies it.
	if p.Can("x:y") || p.HasRole("admin") || p.HasPermission("x:y") || p.InScope("cust-1") {
		t.Fatal("anonymous principal must hold no roles, permissions, or scope")
	}
}

func TestIsAnonymousDistinguishesKinds(t *testing.T) {
	var absent *Principal // unauthenticated, not the anonymous kind
	if absent.IsAnonymous() {
		t.Fatal("a nil (absent) principal is unauthenticated, not anonymous")
	}
	authed := &Principal{UserID: 1, Username: "alice"}
	if authed.IsAnonymous() {
		t.Fatal("a real principal must not report IsAnonymous()")
	}
	if !AnonymousPrincipal("id").IsAnonymous() {
		t.Fatal("the anonymous kind must report IsAnonymous()")
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

func TestInScope(t *testing.T) {
	tests := []struct {
		name  string
		p     *Principal
		scope string
		want  bool
	}{
		{"match", &Principal{Scope: "cust-1"}, "cust-1", true},
		{"wrong scope", &Principal{Scope: "cust-1"}, "cust-2", false},
		{"unscoped principal", &Principal{Scope: ""}, "cust-1", false},
		{"empty target rejects unscoped", &Principal{Scope: ""}, "", false},
		{"nil principal", nil, "cust-1", false},
	}
	for _, tt := range tests {
		if got := tt.p.InScope(tt.scope); got != tt.want {
			t.Errorf("%s: InScope(%q) = %v, want %v", tt.name, tt.scope, got, tt.want)
		}
	}
}

func TestSplitToken(t *testing.T) {
	tests := []struct {
		raw        string
		wantPrefix string
		wantSecret string
		wantOK     bool
	}{
		{"pfx.secret", "pfx", "secret", true},
		{"pfx.sec.ret", "pfx", "sec.ret", true}, // only the first dot splits
		{"nodot", "", "", false},
		{".secret", "", "", false},
		{"prefix.", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		p, s, ok := splitToken(tt.raw)
		if p != tt.wantPrefix || s != tt.wantSecret || ok != tt.wantOK {
			t.Errorf("splitToken(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.raw, p, s, ok, tt.wantPrefix, tt.wantSecret, tt.wantOK)
		}
	}
}

func TestHashTokenSecret(t *testing.T) {
	// Hash is SHA256(secret): deterministic, fixed-width, and not the raw secret.
	h := hashTokenSecret("a-secret")
	if len(h) != sha256.Size {
		t.Fatalf("hash length = %d, want %d", len(h), sha256.Size)
	}
	want := sha256.Sum256([]byte("a-secret"))
	if !bytes.Equal(h, want[:]) {
		t.Fatal("hashTokenSecret is not SHA256(secret)")
	}
	if bytes.Contains(h, []byte("a-secret")) {
		t.Fatal("raw secret leaked into the hash")
	}
	// A different secret hashes differently.
	if bytes.Equal(h, hashTokenSecret("another-secret")) {
		t.Fatal("distinct secrets must hash differently")
	}
}

func TestRandTokenIsRandomAndURLSafe(t *testing.T) {
	a, err := randToken(tokenSecretBytes)
	if err != nil {
		t.Fatalf("randToken: %v", err)
	}
	b, err := randToken(tokenSecretBytes)
	if err != nil {
		t.Fatalf("randToken: %v", err)
	}
	if a == b {
		t.Fatal("two minted secrets collided — not random")
	}
	// base64url: no '.', so the "<prefix>.<secret>" delimiter is unambiguous.
	if strings.ContainsAny(a, ".+/=") {
		t.Fatalf("token %q contains non-url-safe characters", a)
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

// TestAppSuppliedUserID checks the identity migration accepts both a DB-minted id
// (insert omits id) and an externally supplied 64-bit id (insert supplies id).
func TestAppSuppliedUserID(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN unset; skipping live-DB identity test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// DB-minted: omit id, get one back. (The existing example-app login path.)
	var dbID ID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username) VALUES ($1) RETURNING id`,
		"test_dbminted_user").Scan(&dbID); err != nil {
		t.Fatalf("db-minted insert: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, dbID) })
	if dbID == 0 {
		t.Error("db-minted id should be non-zero")
	}

	// App-minted: supply a large 64-bit id explicitly (GENERATED BY DEFAULT).
	const appID ID = 9_000_000_000_000_000_001
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username) VALUES ($1, $2)`,
		appID, "test_appminted_user"); err != nil {
		t.Fatalf("app-minted insert: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, appID) })

	var got ID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM users WHERE username = $1`, "test_appminted_user").Scan(&got); err != nil {
		t.Fatalf("read app-minted user: %v", err)
	}
	if got != appID {
		t.Errorf("app-minted id = %d, want %d", got, appID)
	}
}

// TestTokenMintResolveRevoke exercises the API-token lifecycle against a live DB:
// mint stores only a hash, resolve returns the principal, and a wrong/revoked
// token resolves to ErrInvalidToken.
func TestTokenMintResolveRevoke(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN unset; skipping live-DB token test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	var uid ID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username) VALUES ($1) RETURNING id`,
		"test_token_user").Scan(&uid); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, uid) })

	var roleID ID
	if err := pool.QueryRow(ctx,
		`INSERT INTO roles (slug) VALUES ('token_role') RETURNING id`).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID) })
	if _, err := pool.Exec(ctx,
		`INSERT INTO role_permissions (role_id, permission) VALUES ($1, 'fleet:read')`, roleID); err != nil {
		t.Fatalf("insert role_permission: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, uid, roleID); err != nil {
		t.Fatalf("insert user_role: %v", err)
	}

	store := session.NewStore(pool, session.Config{})
	a := NewAuthenticator(pool, store, LockoutConfig{})

	tok, err := a.MintToken(ctx, uid, "ci-token", nil)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	if tok.Raw == "" || !strings.Contains(tok.Raw, ".") {
		t.Fatalf("minted token should be a non-empty prefix.secret: %q", tok.Raw)
	}

	// The raw secret must not be persisted — only its hash.
	_, secret, _ := splitToken(tok.Raw)
	var rawHits int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM api_tokens WHERE encode(token_hash, 'escape') = $1`, secret).Scan(&rawHits); err != nil {
		t.Fatalf("scan raw-secret check: %v", err)
	}
	if rawHits != 0 {
		t.Error("raw secret found in api_tokens.token_hash — secret-column doctrine violated")
	}

	// Resolve the good token to a principal.
	p, err := a.ResolveToken(ctx, tok.Raw)
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if p == nil || p.UserID != uid || !p.Can("fleet:read") || p.Username != "test_token_user" {
		t.Errorf("resolved token principal wrong: %+v", p)
	}

	// A tampered secret is rejected.
	if _, err := a.ResolveToken(ctx, tok.Prefix+".not-the-secret"); err != ErrInvalidToken {
		t.Errorf("tampered token: got %v, want ErrInvalidToken", err)
	}
	// A malformed token is rejected.
	if _, err := a.ResolveToken(ctx, "garbage"); err != ErrInvalidToken {
		t.Errorf("malformed token: got %v, want ErrInvalidToken", err)
	}

	// Revoke, then the good token no longer resolves.
	if err := a.RevokeToken(ctx, tok.Prefix); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if _, err := a.ResolveToken(ctx, tok.Raw); err != ErrInvalidToken {
		t.Errorf("revoked token: got %v, want ErrInvalidToken", err)
	}
}
