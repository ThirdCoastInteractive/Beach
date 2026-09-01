package main

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// pbField encodes one length-delimited protobuf field for test fixtures.
func pbField(field int, val []byte) []byte {
	var b []byte
	b = binary.AppendUvarint(b, uint64(field)<<3|wireBytes)
	b = binary.AppendUvarint(b, uint64(len(val)))
	return append(b, val...)
}

// buildQueryMsg builds a Query{ name=2, cmd=3, comments=6... , params=5... }.
func buildQueryMsg(name, cmd string, comments []string, paramNames []string) []byte {
	var q []byte
	q = append(q, pbField(2, []byte(name))...)
	q = append(q, pbField(3, []byte(cmd))...)
	for _, c := range comments {
		q = append(q, pbField(6, []byte(c))...)
	}
	for _, p := range paramNames {
		// Parameter{ column=2 -> Column{ name=1 } }
		col := pbField(1, []byte(p))
		q = append(q, pbField(5, pbField(2, col))...)
	}
	return q
}

func TestPlugin_RoundTrip(t *testing.T) {
	// GenerateRequest{ queries=3 [ CreateItem ] } — queries is field 3 in sqlc codegen.proto
	qmsg := buildQueryMsg(
		"CreateItem", "one",
		[]string{"@api POST /items", "@notify items", "@fragment page.ItemCard", "@requires pantry:write"},
		[]string{"name", "quantity"},
	)
	req := pbField(3, qmsg)

	var out bytes.Buffer
	if err := runPlugin(bytes.NewReader(req), &out); err != nil {
		t.Fatalf("runPlugin: %v", err)
	}

	files := decodeResponseForTest(t, out.Bytes())
	if len(files) == 0 {
		t.Fatal("no files in response")
	}
	var gotGo, gotSQL string
	for name, contents := range files {
		switch name {
		case "api.gen.go":
			gotGo = contents
		case "notify.gen.sql":
			gotSQL = contents
		}
	}
	if gotGo == "" {
		t.Fatal("response missing api.gen.go")
	}
	if !strings.Contains(gotGo, "func handleCreateItem(q *Queries, h *hub.Hub) beach.ActionFunc") {
		t.Errorf("plugin output missing action factory\n---\n%s", gotGo)
	}
	if !strings.Contains(gotGo, "beach.Bind[CreateItemParams](c)") {
		t.Errorf("two params should bind a Params struct\n---\n%s", gotGo)
	}
	if !strings.Contains(gotGo, `c.MustPrincipal().Can("pantry:write")`) {
		t.Errorf("@requires not wired through plugin path\n---\n%s", gotGo)
	}
	if gotSQL == "" || !strings.Contains(gotSQL, "pg_notify('items'") {
		t.Errorf("plugin path should emit notify migration, got %q", gotSQL)
	}
}

func TestPlugin_ScalarParam(t *testing.T) {
	qmsg := buildQueryMsg(
		"GetItem", "one",
		[]string{"@api GET /items/{id}", "@page page.ItemDetail"},
		[]string{"id"},
	)
	req := pbField(3, qmsg)

	var out bytes.Buffer
	if err := runPlugin(bytes.NewReader(req), &out); err != nil {
		t.Fatalf("runPlugin: %v", err)
	}
	files := decodeResponseForTest(t, out.Bytes())
	got := files["api.gen.go"]
	if !strings.Contains(got, "q.GetItem(c.Context(), in.ID)") {
		t.Errorf("single param should be scalar arg\n---\n%s", got)
	}
	if !strings.Contains(got, "type GetItemRequest struct") {
		t.Errorf("scalar request struct missing\n---\n%s", got)
	}
}

func TestPlugin_ScopedOK(t *testing.T) {
	// A @scoped query whose scope parameter is among its params generates through
	// the plugin path with no error.
	qmsg := buildQueryMsg(
		"ListCustomerOrders", "many",
		[]string{"@api GET /orders", "@page page.OrderList", "@scoped customer_id"},
		[]string{"customer_id", "status"},
	)
	req := pbField(3, qmsg)

	var out bytes.Buffer
	if err := runPlugin(bytes.NewReader(req), &out); err != nil {
		t.Fatalf("runPlugin on correctly scoped query: %v", err)
	}
	files := decodeResponseForTest(t, out.Bytes())
	if files["api.gen.go"] == "" {
		t.Fatal("plugin path produced no api.gen.go for scoped-ok query")
	}
}

func TestPlugin_ScopedMissingFails(t *testing.T) {
	// A @scoped query whose scope parameter is absent must fail the plugin run,
	// naming the offending query and the missing parameter.
	qmsg := buildQueryMsg(
		"ListAllOrders", "many",
		[]string{"@api GET /orders", "@page page.OrderList", "@scoped customer_id"},
		[]string{"status"},
	)
	req := pbField(3, qmsg)

	var out bytes.Buffer
	err := runPlugin(bytes.NewReader(req), &out)
	if err == nil {
		t.Fatal("expected the plugin run to fail on a query missing its scope parameter")
	}
	for _, want := range []string{"ListAllOrders", "customer_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must name %q", err, want)
		}
	}
}

// decodeResponseForTest parses a GenerateResponse{ files=1 [ File{name=1, contents=2} ] }.
func decodeResponseForTest(t *testing.T, b []byte) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := walkFields(b, func(field, wt int, val []byte) error {
		if field != 1 {
			return nil
		}
		var name, contents string
		_ = walkFields(val, func(f, w int, v []byte) error {
			switch f {
			case 1:
				name = string(v)
			case 2:
				contents = string(v)
			}
			return nil
		})
		files[name] = contents
		return nil
	})
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return files
}
