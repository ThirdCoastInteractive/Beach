package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateItemSQLCarriesApigenAnnotations(t *testing.T) {
	p := filepath.Join("..", "db", "sql", "queries", "items", "items.sql")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{
		"-- name: CreateItem :one",
		"-- @api POST /items",
		"-- @requires pantry:write",
		"-- @notify items",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("items.sql missing %q", want)
		}
	}
}
