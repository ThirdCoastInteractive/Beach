package main

import "testing"

func TestParseAnnotations_Full(t *testing.T) {
	comment := `-- name: CreateItem :one
-- @api POST /items
-- @requires pantry:write
-- @notify items
-- @fragment page.ItemCard`

	a, err := parseAnnotations(comment)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a.Method != "POST" || a.Path != "/items" {
		t.Errorf("api = %q %q, want POST /items", a.Method, a.Path)
	}
	if a.Requires != "pantry:write" {
		t.Errorf("requires = %q", a.Requires)
	}
	if a.Notify != "items" {
		t.Errorf("notify = %q", a.Notify)
	}
	if a.Fragment != "page.ItemCard" {
		t.Errorf("fragment = %q", a.Fragment)
	}
	if !a.HasAPI() || !a.IsMutation() {
		t.Errorf("HasAPI=%v IsMutation=%v", a.HasAPI(), a.IsMutation())
	}
}

func TestParseAnnotations_Scoped(t *testing.T) {
	a, err := parseAnnotations("-- @api GET /orders\n-- @page page.OrderList\n-- @scoped customer_id")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a.Scoped != "customer_id" {
		t.Errorf("scoped = %q, want customer_id", a.Scoped)
	}
}

func TestParseAnnotations_GETPage(t *testing.T) {
	a, err := parseAnnotations("-- @api GET /items/{id}\n-- @page page.ItemDetail\n-- @fragment page.ItemCard")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a.Method != "GET" || a.Path != "/items/{id}" {
		t.Errorf("api = %q %q", a.Method, a.Path)
	}
	if a.Page != "page.ItemDetail" || a.Fragment != "page.ItemCard" {
		t.Errorf("page=%q fragment=%q", a.Page, a.Fragment)
	}
	if a.IsMutation() {
		t.Error("GET must not be a mutation")
	}
}

func TestParseAnnotations_MethodLowercased(t *testing.T) {
	a, err := parseAnnotations("-- @api post /x\n-- @fragment p.C")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a.Method != "POST" {
		t.Errorf("method = %q, want POST (upper-cased)", a.Method)
	}
}

func TestParseAnnotations_HandlerSkip(t *testing.T) {
	a, err := parseAnnotations("-- @api GET /x\n-- @page p.C\n-- @handler skip")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !a.Skip {
		t.Error("expected Skip true")
	}
}

func TestParseAnnotations_NoLeadingDashes(t *testing.T) {
	// sqlc strips "--"; the parser must accept bare annotation lines too.
	a, err := parseAnnotations("@api GET /x\n@page p.C")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a.Method != "GET" || a.Page != "p.C" {
		t.Errorf("got %+v", a)
	}
}

func TestParseAnnotations_Errors(t *testing.T) {
	cases := map[string]string{
		"unknown directive": "-- @notifies items",
		"api missing path":  "-- @api GET",
		"api empty":         "-- @api",
		"page empty":        "-- @page",
		"requires empty":    "-- @requires",
		"handler bad arg":   "-- @handler nope",
		"notify empty":      "-- @notify",
		"scoped empty":      "-- @scoped",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseAnnotations(in); err == nil {
				t.Errorf("expected error for %q", in)
			}
		})
	}
}

func TestAnnotations_Validate(t *testing.T) {
	t.Run("GET needs render target", func(t *testing.T) {
		a := Annotations{Method: "GET", Path: "/x"}
		if err := a.validate("Q"); err == nil {
			t.Error("GET with no page/fragment should fail validate")
		}
	})
	t.Run("mutation needs fragment or notify", func(t *testing.T) {
		a := Annotations{Method: "POST", Path: "/x"}
		if err := a.validate("Q"); err == nil {
			t.Error("POST with no fragment/notify should fail validate")
		}
	})
	t.Run("valid mutation", func(t *testing.T) {
		a := Annotations{Method: "POST", Path: "/x", Fragment: "p.C"}
		if err := a.validate("Q"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("no api is fine", func(t *testing.T) {
		if err := (Annotations{}).validate("Q"); err != nil {
			t.Errorf("no-api validate should pass: %v", err)
		}
	})
	t.Run("bad method", func(t *testing.T) {
		a := Annotations{Method: "OPTIONS", Path: "/x"}
		if err := a.validate("Q"); err == nil {
			t.Error("OPTIONS should be rejected")
		}
	})
}
