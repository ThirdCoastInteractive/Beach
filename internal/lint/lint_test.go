package lint

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// TestBadSnippet asserts every rule fires exactly where bad.go plants a
// violation. We collect the set of rules reported and compare against the
// expected set; counts are asserted per rule where bad.go plants more than one.
func TestBadSnippet(t *testing.T) {
	findings, err := Check(filepath.Join("testdata", "bad"))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings in testdata/bad, got none")
	}

	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Rule]++
		if f.Line == 0 {
			t.Errorf("finding has zero line: %+v", f)
		}
		if f.File == "" {
			t.Errorf("finding has empty file: %+v", f)
		}
	}

	want := map[string]int{
		"raw-datastar":      2, // colon + dash spellings
		"pgtype-import":     1,
		"uuid-import":       1,
		"hardcoded-color":   2, // hex + oklch
		"naked-handlerfunc": 1,
		"custom-script":     1,
		// Both spellings: a Tailwind utility in a class, and a padding
		// declaration in a style attribute. The two non-findings beside them in
		// testdata are the point — a dw-gap-lg and a reserved height must not
		// fire, or the rule would make the sanctioned path unusable.
		"raw-spacing": 2,

		// The a11y rules. a11y-literal-name is not exercised here: it only fires
		// inside the kit (pkg/ui, pkg/chart), and testdata/bad is app-shaped
		// source. TestLiteralNameIsKitOnly covers it directly instead.
		"a11y-img-alt":          1,
		"a11y-unnamed-role-img": 1,
	}
	for rule, n := range want {
		if counts[rule] != n {
			t.Errorf("rule %q: got %d findings, want %d (all: %v)", rule, counts[rule], n, counts)
		}
	}
	for rule := range counts {
		if rule == "parse" {
			t.Errorf("unexpected parse error: %v", findings)
			continue
		}
		if _, ok := want[rule]; !ok {
			t.Errorf("unexpected rule fired: %q (%v)", rule, counts)
		}
	}
}

// TestGoodSnippet asserts the sanctioned spellings produce zero findings.
func TestGoodSnippet(t *testing.T) {
	findings, err := Check(filepath.Join("testdata", "good"))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings in testdata/good, got %d: %+v", len(findings), findings)
	}
}

// TestFindingsSorted asserts Check returns findings sorted by file then line.
func TestFindingsSorted(t *testing.T) {
	findings, err := Check(filepath.Join("testdata", "bad"))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	sorted := sort.SliceIsSorted(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	if !sorted {
		t.Errorf("findings not sorted: %+v", findings)
	}
}

// snippet wraps markup in the smallest parseable Go file that carries it as a
// string literal — the shape templ's generated output has, and the only thing
// the markup rules ever see.
func snippet(markup string) string {
	return "package p\n\nconst m = " + strconv.Quote(markup) + "\n"
}

// snippetFindings parses src as a Go file living in package directory pkg and
// runs one rule over it. It is the smallest harness that still exercises a rule
// the way Check does — through a real AST — so a rule's path sanctioning and its
// literal scanning are both covered.
func snippetFindings(t *testing.T, pkg, src string, r rule) []Finding {
	t.Helper()
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out []Finding
	r(&fileCtx{path: pkg + "/x.go", pkg: pkg, fset: fset, file: af}, &out)
	return out
}

// TestLiteralNameIsKitOnly pins the one rule whose sanctioning runs the other
// way round. An accessible name is content, so the kit's names must come from
// the catalog rather than from its source — but an app's own names are in the
// app's own language already, and flagging them would be noise.
func TestLiteralNameIsKitOnly(t *testing.T) {
	src := snippet(`<button aria-label="Close">x</button>`)
	cases := map[string]bool{
		"pkg/ui/driftwood":    true,
		"pkg/chart":           true,
		"ui":                  true, // an app tree that vendors the kit at the root
		"internal/web":        false,
		"cmd/examples/pantry": false,
	}
	for pkg, want := range cases {
		got := len(snippetFindings(t, pkg, src, ruleLiteralAccessibleName)) > 0
		if got != want {
			t.Errorf("pkg %q: fired=%v, want %v", pkg, got, want)
		}
	}
}

// TestLiteralNameAllowsInterpolation asserts the rule tells a hardcoded name
// apart from a translated one. templ writes an interpolated attribute value
// separately, so the literal ends at `aria-label="` with nothing after it — the
// exact shape i18n.T(ctx, ...) produces, and the shape the kit now uses
// everywhere.
func TestLiteralNameAllowsInterpolation(t *testing.T) {
	interpolated := snippet("<button aria-label=\"")
	if n := len(snippetFindings(t, "pkg/ui/driftwood", interpolated, ruleLiteralAccessibleName)); n != 0 {
		t.Errorf("interpolated aria-label flagged %d times, want 0", n)
	}
}

// TestEachRuleIsolated exercises individual rules through a focused snippet so a
// regression in one rule's regex is localized.
func TestEachRuleIsolated(t *testing.T) {
	cases := []struct {
		name string
		val  string
		fire bool
		re   func(string) bool
	}{
		{"hex fires", "#ff8800", true, func(s string) bool { return hexColorRe.MatchString(s) }},
		{"hex short fires", "#fff", true, func(s string) bool { return hexColorRe.MatchString(s) }},
		{"hex in var ok at regex level", "var(--x, #fff)", true, func(s string) bool { return hexColorRe.MatchString(s) }},
		{"oklch fires", "oklch(0.7 0.1 200)", true, func(s string) bool { return oklchRe.MatchString(s) }},
		{"data-on colon", `<b data-on:click="x">`, true, func(s string) bool { return dataAttrRe.MatchString(s) }},
		{"data-bind dash", `<b data-bind-email>`, true, func(s string) bool { return dataAttrRe.MatchString(s) }},
		{"data unrelated word", "metadata-only", false, func(s string) bool { return dataAttrRe.MatchString(s) }},
		{"script open", `<script>x</script>`, true, func(s string) bool { return scriptOpenRe.MatchString(s) }},
		{"datastar bundle", `/static/js/datastar.js`, true, func(s string) bool { return datastarBundleRe.MatchString(s) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.re(c.val); got != c.fire {
				t.Errorf("%q: got %v want %v", c.val, got, c.fire)
			}
		})
	}
}
