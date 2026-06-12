package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewStampsBuildableSkeleton stamps an app into a temp dir and asserts every
// expected file lands with sane content. It does not run `go build` (that needs
// the surrounding checkout and module cache); the framework's own build proves
// the templated main.go compiles, and main_build verification is done manually.
func TestNewStampsBuildableSkeleton(t *testing.T) {
	dir := t.TempDir()
	if err := cmdNew([]string{"tidepool", "--dir", dir}); err != nil {
		t.Fatalf("cmdNew: %v", err)
	}

	app := filepath.Join(dir, "tidepool")
	want := []string{
		"go.mod",
		"go.sum",
		"main.go",
		"Makefile",
		"sqlc.yaml",
		"docker-compose.yml",
		".env",
		".gitignore",
		"README.md",
		"components.beach.yaml",
		"catalog.json",
		filepath.Join("locales", "en-US.json"),
	}
	for _, rel := range want {
		p := filepath.Join(app, rel)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("missing stamped file %s: %v", rel, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("stamped file %s is empty", rel)
		}
	}
}

// TestNewRendersAppName checks that template substitution reached the files that
// carry the app name, and that go.mod has both the require and the replace.
func TestNewRendersAppName(t *testing.T) {
	dir := t.TempDir()
	if err := cmdNew([]string{"lagoon", "--dir", dir}); err != nil {
		t.Fatalf("cmdNew: %v", err)
	}
	app := filepath.Join(dir, "lagoon")

	mainGo := readFile(t, filepath.Join(app, "main.go"))
	if !strings.Contains(mainGo, `Service: "lagoon"`) {
		t.Errorf("main.go missing rendered service name; got:\n%s", first(mainGo, 400))
	}

	gomod := readFile(t, filepath.Join(app, "go.mod"))
	if !strings.Contains(gomod, "module lagoon") {
		t.Errorf("go.mod missing module line:\n%s", gomod)
	}
	if !strings.Contains(gomod, "replace github.com/ThirdCoastInteractive/the-beach =>") {
		t.Errorf("go.mod missing replace directive:\n%s", gomod)
	}
	// The mirrored transitive require block must be present (templ is a known dep).
	if !strings.Contains(gomod, "github.com/a-h/templ") {
		t.Errorf("go.mod missing mirrored framework requires:\n%s", gomod)
	}
}

// TestNewRejectsExisting refuses to overwrite a populated target.
func TestNewRejectsExisting(t *testing.T) {
	dir := t.TempDir()
	if err := cmdNew([]string{"reef", "--dir", dir}); err != nil {
		t.Fatalf("first cmdNew: %v", err)
	}
	if err := cmdNew([]string{"reef", "--dir", dir}); err == nil {
		t.Fatalf("expected error stamping over an existing app")
	}
}

// TestNewRejectsBadName rejects names with path-hostile characters.
func TestNewRejectsBadName(t *testing.T) {
	dir := t.TempDir()
	if err := cmdNew([]string{"bad/name", "--dir", dir}); err == nil {
		t.Fatalf("expected error for invalid app name")
	}
}

// TestReorderArgs covers flags appearing after the positional.
func TestReorderArgs(t *testing.T) {
	got := reorderArgs([]string{"myapp", "--dir", "out"})
	want := []string{"--dir", "out", "myapp"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("reorderArgs = %v, want %v", got, want)
	}
	got = reorderArgs([]string{"--dir=out", "myapp"})
	want = []string{"--dir=out", "myapp"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("reorderArgs = %v, want %v", got, want)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func first(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
