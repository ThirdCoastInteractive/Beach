package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestParseSQLDir_Fixture(t *testing.T) {
	queries, err := parseSQLDir("testdata/queries")
	if err != nil {
		t.Fatalf("parseSQLDir: %v", err)
	}

	byName := map[string]Query{}
	for _, q := range queries {
		byName[q.Name] = q
	}

	// All four queries parse, including the plain-sqlc one.
	for _, n := range []string{"GetItem", "CreateItem", "ListItems", "DeleteItem"} {
		if _, ok := byName[n]; !ok {
			t.Errorf("missing query %q", n)
		}
	}

	// ListItems has no @api annotation.
	if byName["ListItems"].Ann.HasAPI() {
		t.Error("ListItems should have no @api")
	}

	// GetItem: scalar id param inferred as int64.
	gi := byName["GetItem"]
	if !gi.ArgIsScalar || gi.ScalarArg != "id" || gi.ScalarType != "int64" {
		t.Errorf("GetItem arg = scalar:%v %q %q", gi.ArgIsScalar, gi.ScalarArg, gi.ScalarType)
	}

	// CreateItem: three params => a Params struct.
	ci := byName["CreateItem"]
	if ci.ArgType != "CreateItemParams" {
		t.Errorf("CreateItem ArgType = %q, want CreateItemParams", ci.ArgType)
	}
	if ci.Ann.Notify != "items" || ci.Ann.Requires != "pantry:write" {
		t.Errorf("CreateItem annotations = %+v", ci.Ann)
	}
}

// TestStandalone_EndToEnd drives the full standalone path: parse fixture SQL,
// generate, and confirm the emitted Go compiles (parses) and wires the routes.
func TestStandalone_EndToEnd(t *testing.T) {
	queries, err := parseSQLDir("testdata/queries")
	if err != nil {
		t.Fatalf("parseSQLDir: %v", err)
	}
	files, err := Generate(GenConfig{Package: "api", QuerierType: "*Queries"}, queries)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var gotGo string
	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
		if f.Name == "api.gen.go" {
			gotGo = string(f.Contents)
		}
	}
	if !names["api.gen.go"] || !names["notify.gen.sql"] {
		t.Fatalf("expected api.gen.go and notify.gen.sql, got %v", names)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "api.gen.go", gotGo, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v\n---\n%s", err, gotGo)
	}

	// Register wires all three @api routes (GetItem, CreateItem, DeleteItem) but
	// not ListItems.
	for _, w := range []string{
		`app.Page("/items/{id}", handleGetItem(q))`,
		`app.Action("/items", handleCreateItem(q, h), app.Can("pantry:write"))`,
		`app.Action("/items/{id}", handleDeleteItem(q))`,
	} {
		if !strings.Contains(gotGo, w) {
			t.Errorf("missing route registration %q\n---\n%s", w, gotGo)
		}
	}
	if strings.Contains(gotGo, "handleListItems") {
		t.Error("ListItems has no @api and must not be wired")
	}
}
