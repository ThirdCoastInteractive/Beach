// Package session is the Postgres-backed session store with auth middleware.
//
// See docs/architecture/05-services.md#session. One sessions table, queried
// through the pg pool. The 32-byte crypto/rand token is never stored raw: the
// row's primary key is SHA256(token). Each session carries its own CSRF secret
// (hash-validated server-side); the idle TTL slides on authenticated requests
// (throttled so reads don't turn every GET into a write); Rotate re-mints token
// and CSRF on privilege change; a periodic Sweep reaps dead rows.
//
// This package sits below auth: it knows a user id and the role slugs the
// session was minted with, nothing more. The richer auth.Principal is derived
// from this by the auth layer.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by Get when no live session matches the token.
var ErrNotFound = errors.New("session: not found")

// tokenBytes is the length of the raw random token, per the hardening doctrine.
const tokenBytes = 32

// User is the identity a session carries. It is deliberately small: the id and
// the role slugs the session was minted with. The auth layer derives its richer
// Principal (permissions, username) from this.
type User struct {
	ID    int64
	Roles []string
}

// HasRole reports whether the user was minted with the given role slug.
func (u User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Session is a live session row, as returned by Get.
type Session struct {
	UserID     int64
	Roles      []string
	CSRFSecret []byte
	ExpiresAt  time.Time
}

// User returns the identity view of the session.
func (s Session) User() User {
	return User{ID: s.UserID, Roles: s.Roles}
}

// Config is the plain cookie/TTL configuration. There is no multi-realm
// machinery: one cookie, one TTL.
type Config struct {
	CookieName string        // e.g. "beach_session"
	CookiePath string        // e.g. "/"
	TTL        time.Duration // idle lifetime; sliding on authenticated requests
}

// withDefaults fills zero fields with sensible defaults so a zero Config works.
func (c Config) withDefaults() Config {
	if c.CookieName == "" {
		c.CookieName = "beach_session"
	}
	if c.CookiePath == "" {
		c.CookiePath = "/"
	}
	if c.TTL <= 0 {
		c.TTL = 14 * 24 * time.Hour
	}
	return c
}

// Store is the session store. Construct with NewStore.
type Store struct {
	pool *pgxpool.Pool
	cfg  Config
}

// NewStore builds a Store over the given pool and config.
func NewStore(pool *pgxpool.Pool, cfg Config) *Store {
	return &Store{pool: pool, cfg: cfg.withDefaults()}
}

// Config returns the resolved (defaulted) config.
func (s *Store) Config() Config { return s.cfg }

// mintToken returns a fresh 32-byte token, base64url-encoded for cookie use.
func mintToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session: mint token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken returns SHA256(token) — the value stored as the row primary key. The
// raw token is never persisted.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// mintCSRF returns a fresh 32-byte CSRF secret.
func mintCSRF() ([]byte, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("session: mint csrf: %w", err)
	}
	return b, nil
}

// New mints a session for userID with the given role slugs. It returns the raw
// token (to set in the cookie) and the CSRF secret. Only SHA256(token) is
// stored. expires_at is now()+TTL.
func (s *Store) New(ctx context.Context, userID int64, roles []string) (token string, csrf []byte, err error) {
	token, err = mintToken()
	if err != nil {
		return "", nil, err
	}
	csrf, err = mintCSRF()
	if err != nil {
		return "", nil, err
	}
	if roles == nil {
		roles = []string{}
	}
	expires := time.Now().Add(s.cfg.TTL)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, roles, csrf_secret, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		hashToken(token), userID, roles, csrf, expires)
	if err != nil {
		return "", nil, fmt.Errorf("session: new: %w", err)
	}
	return token, csrf, nil
}

// Get looks up a live session by raw token, filtering expires_at > now(). It
// returns ErrNotFound when no live row matches. On an authenticated hit it
// slides the TTL via a throttled UPDATE (see slideExpiry).
func (s *Store) Get(ctx context.Context, token string) (Session, error) {
	now := time.Now()
	var sess Session
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, roles, csrf_secret, expires_at
		   FROM sessions
		  WHERE token_hash = $1 AND expires_at > $2`,
		hashToken(token), now).Scan(&sess.UserID, &sess.Roles, &sess.CSRFSecret, &sess.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, ErrNotFound
		}
		return Session{}, fmt.Errorf("session: get: %w", err)
	}

	// Sliding TTL, throttled: only write once the session is past half its life.
	if shouldSlide(sess.ExpiresAt, now, s.cfg.TTL) {
		newExpiry := now.Add(s.cfg.TTL)
		if _, err := s.pool.Exec(ctx,
			`UPDATE sessions SET expires_at = $1 WHERE token_hash = $2`,
			newExpiry, hashToken(token)); err != nil {
			return Session{}, fmt.Errorf("session: slide: %w", err)
		}
		sess.ExpiresAt = newExpiry
	}
	return sess, nil
}

// shouldSlide decides whether a sliding-TTL UPDATE is warranted. To avoid
// turning every authenticated GET into a write, we only slide once the session
// has burned more than half its TTL — i.e. the remaining life is less than half
// the full TTL. With a full window of remaining life nothing is written.
func shouldSlide(expiresAt, now time.Time, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	remaining := expiresAt.Sub(now)
	return remaining < ttl/2
}

// ValidateCSRF constant-time compares a presented CSRF secret against the
// session's stored secret.
func ValidateCSRF(sess Session, presented []byte) bool {
	return subtle.ConstantTimeCompare(sess.CSRFSecret, presented) == 1
}

// Rotate re-mints token and CSRF on a privilege change: the old row is deleted
// and a new row inserted in one transaction. It returns the new raw token and
// CSRF secret. Roles may be updated at the same time.
func (s *Store) Rotate(ctx context.Context, oldToken string, userID int64, roles []string) (token string, csrf []byte, err error) {
	token, err = mintToken()
	if err != nil {
		return "", nil, err
	}
	csrf, err = mintCSRF()
	if err != nil {
		return "", nil, err
	}
	if roles == nil {
		roles = []string{}
	}
	expires := time.Now().Add(s.cfg.TTL)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("session: rotate begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx,
		`DELETE FROM sessions WHERE token_hash = $1`, hashToken(oldToken)); err != nil {
		return "", nil, fmt.Errorf("session: rotate delete: %w", err)
	}
	if _, err = tx.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, roles, csrf_secret, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		hashToken(token), userID, roles, csrf, expires); err != nil {
		return "", nil, fmt.Errorf("session: rotate insert: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", nil, fmt.Errorf("session: rotate commit: %w", err)
	}
	return token, csrf, nil
}

// RevokeAll deletes every session for a user — a single indexed DELETE. Used on
// logout-everywhere and on role changes.
func (s *Store) RevokeAll(ctx context.Context, userID int64) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("session: revoke all: %w", err)
	}
	return nil
}

// Revoke deletes a single session by raw token. Idempotent.
func (s *Store) Revoke(ctx context.Context, token string) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE token_hash = $1`, hashToken(token)); err != nil {
		return fmt.Errorf("session: revoke: %w", err)
	}
	return nil
}

// Sweep reaps expired rows: DELETE ... WHERE expires_at < now(). Returns the
// number of rows removed. Call periodically from a background ticker.
func (s *Store) Sweep(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE expires_at < $1`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("session: sweep: %w", err)
	}
	return tag.RowsAffected(), nil
}

// --- middleware ---

// ctxKey is the unexported context key type for the session/user.
type ctxKey struct{}

// withUser returns a copy of ctx carrying the session's user identity.
func withUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the authenticated user from ctx and whether one is present.
// Anonymous requests carry no user.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// resolve looks up the session for the request's cookie. ok is false on a
// missing cookie or a missing/expired session.
func (s *Store) resolve(r *http.Request) (User, bool) {
	c, err := r.Cookie(s.cfg.CookieName)
	if err != nil || c.Value == "" {
		return User{}, false
	}
	sess, err := s.Get(r.Context(), c.Value)
	if err != nil {
		return User{}, false
	}
	return sess.User(), true
}

// OptionalAuth attaches the user to the request context when a valid session
// cookie is present, and passes through unauthenticated requests untouched.
// Anonymous is the absence of a user, not an error.
func (s *Store) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := s.resolve(r); ok {
			r = r.WithContext(withUser(r.Context(), u))
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth rejects requests without a valid session with 401, otherwise
// attaches the user and continues.
func (s *Store) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.resolve(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(withUser(r.Context(), u))
		next.ServeHTTP(w, r)
	})
}

// RequireRole rejects requests lacking the given role: 401 when unauthenticated,
// 403 when authenticated but missing the role.
func (s *Store) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := s.resolve(r)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if !u.HasRole(role) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r = r.WithContext(withUser(r.Context(), u))
			next.ServeHTTP(w, r)
		})
	}
}
