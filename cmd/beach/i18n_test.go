package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanKeysFindsLiteralCalls(t *testing.T) {
	dir := t.TempDir()
	good := `package x

import "context"

var i18n = struct{ T func(context.Context, string, ...any) string }{}

func use(ctx context.Context) {
	_ = i18n.T(ctx, "page.title")
	_ = i18n.T(ctx, "cart.count", 3)
	dynamic := "x"
	_ = i18n.T(ctx, dynamic) // non-literal: ignored
}
`
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(good), 0o644)

	keys, err := scanKeys(dir)
	if err != nil {
		t.Fatalf("scanKeys: %v", err)
	}
	got := strings.Join(keys, ",")
	if got != "cart.count,page.title" {
		t.Errorf("keys = %q, want cart.count,page.title", got)
	}
}

func TestI18nWriteAndVerify(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(`package x

import "context"

var i18n = struct{ T func(context.Context, string, ...any) string }{}

func use(ctx context.Context) { _ = i18n.T(ctx, "hello.world") }
`), 0o644)

	// Seed a catalog with one stale key and a custom label for the live key.
	cat := filepath.Join(dir, "catalog.json")
	os.WriteFile(cat, []byte(`{
  "hello.world": {"label": "Hi", "comment": "greeting"},
  "old.key": {"label": "gone", "comment": ""}
}`), 0o644)

	// Verify must fail because old.key is stale.
	if err := cmdI18n([]string{"--dir", dir}); err == nil {
		t.Fatalf("expected verify to fail on stale key")
	}

	// Write reconciles.
	if err := cmdI18n([]string{"--dir", dir, "--write"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := readFile(t, cat)
	if !strings.Contains(out, `"hello.world"`) {
		t.Errorf("catalog dropped live key:\n%s", out)
	}
	if strings.Contains(out, "old.key") {
		t.Errorf("catalog kept stale key:\n%s", out)
	}
	if !strings.Contains(out, `"Hi"`) {
		t.Errorf("catalog lost existing label:\n%s", out)
	}

	// Verify now passes.
	if err := cmdI18n([]string{"--dir", dir}); err != nil {
		t.Fatalf("verify after write: %v", err)
	}
}
