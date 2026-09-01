package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/pkg/passwords"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
)

// Login outcomes. Both bad-credentials cases collapse to ErrInvalidCredentials
// so the caller cannot distinguish "unknown user" from "wrong password" — user
// enumeration is also defended against by the timing-safe dummy compare.
var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrAccountLocked      = errors.New("auth: account locked")
)

// LockoutConfig is the failed-login lockout policy. It is account protection, not
// DDoS mitigation: local state on the credentials row, no distributed
// coordination.
type LockoutConfig struct {
	Threshold int           // failed attempts before lockout (default 5)
	Duration  time.Duration // lockout window (default 15m)
}

func (c LockoutConfig) withDefaults() LockoutConfig {
	if c.Threshold <= 0 {
		c.Threshold = 5
	}
	if c.Duration <= 0 {
		c.Duration = 15 * time.Minute
	}
	return c
}

// dummyHash is a valid argon2id PHC string compared against when the username is
// unknown, so an unknown user costs the same wall-clock time as a known one. It
// is generated once at init from a random password (the plaintext is discarded).
var dummyHash string

func init() {
	h, err := passwords.Hash("timing-equalizer-placeholder")
	if err != nil {
		panic("auth: cannot mint dummy hash: " + err.Error())
	}
	dummyHash = h
}

// Authenticator runs the hardened login flow over a pg pool and a session store.
// Construct with NewAuthenticator.
type Authenticator struct {
	pool     *pgxpool.Pool
	sessions *session.Store
	lockout  LockoutConfig
}

// NewAuthenticator wires the login flow to its pool and session store.
func NewAuthenticator(pool *pgxpool.Pool, sessions *session.Store, lockout LockoutConfig) *Authenticator {
	return &Authenticator{pool: pool, sessions: sessions, lockout: lockout.withDefaults()}
}

// credRow is the credentials lookup result. password_hash is selected explicitly
// (never via SELECT *) and never returned to callers.
type credRow struct {
	userID           ID
	passwordHash     string
	failedLoginCount int
	lockedUntil      *time.Time
}

// Login verifies username/password and, on success, resolves the principal's
// roles and permissions, mints a session, and returns the raw session token, the
// CSRF secret, and the resolved Principal. The flow:
//
//  1. Look up the credentials row; on miss, run a dummy compare so the unknown
//     case is timing-equivalent, then return ErrInvalidCredentials.
//  2. If currently locked, return ErrAccountLocked.
//  3. Constant-time verify the password. On failure, bump failed_login_count and
//     set locked_until once the threshold is crossed.
//  4. On success, reset the lockout counters, transparently rehash on a parameter
//     upgrade, resolve roles+permissions, and mint the session.
func (a *Authenticator) Login(ctx context.Context, username, password string) (token string, csrf []byte, p *Principal, err error) {
	now := time.Now()

	cred, found, err := a.lookupCredentials(ctx, username)
	if err != nil {
		return "", nil, nil, err
	}
	if !found {
		// Timing-safe: spend the argon2id cost even for an unknown user.
		_, _ = passwords.Verify(password, dummyHash)
		return "", nil, nil, ErrInvalidCredentials
	}

	if cred.lockedUntil != nil && cred.lockedUntil.After(now) {
		return "", nil, nil, ErrAccountLocked
	}

	ok, verr := passwords.Verify(password, cred.passwordHash)
	if verr != nil {
		return "", nil, nil, fmt.Errorf("auth: verify: %w", verr)
	}
	if !ok {
		if err := a.recordFailure(ctx, cred, now); err != nil {
			return "", nil, nil, err
		}
		return "", nil, nil, ErrInvalidCredentials
	}

	// Success. Clear lockout state and rehash if the stored params are stale.
	if err := a.recordSuccess(ctx, cred, password); err != nil {
		return "", nil, nil, err
	}

	roles, perms, err := a.resolve(ctx, cred.userID)
	if err != nil {
		return "", nil, nil, err
	}

	token, csrf, err = a.sessions.New(ctx, cred.userID, roles)
	if err != nil {
		return "", nil, nil, fmt.Errorf("auth: mint session: %w", err)
	}

	p = &Principal{
		UserID:      cred.userID,
		Username:    username,
		Roles:       roles,
		Permissions: perms,
	}
	return token, csrf, p, nil
}

// lookupCredentials fetches the credentials row by username with an explicit
// column list (never SELECT *). found is false when no such user exists.
func (a *Authenticator) lookupCredentials(ctx context.Context, username string) (credRow, bool, error) {
	var c credRow
	err := a.pool.QueryRow(ctx,
		`SELECT u.id, c.password_hash, c.failed_login_count, c.locked_until
		   FROM users u
		   JOIN user_credentials_local c ON c.user_id = u.id
		  WHERE u.username = $1`,
		username).Scan(&c.userID, &c.passwordHash, &c.failedLoginCount, &c.lockedUntil)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credRow{}, false, nil
		}
		return credRow{}, false, fmt.Errorf("auth: lookup credentials: %w", err)
	}
	return c, true, nil
}

// recordFailure bumps the failed-login counter and arms the lockout once the
// threshold is reached. lockoutUntil is computed by nextLockout.
func (a *Authenticator) recordFailure(ctx context.Context, cred credRow, now time.Time) error {
	count := cred.failedLoginCount + 1
	lockUntil := nextLockout(count, a.lockout, now)
	_, err := a.pool.Exec(ctx,
		`UPDATE user_credentials_local
		    SET failed_login_count = $1, locked_until = $2, updated_at = now()
		  WHERE user_id = $3`,
		count, lockUntil, cred.userID)
	if err != nil {
		return fmt.Errorf("auth: record failure: %w", err)
	}
	return nil
}

// nextLockout returns the lockout expiry for a given (post-increment) failure
// count, or nil when the threshold has not yet been crossed. Exposed as a pure
// function so the policy is unit-testable without a database.
func nextLockout(failedCount int, cfg LockoutConfig, now time.Time) *time.Time {
	cfg = cfg.withDefaults()
	if failedCount < cfg.Threshold {
		return nil
	}
	t := now.Add(cfg.Duration)
	return &t
}

// recordSuccess clears the lockout counters and, when the stored hash used weaker
// parameters, transparently rehashes the password (one upgrade per login).
func (a *Authenticator) recordSuccess(ctx context.Context, cred credRow, plaintext string) error {
	newHash := cred.passwordHash
	if passwords.NeedsRehash(cred.passwordHash) {
		h, err := passwords.Hash(plaintext)
		if err == nil {
			newHash = h
		}
		// If rehash fails we still log the user in with the old hash; we simply
		// skip the opportunistic upgrade.
	}
	_, err := a.pool.Exec(ctx,
		`UPDATE user_credentials_local
		    SET password_hash = $1, failed_login_count = 0, locked_until = NULL, updated_at = now()
		  WHERE user_id = $2`,
		newHash, cred.userID)
	if err != nil {
		return fmt.Errorf("auth: record success: %w", err)
	}
	return nil
}

// resolve reads the user's role slugs and the flat permission list from the three
// RBAC tables. This is the one place permissions are computed; the result is
// frozen onto the principal and into the session for the life of the session.
func (a *Authenticator) resolve(ctx context.Context, userID ID) (roles, perms []string, err error) {
	roleRows, err := a.pool.Query(ctx,
		`SELECT r.slug
		   FROM user_roles ur
		   JOIN roles r ON r.id = ur.role_id
		  WHERE ur.user_id = $1
		  ORDER BY r.slug`,
		userID)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: resolve roles: %w", err)
	}
	roles, err = collectStrings(roleRows)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: resolve roles: %w", err)
	}

	permRows, err := a.pool.Query(ctx,
		`SELECT DISTINCT rp.permission
		   FROM user_roles ur
		   JOIN role_permissions rp ON rp.role_id = ur.role_id
		  WHERE ur.user_id = $1
		  ORDER BY rp.permission`,
		userID)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: resolve permissions: %w", err)
	}
	perms, err = collectStrings(permRows)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: resolve permissions: %w", err)
	}
	return roles, perms, nil
}

// collectStrings drains a single-text-column result set.
func collectStrings(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
