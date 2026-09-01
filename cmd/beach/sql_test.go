package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestSlugMigrationName(t *testing.T) {
	got, err := slugMigrationName("Add Users Table")
	if err != nil {
		t.Fatal(err)
	}
	if got != "add_users_table" {
		t.Fatalf("slug = %q, want add_users_table", got)
	}
	if _, err := slugMigrationName("00001"); err == nil {
		t.Fatal("expected error for digit-only name")
	}
	if _, err := slugMigrationName("2fa_tokens"); err == nil {
		t.Fatal("expected error for name starting with a digit")
	}
	if _, err := slugMigrationName("???"); err == nil {
		t.Fatal("expected error for empty slug")
	}
}

func TestSQLNewEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := cmdSQLNew([]string{"init", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "00001_init.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, marker := range []string{"-- +goose Up", "-- +goose Down", "-- +goose StatementBegin", "-- +goose StatementEnd"} {
		if !strings.Contains(s, marker) {
			t.Errorf("missing %s in:\n%s", marker, s)
		}
	}
}

func TestSQLNewSkipsGapsAndNonMigrations(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "00001_init.sql"), "-- +goose Up\n")
	mustWrite(t, filepath.Join(dir, "00003_later.sql"), "-- +goose Up\n")
	mustWrite(t, filepath.Join(dir, "README.md"), "notes\n")
	if err := cmdSQLNew([]string{"indexes", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "00004_indexes.sql")); err != nil {
		t.Fatalf("expected 00004_indexes.sql: %v", err)
	}
}

func TestSQLNewParallelAgentsGetDistinctVersions(t *testing.T) {
	dir := t.TempDir()
	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			errs[i] = cmdSQLNew([]string{"m" + strconv.Itoa(i), "--dir", dir})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("agent %d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]string{}
	for _, e := range entries {
		v, ok := fileVersion(e.Name())
		if !ok || !strings.Contains(e.Name(), "_") {
			t.Errorf("leftover claim or junk: %s", e.Name())
			continue
		}
		if prev, dup := seen[v]; dup {
			t.Errorf("version %d claimed by %s and %s", v, prev, e.Name())
		}
		seen[v] = e.Name()
	}
	if len(seen) != n {
		t.Fatalf("got %d migrations, want %d (%v)", len(seen), n, seen)
	}
}

func TestResolveMigrationsDirUnique(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveMigrationsDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "migrations" {
		t.Fatalf("dir = %q, want migrations", got)
	}
}

func TestResolveMigrationsDirMultiple(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Join("internal", "analytics", "chmigrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := resolveMigrationsDir()
	if err == nil {
		t.Fatal("expected error listing both dirs")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--dir") || !strings.Contains(msg, "migrations") || !strings.Contains(msg, "chmigrations") {
		t.Fatalf("error should list both dirs: %v", err)
	}
}

func TestFileVersion(t *testing.T) {
	cases := map[string]int64{
		"00001_init.sql": 1,
		"00010_idx.sql":  10,
		"00002.sql":      2,
	}
	for name, want := range cases {
		got, ok := fileVersion(name)
		if !ok || got != want {
			t.Errorf("fileVersion(%q) = %d,%v; want %d,true", name, got, ok, want)
		}
	}
	for _, name := range []string{"README.md", "foo.sql", "00000_zero.sql"} {
		if _, ok := fileVersion(name); ok {
			t.Errorf("fileVersion(%q) should reject", name)
		}
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
