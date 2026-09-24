package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// sqlNewTemplate is the goose SQL skeleton stamped by `beach sql new`.
// StatementBegin/End is the house style (plpgsql-safe; still fine for one-shot
// DDL). Agents fill in the bodies; they do not invent a second marker dialect.
const sqlNewTemplate = `-- +goose Up
-- +goose StatementBegin

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- +goose StatementEnd
`

const (
	sqlVersionWidth = 5
	sqlMaxAttempts  = 1024
)

// skipSQLWalk is directory names `findMigrationsDirs` does not descend into.
var skipSQLWalk = map[string]bool{
	".git":         true,
	"vendor":       true,
	"node_modules": true,
	"testdata":     true,
	"bin":          true,
	"dist":         true,
}

func cmdSQL(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("sql: expected a subcommand (new)")
	}
	switch args[0] {
	case "new":
		return cmdSQLNew(args[1:])
	default:
		return fmt.Errorf("sql: unknown subcommand %q (want: new)", args[0])
	}
}

// cmdSQLNew implements `beach sql new <name> [--dir DIR]`.
//
// It looks up a goose migrations directory (or uses --dir), finds the highest
// numeric version, and creates the next NNNNN_name.sql with goose Up/Down
// markers. Version numbers are claimed with O_EXCL so parallel agents cannot
// both land 00002_*.sql under different names.
func cmdSQLNew(args []string) error {
	fs := flag.NewFlagSet("sql new", flag.ContinueOnError)
	dirFlag := fs.String("dir", "", "migrations directory (default: unique migrations/ or chmigrations/ under cwd)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("sql new: expected exactly one name (got %d)", fs.NArg())
	}
	slug, err := slugMigrationName(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("sql new: %w", err)
	}

	dir := *dirFlag
	if dir == "" {
		dir, err = resolveMigrationsDir()
		if err != nil {
			return fmt.Errorf("sql new: %w", err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sql new: mkdir %s: %w", dir, err)
	}

	path, err := createNextMigration(dir, slug)
	if err != nil {
		return fmt.Errorf("sql new: %w", err)
	}
	fmt.Println(path)
	return nil
}

func slugMigrationName(name string) (string, error) {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r == ' ' || r == '-' || r == '.':
			if b.Len() > 0 && !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_':
			b.WriteRune(r)
			lastUnderscore = r == '_'
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return "", fmt.Errorf("name %q slugs to empty; use letters/digits", name)
	}
	hasLetter := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return "", fmt.Errorf("name %q must contain a letter", name)
	}
	if unicode.IsDigit(rune(s[0])) {
		return "", fmt.Errorf("name %q must not start with a digit (goose would misread the version)", s)
	}
	return s, nil
}

func resolveMigrationsDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	found, err := findMigrationsDirs(cwd)
	if err != nil {
		return "", err
	}
	switch len(found) {
	case 0:
		return "migrations", nil
	case 1:
		rel, err := filepath.Rel(cwd, found[0])
		if err != nil {
			return found[0], nil
		}
		return rel, nil
	default:
		rels := make([]string, len(found))
		for i, p := range found {
			rel, err := filepath.Rel(cwd, p)
			if err != nil {
				rel = p
			}
			rels[i] = rel
		}
		return "", fmt.Errorf("multiple migration directories; pass --dir\n  %s", strings.Join(rels, "\n  "))
	}
}

func findMigrationsDirs(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && skipSQLWalk[name] {
			return filepath.SkipDir
		}
		if name == "migrations" || name == "chmigrations" {
			found = append(found, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// createNextMigration writes the next goose file under dir and returns its path.
// A sibling NNNNN.sql claim file is created with O_EXCL so two processes cannot
// pick the same version even when their slug names differ.
func createNextMigration(dir, slug string) (string, error) {
	occupied, err := occupiedVersions(dir)
	if err != nil {
		return "", err
	}
	ver := nextVersion(occupied)
	for range sqlMaxAttempts {
		if occupied[ver] {
			ver++
			continue
		}
		path, err := claimMigration(dir, ver, slug)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		occupied[ver] = true
		ver++
	}
	return "", fmt.Errorf("could not claim a version in %s after %d attempts", dir, sqlMaxAttempts)
}

func nextVersion(occupied map[int64]bool) int64 {
	var max int64
	for v := range occupied {
		if v > max {
			max = v
		}
	}
	return max + 1
}

func occupiedVersions(dir string) (map[int64]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if v, ok := fileVersion(e.Name()); ok {
			out[v] = true
		}
	}
	return out, nil
}

// fileVersion reports the goose version encoded in name. NNNNN_foo.sql is a
// real migration; NNNNN.sql is an in-flight exclusive claim.
func fileVersion(name string) (int64, bool) {
	if !strings.HasSuffix(name, ".sql") {
		return 0, false
	}
	stem := strings.TrimSuffix(name, ".sql")
	before, _, hadSep := strings.Cut(stem, "_")
	if !hadSep {
		before = stem
	}
	if before == "" {
		return 0, false
	}
	for _, r := range before {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(before, 10, 64)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func claimMigration(dir string, ver int64, slug string) (string, error) {
	claim := filepath.Join(dir, fmt.Sprintf("%0*d.sql", sqlVersionWidth, ver))
	final := filepath.Join(dir, fmt.Sprintf("%0*d_%s.sql", sqlVersionWidth, ver, slug))
	f, err := os.OpenFile(claim, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, werr := f.WriteString(sqlNewTemplate)
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(claim)
		return "", werr
	}
	if cerr != nil {
		_ = os.Remove(claim)
		return "", cerr
	}
	if err := os.Rename(claim, final); err != nil {
		_ = os.Remove(claim)
		return "", err
	}
	return final, nil
}
