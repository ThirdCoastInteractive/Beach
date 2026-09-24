package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// genOne generates the api.gen.go contents for a single query and fails on error.
func genOne(t *testing.T, q Query) string {
	t.Helper()
	files, err := Generate(GenConfig{Package: "api", QuerierType: "*Queries"}, []Query{q})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, f := range files {
		if f.Name == "api.gen.go" {
			return string(f.Contents)
		}
	}
	t.Fatal("no api.gen.go produced")
	return ""
}

// assertParses confirms the generated Go is syntactically valid (gofmt already
// ran inside Generate, but parse again so a broken template fails the test).
func assertParses(t *testing.T, src string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "api.gen.go", src, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v\n---\n%s", err, src)
	}
}

func TestGenerate_PageHandler(t *testing.T) {
	q := Query{
		Name: "GetItem", Cmd: ":one",
		Ann:         Annotations{Method: "GET", Path: "/items/{id}", Page: "page.ItemDetail", Fragment: "page.ItemCard"},
		ArgIsScalar: true, ScalarArg: "id", ScalarType: "int64",
	}
	src := genOne(t, q)
	assertParses(t, src)

	wants := []string{
		"func handleGetItem(q *Queries) beach.PageFunc",
		"func(c *beach.Ctx) (beach.View, error)",
		"beach.Bind[GetItemRequest](c)",
		"q.GetItem(c.Context(), in.ID)",
		"page.ItemDetail(row)",
		"page.ItemCard(row)",
		`app.Page("/items/{id}", handleGetItem(q))`,
		"type GetItemRequest struct",
		`json:"id"`,
	}
	for _, w := range wants {
		if !strings.Contains(src, w) {
			t.Errorf("page handler missing %q\n---\n%s", w, src)
		}
	}
}

func TestGenerate_ActionHandler_FullStack(t *testing.T) {
	q := Query{
		Name: "CreateItem", Cmd: ":one",
		Ann: Annotations{
			Method: "POST", Path: "/items",
			Requires: "pantry:write", Notify: "items", Fragment: "page.ItemCard",
		},
		ArgType: "CreateItemParams",
		Params:  []Param{{Field: "Name", JSONName: "name", Type: "string"}},
	}
	src := genOne(t, q)
	assertParses(t, src)

	wants := []string{
		"func handleCreateItem(q *Queries, h *hub.Hub) beach.ActionFunc",
		"func(c *beach.Ctx) (beach.Patches, error)",
		`c.MustPrincipal().Can("pantry:write")`,
		"return nil, beach.ErrForbidden",
		"beach.Bind[CreateItemParams](c)",
		"q.CreateItem(c.Context(), in)",
		`notifyPublish(h, "items", page.ItemCard(row))`,
		"return beach.Patches{",
		"{Fragment: page.ItemCard(row)}",
		`app.Action("/items", handleCreateItem(q, h), app.Can("pantry:write"))`,
		"func notifyPublish(h *hub.Hub, topic string, frag templ.Component)",
		"hub.Event{Bytes: buf.Bytes()}",
	}
	for _, w := range wants {
		if !strings.Contains(src, w) {
			t.Errorf("action handler missing %q\n---\n%s", w, src)
		}
	}
}

func TestGenerate_NoNotify_NoHubImport(t *testing.T) {
	q := Query{
		Name: "DeleteItem", Cmd: ":exec",
		Ann:         Annotations{Method: "DELETE", Path: "/items/{id}", Fragment: "page.ItemList"},
		ArgIsScalar: true, ScalarArg: "id", ScalarType: "int64",
	}
	src := genOne(t, q)
	assertParses(t, src)

	if strings.Contains(src, "hub.Hub") {
		t.Errorf("query without @notify must not thread the hub\n---\n%s", src)
	}
	if strings.Contains(src, `"github.com/ThirdCoastInteractive/Beach/pkg/hub"`) {
		t.Errorf("query without @notify must not import hub\n---\n%s", src)
	}
	if !strings.Contains(src, "func handleDeleteItem(q *Queries) beach.ActionFunc") {
		t.Errorf("factory should take only q\n---\n%s", src)
	}
}

func TestGenerate_HandlerSkip(t *testing.T) {
	q := Query{
		Name: "WeirdQuery", Cmd: ":one",
		Ann: Annotations{Method: "GET", Path: "/weird", Page: "page.Weird", Skip: true},
	}
	src := genOne(t, q)
	assertParses(t, src)

	// Route is registered referencing the hand-written handler, but no body is
	// generated.
	if !strings.Contains(src, `app.Page("/weird", handleWeirdQuery(q))`) {
		t.Errorf("skip should still register the route\n---\n%s", src)
	}
	if strings.Contains(src, "func handleWeirdQuery(q *Queries) beach.PageFunc {") {
		t.Errorf("skip must NOT generate a handler body\n---\n%s", src)
	}
}

func TestGenerate_NotifyMigration(t *testing.T) {
	q := Query{
		Name: "CreateItem", Cmd: ":one",
		Ann:     Annotations{Method: "POST", Path: "/items", Notify: "items", Fragment: "page.ItemCard"},
		ArgType: "CreateItemParams",
	}
	files, err := Generate(GenConfig{Package: "api", QuerierType: "*Queries"}, []Query{q})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var mig string
	for _, f := range files {
		if f.Name == "notify.gen.sql" {
			mig = string(f.Contents)
		}
	}
	if mig == "" {
		t.Fatal("expected notify.gen.sql")
	}
	for _, w := range []string{
		"+goose Up", "+goose Down",
		"CREATE OR REPLACE FUNCTION items_notify()",
		"pg_notify('items'",
		"json_build_object('table', TG_TABLE_NAME, 'id', rec.id, 'op'",
		"CREATE TRIGGER items_notify",
		"DROP TRIGGER IF EXISTS items_notify ON items",
	} {
		if !strings.Contains(mig, w) {
			t.Errorf("migration missing %q\n---\n%s", w, mig)
		}
	}
}

func TestGenerate_ScopedPresent(t *testing.T) {
	// A @scoped query whose scope parameter IS present generates as any other
	// query — the rule is invisible on the happy path.
	q := Query{
		Name: "ListCustomerOrders", Cmd: ":many",
		Ann:     Annotations{Method: "GET", Path: "/orders", Page: "page.OrderList", Scoped: "customer_id"},
		ArgType: "ListCustomerOrdersParams",
		Params: []Param{
			{Field: "CustomerID", JSONName: "customer_id", Type: "int64"},
			{Field: "Status", JSONName: "status", Type: "string"},
		},
	}
	src := genOne(t, q)
	assertParses(t, src)
	if !strings.Contains(src, `app.Page("/orders", handleListCustomerOrders(q))`) {
		t.Errorf("scoped-ok query should generate normally\n---\n%s", src)
	}
}

func TestGenerate_ScopedMissingFails(t *testing.T) {
	// A @scoped query whose scope parameter is absent must fail generation with a
	// query-named error that names the missing parameter.
	q := Query{
		Name: "ListAllOrders", Cmd: ":many",
		Ann:     Annotations{Method: "GET", Path: "/orders", Page: "page.OrderList", Scoped: "customer_id"},
		ArgType: "ListAllOrdersParams",
		Params:  []Param{{Field: "Status", JSONName: "status", Type: "string"}},
	}
	_, err := Generate(GenConfig{Package: "api", QuerierType: "*Queries"}, []Query{q})
	if err == nil {
		t.Fatal("expected @scoped validation to fail when the scope parameter is missing")
	}
	for _, want := range []string{"ListAllOrders", "customer_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must name %q", err, want)
		}
	}
}

func TestGenerate_SkipsNonAPIQueries(t *testing.T) {
	q := Query{Name: "InternalOnly", Cmd: ":one"} // no @api
	files, err := Generate(GenConfig{Package: "api", QuerierType: "*Queries"}, []Query{q})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("non-api query should produce no files, got %d", len(files))
	}
}
