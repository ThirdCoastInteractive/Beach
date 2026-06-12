package driftwood

import (
	"bytes"
	"context"
	"html"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// text is a tiny child component for feeding { children... } slots in tests.
func text(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, html.EscapeString(s))
		return err
	})
}

// render renders c; any children are joined and passed through the context so
// the component's { children... } slot receives them.
func render(t *testing.T, c templ.Component, children ...templ.Component) string {
	t.Helper()
	ctx := context.Background()
	if len(children) > 0 {
		ctx = templ.WithChildren(ctx, templ.Join(children...))
	}
	var b bytes.Buffer
	if err := c.Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

// noColorLiteral fails if the markup carries a hardcoded color (the kit must
// reference design tokens only). Markup never legitimately contains "color:#"
// or "background:#".
func noColorLiteral(t *testing.T, name, out string) {
	t.Helper()
	if strings.Contains(out, "color:#") || strings.Contains(out, "background:#") {
		t.Errorf("%s hardcodes a color: %s", name, out)
	}
}

// renderCase pairs a component with the children its slot should receive.
type renderCase struct {
	c        templ.Component
	children []templ.Component
}

func TestEveryCoreComponentRenders(t *testing.T) {
	cases := map[string]renderCase{
		"container":   {c: Container(ContainerProps{MaxWidth: "13"}), children: []templ.Component{PageHeading(PageHeadingProps{Title: "Wallet", Subtitle: "Balances"})}},
		"card":        {c: Card(CardProps{Surface: SurfacePanel, Heading: "Holdings"}), children: []templ.Component{text("body")}},
		"cardwell":    {c: Card(CardProps{Surface: SurfaceWell})},
		"section":     {c: SectionHeading(SectionHeadingProps{Label: "Recent"})},
		"divider":     {c: Divider(DividerProps{Label: "or"})},
		"divplain":    {c: Divider(DividerProps{})},
		"navbar":      {c: Navbar(NavbarProps{Brand: "Beach", Items: []NavItem{{Label: "Home", Href: "/", Active: true, Icon: "home"}}})},
		"breadcrumbs": {c: Breadcrumbs(BreadcrumbsProps{Items: []NavItem{{Label: "Root", Href: "/"}, {Label: "Now"}}})},
		"tabs":        {c: Tabs(TabsProps{Name: "g", Tabs: []Tab{{ID: "a", Label: "A", Active: true}, {ID: "b", Label: "B", Get: "/b"}}})},
		"pagination":  {c: Pagination(PaginationProps{Page: 2, Pages: 5, GetBase: "/list"})},
		"button":      {c: Button(ButtonProps{Label: "Go", Role: RoleAccent, LeadIcon: "play", Loading: "$busy"})},
		"iconbtn":     {c: IconButton(IconButtonProps{Icon: "gear", Label: "Settings"})},
		"field":       {c: Field(FieldProps{Label: "Email", For: "e", Error: "required"}), children: []templ.Component{Input(TextInputProps{ID: "e", Name: "email", Error: true, Prefix: "@"})}},
		"textarea":    {c: Textarea(TextareaProps{Name: "msg", Rows: 3, AutoGrow: true})},
		"select":      {c: Select(SelectProps{Name: "tier", Placeholder: "Pick", Options: []Option{{Value: "a", Label: "A", Selected: true}}})},
		"table":       {c: Table(TableProps{Columns: []Column{{Header: "Name", Get: "/sort"}, {Header: "Qty"}}, Rows: [][]string{{"Oats", "2"}}})},
		"emptytable":  {c: Table(TableProps{Empty: "Nothing here"})},
		"stat":        {c: Stat(StatProps{Label: "Users", Value: "12", Delta: Delta{Value: "3", Dir: "up"}})},
		"badge":       {c: Badge(BadgeProps{Label: "Online", Role: RoleGood, Dot: true})},
		"alert":       {c: Alert(AlertProps{Role: RoleWarn, Title: "Heads up", Dismissible: true}), children: []templ.Component{text("careful")}},
		"empty":       {c: EmptyState(EmptyStateProps{Icon: "inbox", Message: "No items"})},
		"spinner":     {c: Spinner(SpinnerProps{Block: true, Size: SizeLg})},
		"skeleton":    {c: Skeleton(SkeletonProps{Width: "100%", Height: "4rem"})},
		"modal":       {c: Modal(ModalProps{ID: "m", Title: "Confirm"}), children: []templ.Component{text("sure?")}},
		"tooltip":     {c: Tooltip(TooltipProps{Text: "Hi"}), children: []templ.Component{text("?")}},
		"avatar":      {c: Avatar(AvatarProps{Initials: "HS", Status: "online"})},
		"avatarimg":   {c: Avatar(AvatarProps{Src: "/a.jpg", Alt: "me"})},
		"image":       {c: Image(ImageProps{Src: "/a.avif", Alt: "x", Ratio: RatioPhoto, Fit: FitCover, Eager: true})},
		"aspect":      {c: AspectRatio(AspectBoxProps{Ratio: RatioWide})},
		"msglist":     {c: MessageList(MessageListProps{Messages: []Message{{Author: "A", Body: "hi", Own: true, At: "1m"}, {Body: "joined", System: true}}})},
		"composer":    {c: Composer(ComposerProps{Name: "draft", Placeholder: "Say…", Post: "/send"})},
		"presence":    {c: Presence(PresencePillProps{State: "connected", Label: "online"})},
	}
	for name, tc := range cases {
		out := render(t, tc.c, tc.children...)
		if strings.TrimSpace(out) == "" {
			t.Errorf("%s rendered empty", name)
		}
		noColorLiteral(t, name, out)
	}
}

func TestSelectedMarkupDetails(t *testing.T) {
	// Eager image opts into high priority and sits in an aspect box.
	img := render(t, Image(ImageProps{Src: "/a", Alt: "a", Ratio: RatioWide, Eager: true}))
	for _, want := range []string{`loading="eager"`, `fetchpriority="high"`, "dw-aspect-wide"} {
		if !strings.Contains(img, want) {
			t.Errorf("image missing %q: %s", want, img)
		}
	}

	// Sortable header wires a hypermedia @get.
	tbl := render(t, Table(TableProps{Columns: []Column{{Header: "Name", Get: "/sort"}}, Rows: [][]string{{"x"}}}))
	if !strings.Contains(tbl, "data-on:click=") || !strings.Contains(tbl, "/sort") {
		t.Errorf("sortable header missing @get: %s", tbl)
	}

	// Empty table shows its message and no <table>.
	empty := render(t, Table(TableProps{Empty: "Nothing here"}))
	if !strings.Contains(empty, "Nothing here") || strings.Contains(empty, "<table") {
		t.Errorf("empty table wrong: %s", empty)
	}

	// Loading button binds aria-busy to the signal and renders a spinner.
	btn := render(t, Button(ButtonProps{Label: "Go", Loading: "$busy"}))
	if !strings.Contains(btn, `data-attr:aria-busy="$busy"`) || !strings.Contains(btn, "dw-btn-spinner") {
		t.Errorf("loading button wrong: %s", btn)
	}

	// A named input binds its same-named Datastar signal.
	in := render(t, Input(TextInputProps{Name: "email"}))
	if !strings.Contains(in, "data-bind:email") {
		t.Errorf("input missing signal bind: %s", in)
	}

	// Escaping holds for interpolated text.
	h := render(t, PageHeading(PageHeadingProps{Title: "<script>"}))
	if strings.Contains(h, "<script>") {
		t.Errorf("title not escaped: %s", h)
	}
}

func TestPageShell(t *testing.T) {
	out := render(t, Page(PageProps{Title: "Home", Description: "d"}), text("hello"))
	for _, want := range []string{
		"<!doctype html>", // templ normalizes the doctype to lowercase
		`<html lang="en" class="dw-root">`,
		`<link rel="stylesheet" href="/static/css/app.css">`,
		`<script type="module" src="/static/js/datastar.js" defer></script>`,
		`<script type="module" src="/static/js/chart.js" defer></script>`,
		`<title>Home</title>`,
		"hello",
		"</html>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("page shell missing %q", want)
		}
	}
}
