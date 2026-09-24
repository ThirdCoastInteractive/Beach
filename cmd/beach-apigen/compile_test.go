package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCompile is the e2e guarantee that generated handlers compile against the
// REAL beach module API — not just that they parse. It generates api.gen.go into
// the committed stub package testdata/compile (Queries + page.* stubs) and runs
// `go build` on it. A signature drift in beach (View, Patches, Bind, Ctx, the
// hub) breaks this test, which is the point.
//
// It is skipped in -short and when the Go toolchain is unavailable.
func TestCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go-build compile check in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not found on PATH")
	}

	const compileDir = "testdata/compile"
	const pkgPath = "github.com/ThirdCoastInteractive/Beach/cmd/beach-apigen/testdata/compile/page"

	queries, err := parseSQLDir("testdata/queries")
	if err != nil {
		t.Fatalf("parseSQLDir: %v", err)
	}
	files, err := Generate(GenConfig{
		Package:          "apicompile",
		QuerierType:      "*Queries",
		ComponentImports: map[string]string{"page": pkgPath},
	}, queries)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Write generated files into the stub package, cleaning them up afterward so
	// the tree stays pristine (only the stubs are committed).
	for _, f := range files {
		dst := filepath.Join(compileDir, f.Name)
		if err := os.WriteFile(dst, f.Contents, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
		t.Cleanup(func() { _ = os.Remove(dst) })
	}

	// Build by full import path so the package resolves regardless of cmd.Dir.
	const buildTarget = "github.com/ThirdCoastInteractive/Beach/cmd/beach-apigen/testdata/compile/..."
	cmd := exec.Command(goBin, "build", buildTarget)
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated code failed to compile: %v\n%s", err, out)
	}
}

// moduleRoot walks up from the test's working directory to the dir holding
// go.mod, so `go build` runs in module context regardless of where the test is
// invoked.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}
