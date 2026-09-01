package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// API/bearer tokens give non-interactive principals (service accounts, CLIs) a
// credential that resolves to the same Principal an interactive login produces.
// A token has the shape "<prefix>.<secret>":
//
//   - prefix is a public, indexed lookup handle — safe to log and to show in a
//     token list.
//   - secret is the bearer credential. Only SHA256(secret) is persisted; the raw
//     token is shown exactly once, at mint time, and never again (secret-column
//     doctrine — it never appears in a RETURNING clause or SELECT *).
//
// Lookup is a single fetch by prefix followed by a constant-time hash compare, so
// the table stays cheap and the comparison cannot be timed.

// ErrInvalidToken is returned when a presented token is malformed, unknown,
// expired, or revoked. The cases collapse to one error so a caller cannot probe
// which tokens exist.
var ErrInvalidToken = errors.New("auth: invalid token")

// tokenPrefixBytes and tokenSecretBytes size the two halves of a minted token.
// The prefix is short (a lookup handle); the secret carries the entropy.
const (
	tokenPrefixBytes = 6
	tokenSecretBytes = 32
)

// Token is a minted API token returned by MintToken. Raw is the full
// "<prefix>.<secret>" string; it is the only time the secret is ever available,
// so the caller must hand it to the user immediately and keep nothing.
type Token struct {
	Raw       string
	Prefix    string
	UserID    ID
	Name      string
	ExpiresAt *time.Time
}

// MintToken creates a token for userID, stores SHA256(secret) keyed by a public
// prefix, and returns the raw "<prefix>.<secret>" string. expiresAt may be nil
// for a non-expiring token. The raw secret is never persisted and cannot be
// recovered later.
func (a *Authenticator) MintToken(ctx context.Context, userID ID, name string, expiresAt *time.Time) (Token, error) {
	prefix, err := randToken(tokenPrefixBytes)
	if err != nil {
		return Token{}, err
	}
	secret, err := randToken(tokenSecretBytes)
	if err != nil {
		return Token{}, err
	}
	hash := hashTokenSecret(secret)

	_, err = a.pool.Exec(ctx,
		`INSERT INTO api_tokens (prefix, token_hash, user_id, name, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		prefix, hash, userID, name, expiresAt)
	if err != nil {
		return Token{}, fmt.Errorf("auth: mint token: %w", err)
	}

	return Token{
		Raw:       prefix + "." + secret,
		Prefix:    prefix,
		UserID:    userID,
		Name:      name,
		ExpiresAt: expiresAt,
	}, nil
}

// tokenRow is the api_tokens lookup result. token_hash is selected explicitly
// (never SELECT *) and is compared in constant time, never returned to callers.
type tokenRow struct {
	userID    ID
	tokenHash []byte
	expiresAt *time.Time
	revokedAt *time.Time
}

// ResolveToken verifies a raw "<prefix>.<secret>" token and, on success, resolves
// the bearer's roles and permissions into a Principal — the same resolution an
// interactive login runs. A malformed, unknown, expired, or revoked token returns
// ErrInvalidToken. On a successful resolve last_used_at is stamped.
func (a *Authenticator) ResolveToken(ctx context.Context, raw string) (*Principal, error) {
	prefix, secret, ok := splitToken(raw)
	if !ok {
		return nil, ErrInvalidToken
	}

	row, found, err := a.lookupToken(ctx, prefix)
	if err != nil {
		return nil, err
	}
	if !found {
		// Spend a hash compare against a throwaway value so an unknown prefix is
		// timing-equivalent to a known one.
		_ = subtle.ConstantTimeCompare(hashTokenSecret(secret), make([]byte, sha256.Size))
		return nil, ErrInvalidToken
	}

	if subtle.ConstantTimeCompare(hashTokenSecret(secret), row.tokenHash) != 1 {
		return nil, ErrInvalidToken
	}

	now := time.Now()
	if row.revokedAt != nil {
		return nil, ErrInvalidToken
	}
	if row.expiresAt != nil && !row.expiresAt.After(now) {
		return nil, ErrInvalidToken
	}

	username, err := a.lookupUsername(ctx, row.userID)
	if err != nil {
		return nil, err
	}

	roles, perms, err := a.resolve(ctx, row.userID)
	if err != nil {
		return nil, err
	}

	if err := a.stampTokenUse(ctx, prefix, now); err != nil {
		return nil, err
	}

	return &Principal{
		UserID:      row.userID,
		Username:    username,
		Roles:       roles,
		Permissions: perms,
	}, nil
}

// RevokeToken marks the token with the given prefix revoked. It is idempotent:
// revoking an unknown or already-revoked prefix is not an error.
func (a *Authenticator) RevokeToken(ctx context.Context, prefix string) error {
	_, err := a.pool.Exec(ctx,
		`UPDATE api_tokens SET revoked_at = now() WHERE prefix = $1 AND revoked_at IS NULL`,
		prefix)
	if err != nil {
		return fmt.Errorf("auth: revoke token: %w", err)
	}
	return nil
}

// lookupToken fetches a token row by prefix with an explicit column list (never
// SELECT *). found is false when no such prefix exists.
func (a *Authenticator) lookupToken(ctx context.Context, prefix string) (tokenRow, bool, error) {
	var r tokenRow
	err := a.pool.QueryRow(ctx,
		`SELECT user_id, token_hash, expires_at, revoked_at
		   FROM api_tokens
		  WHERE prefix = $1`,
		prefix).Scan(&r.userID, &r.tokenHash, &r.expiresAt, &r.revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tokenRow{}, false, nil
		}
		return tokenRow{}, false, fmt.Errorf("auth: lookup token: %w", err)
	}
	return r, true, nil
}

// lookupUsername fetches the username for a resolved token's user id.
func (a *Authenticator) lookupUsername(ctx context.Context, userID ID) (string, error) {
	var username string
	err := a.pool.QueryRow(ctx,
		`SELECT username FROM users WHERE id = $1`, userID).Scan(&username)
	if err != nil {
		return "", fmt.Errorf("auth: lookup token user: %w", err)
	}
	return username, nil
}

// stampTokenUse records the last time a token successfully resolved.
func (a *Authenticator) stampTokenUse(ctx context.Context, prefix string, now time.Time) error {
	_, err := a.pool.Exec(ctx,
		`UPDATE api_tokens SET last_used_at = $1 WHERE prefix = $2`, now, prefix)
	if err != nil {
		return fmt.Errorf("auth: stamp token use: %w", err)
	}
	return nil
}

// splitToken parses a raw token into its prefix and secret halves. ok is false
// when the token is not a single "<prefix>.<secret>" pair with both halves
// present.
func splitToken(raw string) (prefix, secret string, ok bool) {
	prefix, secret, ok = strings.Cut(raw, ".")
	if !ok || prefix == "" || secret == "" {
		return "", "", false
	}
	return prefix, secret, true
}

// hashTokenSecret returns SHA256(secret) — the value stored for a token. The raw
// secret is never persisted. A fast hash is correct here: the secret is 256 bits
// of crypto/rand, so there is nothing to brute-force.
func hashTokenSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// randToken returns n bytes of crypto/rand, base64url-encoded.
func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: rand token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
