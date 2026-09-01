package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/pkg/passwords"
	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
)

// Registration outcomes.
var (
	// ErrUsernameTaken is returned when the username already exists.
	ErrUsernameTaken = errors.New("auth: username taken")
	// ErrInvalidUsername is returned when the username fails validation.
	ErrInvalidUsername = errors.New("auth: invalid username")
)

// pgUniqueViolation is the SQLSTATE for a unique-constraint violation.
const pgUniqueViolation = "23505"

// RegisterUser creates a local-password account: it validates the username,
// hashes the password with argon2id (passwords.Hash enforces the length bounds),
// and inserts the users + user_credentials_local rows in one transaction. It
// returns ErrUsernameTaken if the username is already in use, ErrInvalidUsername
// for a malformed username, and the passwords package's length errors verbatim.
//
// This is the turnkey counterpart to Authenticator.Login: an app's signup handler
// calls RegisterUser, then Login to issue the first session.
func RegisterUser(ctx context.Context, pool *pgxpool.Pool, username, password string) (ID, error) {
	username = strings.TrimSpace(username)
	if !ValidUsername(username) {
		return 0, ErrInvalidUsername
	}
	hash, err := passwords.Hash(password)
	if err != nil {
		return 0, err
	}

	var id ID
	err = pg.InTx(ctx, pool, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx,
			`INSERT INTO users (username) VALUES ($1) RETURNING id`, username).Scan(&id); e != nil {
			var pgErr *pgconn.PgError
			if errors.As(e, &pgErr) && pgErr.Code == pgUniqueViolation {
				return ErrUsernameTaken
			}
			return fmt.Errorf("auth: insert user: %w", e)
		}
		if _, e := tx.Exec(ctx,
			`INSERT INTO user_credentials_local (user_id, password_hash) VALUES ($1, $2)`, id, hash); e != nil {
			return fmt.Errorf("auth: insert credentials: %w", e)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// ValidUsername reports whether s is an acceptable username: 3–32 characters of
// ASCII letters, digits, underscore, or hyphen, beginning with a letter or digit.
func ValidUsername(s string) bool {
	if len(s) < 3 || len(s) > 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		switch {
		case isAlnum:
		case (c == '_' || c == '-') && i > 0: // not as the first character
		default:
			return false
		}
	}
	return true
}
