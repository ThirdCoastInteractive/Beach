package driftwood

import (
	"bytes"
	"context"
	"html"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
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
		"container":   {c: Container(ContainerProps{Width: MeasureText, Pad: SpaceLg}), children: []templ.Component{PageHeading(PageHeadingProps{Title: "Wallet", Subtitle: "Balances"})}},
		"card":        {c: Card(CardProps{Surface: SurfacePanel, Heading: "Holdings"}), children: []templ.Component{text("body")}},
		"cardwell":    {c: Card(CardProps{Surface: SurfaceWell})},
		"section":     {c: SectionHeading(SectionHeadingProps{Label: "Recent"})},
		"divider":     {c: Divider(DividerProps{Label: "or"})},
		"divplain":    {c: Divider(DividerProps{})},
		"navbar":      {c: Navbar(NavbarProps{Brand: "Beach", Items: []NavItem{{Label: "Home", Href: "/", Active: true, Icon: "home"}}})},
		"breadcrumbs": {c: Breadcrumbs(BreadcrumbsProps{Items: []NavItem{{Label: "Root", Href: "/"}, {Label: "Now"}}})},
		"tabs":        {c: Tabs(TabsProps{Name: "g", Label: "Views", Tabs: []Tab{{ID: "a", Label: "A", Active: true, Panel: text("panel a")}, {ID: "b", Label: "B", Get: "/b"}}})},
		"tabsplain":   {c: Tabs(TabsProps{Name: "h", Tabs: []Tab{{ID: "a", Label: "A", Active: true}}})},
		"pagination":  {c: Pagination(PaginationProps{Page: 2, Pages: 5, GetBase: "/list"})},
		"button":      {c: Button(ButtonProps{Label: "Go", Role: RoleAccent, LeadIcon: "play", Loading: "$busy"})},
		"iconbtn":     {c: IconButton(IconButtonProps{Icon: "gear", Label: "Settings"})},
		"field":       {c: Field(FieldProps{Label: "Email", For: "e", Error: "required"}), children: []templ.Component{Input(TextInputProps{ID: "e", Name: "email", Error: true, Prefix: "@"})}},
		"textarea":    {c: Textarea(TextareaProps{Name: "msg", Label: "Message", Rows: 3, AutoGrow: true})},
		"select":      {c: Select(SelectProps{Name: "tier", Label: "Tier", Placeholder: "Pick", Options: []Option{{Value: "a", Label: "A", Selected: true}}})},
		"selectfield": {c: Field(FieldProps{Label: "Tier", For: "tier"}), children: []templ.Component{Select(SelectProps{ID: "tier", Name: "tier", Options: []Option{{Value: "a", Label: "A"}}})}},
		"fieldgroup":  {c: Field(FieldProps{Label: "Tide window", Help: "Local time."}), children: []templ.Component{RadioGroup(RadioGroupProps{Name: "tide", Options: []RadioOption{{Value: "am", Label: "AM"}}})}},
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
		referencesResolve(t, name, out)
		controlsAreLabelled(t, name, out)
		clickablesAreNamed(t, name, out)
		rolesAreNamed(t, name, out)
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
	if !strings.Contains(btn, `data-attr:aria-busy="($busy) ? &#39;true&#39; : &#39;false&#39;"`) || !strings.Contains(btn, "dw-btn-spinner") {
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

// TestPageDeclaresRequestLanguage covers the join between i18n and the page
// shell: <html lang> and dir follow the request's locale, which is what lets a
// screen reader pick the right voice and pronunciation rules (WCAG 3.1.1). A
// page that renders Spanish under lang="en" is announced in an English accent,
// which is the sort of defect nobody sees and everybody using a screen reader
// hears immediately.
func TestPageDeclaresRequestLanguage(t *testing.T) {
	cases := []struct {
		name   string
		locale string
		props  PageProps
		want   string
	}{
		{"no locale configured", "", PageProps{}, `<html lang="en" dir="ltr"`},
		{"locale from the request", "es-ES", PageProps{}, `<html lang="es-ES" dir="ltr"`},
		{"an RTL locale flips dir", "ar-EG", PageProps{}, `<html lang="ar-EG" dir="rtl"`},
		{"explicit Lang wins", "es-ES", PageProps{Lang: "fr-CA"}, `<html lang="fr-CA" dir="ltr"`},
		{"explicit Dir wins", "ar-EG", PageProps{Dir: "ltr"}, `<html lang="ar-EG" dir="ltr"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.locale != "" {
				ctx = i18n.WithLocale(ctx, tc.locale)
			}
			var b bytes.Buffer
			if err := Page(tc.props).Render(ctx, &b); err != nil {
				t.Fatalf("render: %v", err)
			}
			if !strings.Contains(b.String(), tc.want) {
				t.Errorf("page shell missing %q\n%s", tc.want, b.String()[:200])
			}
		})
	}
}

// TestKitNamesComeFromTheCatalog proves the accessible names are translations
// and not literals: the same component, rendered under a locale that carries a
// different wording, announces differently.
func TestKitNamesComeFromTheCatalog(t *testing.T) {
	cat, err := i18n.Load(fstest.MapFS{
		"locales/en-US.json": &fstest.MapFile{Data: []byte(`{"ui.a11y.dismiss":"Dismiss"}`)},
		"locales/es-ES.json": &fstest.MapFile{Data: []byte(`{"ui.a11y.dismiss":"Descartar"}`)},
	}, "en-US")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	alert := Alert(AlertProps{Role: RoleInfo, Title: "Hi", Dismissible: true})
	for locale, want := range map[string]string{"en-US": `aria-label="Dismiss"`, "es-ES": `aria-label="Descartar"`} {
		ctx := i18n.WithCatalog(i18n.WithLocale(context.Background(), locale), cat)
		var b bytes.Buffer
		if err := alert.Render(ctx, &b); err != nil {
			t.Fatalf("render: %v", err)
		}
		if !strings.Contains(b.String(), want) {
			t.Errorf("locale %s: alert missing %s\n%s", locale, want, b.String())
		}
	}
}

func TestPageShell(t *testing.T) {
	out := render(t, Page(PageProps{Title: "Home", Description: "d"}), text("hello"))
	for _, want := range []string{
		"<!doctype html>", // templ normalizes the doctype to lowercase
		`<html lang="en" dir="ltr" class="dw-root">`,
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
