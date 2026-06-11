package driftwood

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

// noColorLiteral fails if the markup carries a hardcoded color (the kit must
// reference Stellar tokens only). Markup never legitimately contains "color:#"
// or "background:#".
func noColorLiteral(t *testing.T, name, out string) {
	t.Helper()
	if strings.Contains(out, "color:#") || strings.Contains(out, "background:#") {
		t.Errorf("%s hardcodes a color: %s", name, out)
	}
}

func TestKitRegistersUnderName(t *testing.T) {
	if New().Name() != "driftwood" {
		t.Fatalf("Name() = %q", New().Name())
	}
	if k, ok := ui.Lookup("driftwood"); !ok || k.Name() != "driftwood" {
		t.Fatalf("driftwood not registered via init(): ok=%v", ok)
	}
}

func TestEverySubInterfaceRenders(t *testing.T) {
	k := New()
	cases := map[string]templ.Component{
		"container":   k.Layout().Container(ui.ContainerProps{MaxWidth: "13"}, k.Layout().PageHeading(ui.PageHeadingProps{Title: "Wallet", Subtitle: "Balances"})),
		"card":        k.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: "Holdings"}, txt("body")),
		"cardwell":    k.Layout().Card(ui.CardProps{Surface: ui.SurfaceWell}),
		"section":     k.Layout().SectionHeading(ui.SectionHeadingProps{Label: "Recent"}),
		"divider":     k.Layout().Divider(ui.DividerProps{Label: "or"}),
		"divplain":    k.Layout().Divider(ui.DividerProps{}),
		"navbar":      k.Nav().Navbar(ui.NavbarProps{Brand: "Beach", Items: []ui.NavItem{{Label: "Home", Href: "/", Active: true, Icon: "home"}}}),
		"breadcrumbs": k.Nav().Breadcrumbs(ui.BreadcrumbsProps{Items: []ui.NavItem{{Label: "Root", Href: "/"}, {Label: "Now"}}}),
		"tabs":        k.Nav().Tabs(ui.TabsProps{Name: "g", Tabs: []ui.Tab{{ID: "a", Label: "A", Active: true}, {ID: "b", Label: "B", Get: "/b"}}}),
		"pagination":  k.Nav().Pagination(ui.PaginationProps{Page: 2, Pages: 5, GetBase: "/list"}),
		"button":      k.Buttons().Button(ui.ButtonProps{Label: "Go", Role: ui.RoleAccent, LeadIcon: "play", Loading: "$busy"}),
		"iconbtn":     k.Buttons().IconButton(ui.IconButtonProps{Icon: "gear", Label: "Settings"}),
		"field":       k.Forms().Field(ui.FieldProps{Label: "Email", For: "e", Error: "required"}, k.Forms().TextInput(ui.TextInputProps{ID: "e", Name: "email", Error: true, Prefix: "@"})),
		"textarea":    k.Forms().Textarea(ui.TextareaProps{Name: "msg", Rows: 3, AutoGrow: true}),
		"select":      k.Forms().Select(ui.SelectProps{Name: "tier", Placeholder: "Pick", Options: []ui.Option{{Value: "a", Label: "A", Selected: true}}}),
		"table":       k.Data().Table(ui.TableProps{Columns: []ui.Column{{Header: "Name", Get: "/sort"}, {Header: "Qty"}}, Rows: [][]string{{"Oats", "2"}}}),
		"emptytable":  k.Data().Table(ui.TableProps{Empty: "Nothing here"}),
		"stat":        k.Data().Stat(ui.StatProps{Label: "Users", Value: "12", Delta: ui.Delta{Value: "3", Dir: "up"}}),
		"badge":       k.Data().Badge(ui.BadgeProps{Label: "Online", Role: ui.RoleGood, Dot: true}),
		"alert":       k.Feedback().Alert(ui.AlertProps{Role: ui.RoleWarn, Title: "Heads up", Dismissible: true}, txt("careful")),
		"empty":       k.Feedback().EmptyState(ui.EmptyStateProps{Icon: "inbox", Message: "No items"}),
		"spinner":     k.Feedback().Spinner(ui.SpinnerProps{Block: true, Size: ui.SizeLg}),
		"skeleton":    k.Feedback().Skeleton(ui.SkeletonProps{Width: "100%", Height: "4rem"}),
		"modal":       k.Overlays().Modal(ui.ModalProps{ID: "m", Title: "Confirm"}, txt("sure?")),
		"tooltip":     k.Overlays().Tooltip(ui.TooltipProps{Text: "Hi"}, txt("?")),
		"avatar":      k.Media().Avatar(ui.AvatarProps{Initials: "HS", Status: "online"}),
		"avatarimg":   k.Media().Avatar(ui.AvatarProps{Src: "/a.jpg", Alt: "me"}),
		"image":       k.Media().Image(ui.ImageProps{Src: "/a.avif", Alt: "x", Ratio: ui.RatioPhoto, Fit: ui.FitCover, Eager: true}),
		"aspect":      k.Media().AspectBox(ui.AspectBoxProps{Ratio: ui.RatioWide}),
		"msglist":     k.Messaging().MessageList(ui.MessageListProps{Messages: []ui.Message{{Author: "A", Body: "hi", Own: true, At: "1m"}, {Body: "joined", System: true}}}),
		"composer":    k.Messaging().Composer(ui.ComposerProps{Name: "draft", Placeholder: "Say…", Post: "/send"}),
		"presence":    k.Messaging().PresencePill(ui.PresencePillProps{State: "connected", Label: "online"}),
	}
	for name, c := range cases {
		out := render(t, c)
		if strings.TrimSpace(out) == "" {
			t.Errorf("%s rendered empty", name)
		}
		noColorLiteral(t, name, out)
	}
}

func TestSelectedMarkupDetails(t *testing.T) {
	k := New()

	// Eager image opts into high priority and sits in an aspect box.
	img := render(t, k.Media().Image(ui.ImageProps{Src: "/a", Alt: "a", Ratio: ui.RatioWide, Eager: true}))
	for _, want := range []string{`loading="eager"`, `fetchpriority="high"`, "dw-aspect-wide"} {
		if !strings.Contains(img, want) {
			t.Errorf("image missing %q: %s", want, img)
		}
	}

	// Sortable header wires a hypermedia @get.
	tbl := render(t, k.Data().Table(ui.TableProps{Columns: []ui.Column{{Header: "Name", Get: "/sort"}}, Rows: [][]string{{"x"}}}))
	if !strings.Contains(tbl, "data-on:click=") || !strings.Contains(tbl, "/sort") {
		t.Errorf("sortable header missing @get: %s", tbl)
	}

	// Empty table shows its message and no <table>.
	empty := render(t, k.Data().Table(ui.TableProps{Empty: "Nothing here"}))
	if !strings.Contains(empty, "Nothing here") || strings.Contains(empty, "<table") {
		t.Errorf("empty table wrong: %s", empty)
	}

	// Loading button binds aria-busy to the signal and renders a spinner.
	btn := render(t, k.Buttons().Button(ui.ButtonProps{Label: "Go", Loading: "$busy"}))
	if !strings.Contains(btn, `data-attr-aria-busy="$busy"`) || !strings.Contains(btn, "dw-btn-spinner") {
		t.Errorf("loading button wrong: %s", btn)
	}

	// Escaping holds for interpolated text.
	h := render(t, k.Layout().PageHeading(ui.PageHeadingProps{Title: "<script>"}))
	if strings.Contains(h, "<script>") {
		t.Errorf("title not escaped: %s", h)
	}
}

func TestPageShell(t *testing.T) {
	out := render(t, Page(PageProps{Title: "Home", Description: "d"}, txt("hello")))
	for _, want := range []string{
		"<!DOCTYPE html>",
		`<html lang="en" class="dw-root">`,
		`<link rel="stylesheet" href="/static/css/stellar.css">`,
		`href="/static/css/driftwood.css"`,
		`<title>Home</title>`,
		"hello",
		"</html>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("page shell missing %q", want)
		}
	}
}
