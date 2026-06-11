package chart

import (
	"strings"
	"testing"
)

func TestGallerySVGs(t *testing.T) {
	items := GallerySVGs()
	if len(items) == 0 {
		t.Fatal("GallerySVGs returned no items")
	}
	seen := map[string]bool{}
	for _, it := range items {
		if it.Title == "" {
			t.Errorf("item has empty title")
		}
		if seen[it.Title] {
			t.Errorf("duplicate title %q", it.Title)
		}
		seen[it.Title] = true
		svg := string(it.SVG)
		if !strings.HasPrefix(svg, "<svg") {
			t.Errorf("%s: SVG does not start with <svg: %.40q", it.Title, svg)
		}
		if !strings.HasSuffix(svg, "</svg>") {
			t.Errorf("%s: SVG does not end with </svg>: ...%.40q", it.Title, svg[max0(len(svg)-40):])
		}
		if !strings.Contains(svg, "var(--chart-qualitative-") &&
			!strings.Contains(svg, "var(--chart-diverging-") &&
			!strings.Contains(svg, "var(--success") &&
			!strings.Contains(svg, "var(--error") {
			t.Errorf("%s: SVG references no chart token color", it.Title)
		}
	}
	t.Logf("gallery rendered %d charts", len(items))
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
