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
				`data-on:intersect="@get(&#39;/wallet/history&#39;)"`,
				`aria-busy="true"`,
			},
		},
		{
			name: "onload for behind-a-tab",
			p:    DeferProps{ID: "tabbed", Height: "10rem", Width: "30rem", Get: "/x", OnLoad: true},
			want: []string{
				`width:30rem`,
				`data-on:load="@get(&#39;/x&#39;)"`,
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
	out := render(t, I("gear", Spin, IconLabel("settings")))
	for _, w := range []string{`icon-gear`, `icon-spin`, `role="img"`, `aria-label="settings"`} {
		if !strings.Contains(out, w) {
			t.Errorf("icon missing %q: %s", w, out)
		}
	}
	// Decorative (no label) is aria-hidden.
	dec := render(t, I("bell"))
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

func TestRegistryMandatoryEnforcement(t *testing.T) {
	reset()
	t.Cleanup(reset)

	// No kit registered: Mandatory panics (the enforcement).
	assertPanics(t, "Mandatory with no kit", func() { Mandatory() })

	Register(noopKit{})
	if got := Mandatory().Name(); got != "noop" {
		t.Fatalf("Mandatory() = %q, want noop", got)
	}

	// Duplicate registration panics.
	assertPanics(t, "duplicate Register", func() { Register(noopKit{}) })

	// SetMandatory of an unknown kit panics.
	assertPanics(t, "SetMandatory unknown", func() { SetMandatory("ghost") })

	if names := Registered(); len(names) != 1 || names[0] != "noop" {
		t.Fatalf("Registered() = %v, want [noop]", names)
	}
}

func TestContextInjectionFallsBackToMandatory(t *testing.T) {
	reset()
	t.Cleanup(reset)
	Register(noopKit{})

	// No kit in context -> falls back to Mandatory().
	if KitFrom(context.Background()).Name() != "noop" {
		t.Fatal("KitFrom should fall back to Mandatory")
	}

	// Explicit kit in context wins.
	ctx := WithKit(context.Background(), tagged{"custom"})
	if got := KitFrom(ctx).Name(); got != "custom" {
		t.Fatalf("KitFrom = %q, want custom", got)
	}
}

func TestNoopKitRendersAllSubInterfaces(t *testing.T) {
	k := NoopKit()
	// Smoke-render one component per sub-interface; assert non-empty, escaped,
	// and token-class-bearing markup. This proves the interface is satisfiable.
	cases := map[string]templ.Component{
		"container": k.Layout().Container(ContainerProps{}, k.Layout().PageHeading(PageHeadingProps{Title: "Hi"})),
		"card":      k.Layout().Card(CardProps{Surface: SurfacePanel, Heading: "Wallet"}),
		"navbar":    k.Nav().Navbar(NavbarProps{Brand: "B", Items: []NavItem{{Label: "Home", Href: "/", Active: true}}}),
		"button":    k.Buttons().Button(ButtonProps{Label: "Go", Role: RoleAccent, LeadIcon: "play"}),
		"field":     k.Forms().Field(FieldProps{Label: "Email", Error: "required"}, k.Forms().TextInput(TextInputProps{Name: "email", Error: true})),
		"table":     k.Data().Table(TableProps{Columns: []Column{{Header: "Name"}}, Rows: [][]string{{"a"}}}),
		"emptytbl":  k.Data().Table(TableProps{Empty: "Nothing here"}),
		"stat":      k.Data().Stat(StatProps{Label: "Users", Value: "12", Delta: Delta{Value: "3", Dir: "up"}}),
		"alert":     k.Feedback().Alert(AlertProps{Role: RoleWarn, Title: "Heads up"}),
		"spinner":   k.Feedback().Spinner(SpinnerProps{Block: true}),
		"modal":     k.Overlays().Modal(ModalProps{ID: "m", Title: "Confirm"}),
		"image":     k.Media().Image(ImageProps{Src: "/a.avif", Alt: "x", Ratio: RatioPhoto, Fit: FitCover}),
		"avatar":    k.Media().Avatar(AvatarProps{Initials: "HS"}),
		"msglist":   k.Messaging().MessageList(MessageListProps{Messages: []Message{{Body: "hi", Own: true}}}),
		"presence":  k.Messaging().PresencePill(PresencePillProps{State: "connected", Label: "online"}),
	}
	for name, c := range cases {
		out := render(t, c)
		if strings.TrimSpace(out) == "" {
			t.Errorf("%s rendered empty", name)
		}
		if strings.Contains(out, "#") && (strings.Contains(out, "color:#") || strings.Contains(out, "background:#")) {
			t.Errorf("%s appears to hardcode a color: %s", name, out)
		}
	}

	// Empty-state table shows the message.
	if !strings.Contains(render(t, k.Data().Table(TableProps{Empty: "Nothing here"})), "Nothing here") {
		t.Error("empty table should show empty message")
	}
	// Escaping holds.
	out := render(t, k.Layout().PageHeading(PageHeadingProps{Title: "<script>"}))
	if strings.Contains(out, "<script>") {
		t.Errorf("title not escaped: %s", out)
	}
}

func TestImageEagerVsLazy(t *testing.T) {
	k := NoopKit()
	lazy := render(t, k.Media().Image(ImageProps{Src: "/a", Alt: "a", Ratio: RatioWide}))
	if !strings.Contains(lazy, `loading="lazy"`) {
		t.Errorf("below-fold image should be lazy: %s", lazy)
	}
	eager := render(t, k.Media().Image(ImageProps{Src: "/a", Alt: "a", Ratio: RatioWide, Eager: true}))
	if !strings.Contains(eager, `loading="eager"`) || !strings.Contains(eager, `fetchpriority="high"`) {
		t.Errorf("hero image should be eager + high priority: %s", eager)
	}
	if !strings.Contains(eager, "ui-aspect-wide") {
		t.Errorf("image should sit in an aspect box: %s", eager)
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

func assertPanics(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected panic, got none", what)
		}
	}()
	fn()
}

// tagged is a minimal Kit used to test context injection without the no-op's
// name. It embeds noopKit for all methods and overrides Name.
type tagged struct{ name string }

func (k tagged) Name() string       { return k.name }
func (tagged) Layout() Layout       { return noopKit{} }
func (tagged) Nav() Nav             { return noopKit{} }
func (tagged) Buttons() Buttons     { return noopKit{} }
func (tagged) Forms() Forms         { return noopKit{} }
func (tagged) Data() Data           { return noopKit{} }
func (tagged) Feedback() Feedback   { return noopKit{} }
func (tagged) Overlays() Overlays   { return noopKit{} }
func (tagged) Media() Media         { return noopKit{} }
func (tagged) Messaging() Messaging { return noopKit{} }
