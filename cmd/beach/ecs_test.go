package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSchemaFlowFields(t *testing.T) {
	src := `package: components
components:
  - name: Position
    doc: A board position.
    fields:
      - { name: X, type: int }
      - { name: Y, type: int }
  - name: Label
    fields:
      - { name: Text, type: string }
`
	sc, err := parseSchema([]byte(src))
	if err != nil {
		t.Fatalf("parseSchema: %v", err)
	}
	if sc.Package != "components" {
		t.Errorf("package = %q, want components", sc.Package)
	}
	if len(sc.Components) != 2 {
		t.Fatalf("got %d components, want 2", len(sc.Components))
	}
	p := sc.Components[0]
	if p.Name != "Position" || p.Doc != "A board position." || len(p.Fields) != 2 {
		t.Errorf("Position parsed wrong: %+v", p)
	}
	if p.Fields[0].Name != "X" || p.Fields[0].Type != "int" {
		t.Errorf("Position field 0 = %+v", p.Fields[0])
	}
	if sc.Components[1].Fields[0].Type != "string" {
		t.Errorf("Label field type = %q", sc.Components[1].Fields[0].Type)
	}
}

func TestParseSchemaBlockFields(t *testing.T) {
	src := `package: world
components:
  - name: Health
    fields:
      - name: Current
        type: int
      - name: Max
        type: int
`
	sc, err := parseSchema([]byte(src))
	if err != nil {
		t.Fatalf("parseSchema: %v", err)
	}
	if len(sc.Components) != 1 || len(sc.Components[0].Fields) != 2 {
		t.Fatalf("unexpected parse: %+v", sc)
	}
	if sc.Components[0].Fields[1].Name != "Max" || sc.Components[0].Fields[1].Type != "int" {
		t.Errorf("field = %+v", sc.Components[0].Fields[1])
	}
}

func TestValidateSchemaRejectsBadType(t *testing.T) {
	src := `package: p
components:
  - name: Bad
    fields:
      - { name: Where, type: SomeStruct }
`
	if _, err := parseSchema([]byte(src)); err == nil {
		t.Fatalf("expected error for unsupported type")
	}
}

func TestRenderComponentsCompilesShape(t *testing.T) {
	sc := schema{
		Package: "components",
		Components: []component{
			{Name: "Position", Doc: "A spot.", Fields: []field{{"X", "int"}, {"Y", "int"}}},
		},
	}
	code, err := renderComponents(sc)
	if err != nil {
		t.Fatalf("renderComponents: %v", err)
	}
	s := string(code)
	for _, want := range []string{
		"package components",
		"type Position struct",
		`ecs.Register[Position]("components.Position")`,
		"DO NOT EDIT",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("generated code missing %q\n%s", want, s)
		}
	}
}

func TestECSGenWritesFile(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "components.beach.yaml")
	os.WriteFile(schemaPath, []byte(`package: components
components:
  - name: Tag
    fields:
      - { name: Name, type: string }
`), 0o644)

	if err := cmdECSGen([]string{"--file", schemaPath}); err != nil {
		t.Fatalf("cmdECSGen: %v", err)
	}
	out := filepath.Join(dir, "components_gen.go")
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated: %v", err)
	}
	if !strings.Contains(string(b), "type Tag struct") {
		t.Errorf("generated file missing struct:\n%s", b)
	}
}
