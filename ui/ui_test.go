package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// render runs a component to a string for assertions.
func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestDeferReservesSpaceAndWiresGet(t *testing.T) {
	tests := []struct {
		name string
		p    DeferProps
		want []string
	}{
		{
			name: "intersection by default",
			p:    DeferProps{ID: "trade-history", Height: "24rem", Get: "/wallet/history"},
			want: []string{
				`id="trade-history"`,
				`height:24rem`,
				`width:100%`,
				`data-on-intersect="@get(&#39;/wallet/history&#39;)"`,
				`aria-busy="true"`,
			},
		},
		{
			name: "init for behind-a-tab",
			p:    DeferProps{ID: "tabbed", Height: "10rem", Width: "30rem", Get: "/x", OnLoad: true},
			want: []string{
				`width:30rem`,
				`data-init="@get(&#39;/x&#39;)"`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := render(t, Defer(tt.p))
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("Defer output missing %q\ngot: %s", w, out)
				}
			}
		})
	}
}

func TestIconRecordsGlyphAndRenders(t *testing.T) {
	out := render(t, Icon("gear", Spin, IconLabel("settings")))
	for _, w := range []string{`icon-gear`, `icon-spin`, `role="img"`, `aria-label="settings"`} {
		if !strings.Contains(out, w) {
			t.Errorf("icon missing %q: %s", w, out)
		}
	}
	// Decorative (no label) is aria-hidden.
	dec := render(t, Icon("bell"))
	if !strings.Contains(dec, `aria-hidden="true"`) {
		t.Errorf("decorative icon should be aria-hidden: %s", dec)
	}
	// The referenced glyphs are recorded for font subsetting.
	used := UsedIcons()
	if !contains(used, "gear") || !contains(used, "bell") {
		t.Errorf("UsedIcons missing referenced glyphs: %v", used)
	}
	// Sorted output.
	for i := 1; i < len(used); i++ {
		if used[i-1] > used[i] {
			t.Errorf("UsedIcons not sorted: %v", used)
		}
	}
}

func TestIconSizeStep(t *testing.T) {
	out := render(t, Icon("gear", IconSize(SizeLg)))
	if !strings.Contains(out, `icon-lg`) {
		t.Errorf("sized icon missing size class: %s", out)
	}
}

// --- helpers ---

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
