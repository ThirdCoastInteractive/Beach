package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/ThirdCoastInteractive/Beach/pkg/pg"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
)

func TestValidUsername(t *testing.T) {
	good := []string{"abc", "Muskie", "fish_99", "a-b-c", "user123"}
	bad := []string{"", "ab", "_lead", "-lead", "has space", "emoji🐟", "way_too_long_username_exceeding_the_limit_of_chars"}
	for _, u := range good {
		if !ValidUsername(u) {
			t.Errorf("ValidUsername(%q) = false, want true", u)
		}
	}
	for _, u := range bad {
		if ValidUsername(u) {
			t.Errorf("ValidUsername(%q) = true, want false", u)
		}
	}
}

// TestRegisterUser exercises the full DB path; it is skipped unless
// TEST_DATABASE_URL points at a throwaway Postgres.
func TestRegisterUser(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the RegisterUser integration test")
	}
	ctx := context.Background()
	pool, err := pg.Pool(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	if err := pg.Migrate(ctx, pool, session.Migrations); err != nil {
		t.Fatalf("session migrate: %v", err)
	}
	if err := pg.Migrate(ctx, pool, Migrations); err != nil {
		t.Fatalf("auth migrate: %v", err)
	}

	name := "rt_" + randName(t)
	id, err := RegisterUser(ctx, pool, name, "correct horse battery")
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	if id == 0 {
		t.Fatal("RegisterUser returned id 0")
	}
	if _, err := RegisterUser(ctx, pool, name, "another password"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate register err = %v, want ErrUsernameTaken", err)
	}
	if _, err := RegisterUser(ctx, pool, "x", "short username"); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("short-username err = %v, want ErrInvalidUsername", err)
	}
}

func randName(t *testing.T) string {
	t.Helper()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}
