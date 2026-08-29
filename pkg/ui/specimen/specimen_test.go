package specimen

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestPageRenders smoke-renders the full specimen document: it must be a
// complete HTML page containing the token sheet, the icon set, the gamut
// strips, and every chart widget wrapped in the dash-widget structure the
// chart toolbar attaches to.
func TestPageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := Page().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(strings.ToLower(out), "<!doctype html") {
		t.Error("specimen should be a full document (driftwood.Page)")
	}
	for _, want := range []string{
		"--color-series-a",  // token sheet series strip
		"--color-paper",     // token sheet surface swatch
		"icon icon-gear",    // icon set
		"dash-widget-body",  // chart gallery card structure
		"spec-line",         // line chart hover id
		"race-tide-rentals", // bar-race static frame, stable slug id
		"Munsell",           // gamut provenance line
	} {
		if !strings.Contains(out, want) {
			t.Errorf("specimen missing %q", want)
		}
	}

	// Every gallery chart sits in its own dash-widget card: 22 chart types
	// plus the static bar-race frame.
	if got := strings.Count(out, `class="dash-widget `); got < 23 {
		t.Errorf("expected at least 23 dash-widget cards, got %d", got)
	}
}

// TestContrastRowsAllPass asserts the page the framework shows off with does not
// advertise a failure. Every pair in the specimen's table is one the palette test
// also asserts, so this failing means the two lists have diverged.
func TestContrastRowsAllPass(t *testing.T) {
	rows := contrastRows()
	if len(rows) != len(contrastPairs) {
		t.Fatalf("got %d rows for %d pairs", len(rows), len(contrastPairs))
	}
	for i, r := range rows {
		if r[len(r)-1] != "pass" {
			t.Errorf("%s: %s (needs %s, WCAG %s)", contrastPairs[i].what, r[1], r[2], r[3])
		}
	}
}
