package lint

import (
	"path/filepath"
	"sort"
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
