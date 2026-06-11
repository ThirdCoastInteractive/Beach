package driftwood

import (
	"strings"
	"testing"

	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// TestCatalogComponentsRender smoke-renders every catalog component added on top
// of the Core set: non-empty markup, no hardcoded color, and the key structural
// hooks (native popover/dialog, hypermedia @-attrs) where they matter.
func TestCatalogComponentsRender(t *testing.T) {
	k := New()
	side := k.Nav().SidebarNav(ui.SidebarNavProps{Sections: []ui.SidebarSection{
		{Label: "Main", Items: []ui.NavItem{{Label: "Home", Href: "/", Active: true, Icon: "home"}}},
	}})
	cases := map[string]templ.Component{
		"appshell":  k.Layout().AppShell(ui.AppShellProps{Topbar: txt("bar"), Sidebar: side}, txt("main")),
		"appstack":  k.Layout().AppShell(ui.AppShellProps{Topbar: txt("bar")}, txt("main")),
		"cardhead":  k.Layout().CardHeading(ui.CardHeadingProps{Title: "Holdings", Meta: "updated 1m", Action: txt("x")}),
		"grid":      k.Layout().Grid(ui.GridProps{Cols: 4, Gap: "1rem"}, txt("a"), txt("b")),
		"split":     k.Layout().Split(ui.SplitProps{List: txt("list"), Detail: txt("detail")}),
		"sidebar":   side,
		"btngroup":  k.Buttons().ButtonGroup(ui.ButtonGroupProps{Label: "View"}, k.Buttons().Button(ui.ButtonProps{Label: "Day"})),
		"menubtn":   k.Buttons().MenuButton(ui.MenuButtonProps{ID: "m", Label: "Actions", Items: []ui.MenuItem{{Label: "Edit", Href: "/e"}, {Label: "Delete", On: "@post('/d')"}}}),
		"form":      k.Forms().Form(ui.FormProps{Post: "/save", TwoCol: true}, txt("fields")),
		"checkbox":  k.Forms().Checkbox(ui.CheckboxProps{Name: "agree", Label: "I agree", Checked: true}),
		"radio":     k.Forms().RadioGroup(ui.RadioGroupProps{Name: "plan", Inline: true, Options: []ui.RadioOption{{Value: "a", Label: "A", Checked: true}, {Value: "b", Label: "B"}}}),
		"toggle":    k.Forms().Toggle(ui.ToggleProps{Name: "live", Label: "Live", Description: "stream updates"}),
		"inputgrp":  k.Forms().InputGroup(ui.InputGroupProps{Input: ui.TextInputProps{Name: "q", Placeholder: "Search"}, Button: ui.ButtonProps{Label: "Go", Type: "submit"}}),
		"formerr":   k.Forms().FormError(ui.FormErrorProps{Title: "Fix these", Messages: []string{"Email required", "Name too long"}}),
		"desclist":  k.Data().DescList(ui.DescListProps{TwoCol: true, Items: []ui.DescItem{{Term: "Name", Value: "Oats"}}}),
		"stacked":   k.Data().StackedList(ui.StackedListProps{Rows: []ui.ListRow{{Title: "Row", Meta: "meta", Badge: &ui.BadgeProps{Label: "New", Role: ui.RoleGood}}}}),
		"gridlist":  k.Data().GridList(ui.GridListProps{Cols: 3}, txt("card")),
		"toast":     k.Feedback().Toast(ui.ToastProps{Role: ui.RoleGood, Title: "Saved", Message: "All set"}),
		"progress":  k.Feedback().Progress(ui.ProgressProps{Value: 3, Max: 10, Label: "Upload"}),
		"progind":   k.Feedback().Progress(ui.ProgressProps{Label: "Loading"}),
		"drawer":    k.Overlays().Drawer(ui.DrawerProps{ID: "d", Title: "Filters", Side: "left"}, txt("body")),
		"popover":   k.Overlays().Popover(ui.PopoverProps{ID: "p", Label: "Info"}, txt("panel")),
		"figure":    k.Media().Figure(ui.FigureProps{Image: ui.ImageProps{Src: "/a.avif", Alt: "x", Ratio: ui.RatioPhoto}, Caption: "A caption"}),
	}
	for name, c := range cases {
		out := render(t, c)
		if strings.TrimSpace(out) == "" {
			t.Errorf("%s rendered empty", name)
		}
		noColorLiteral(t, name, out)
	}
}

func TestCatalogStructuralDetails(t *testing.T) {
	k := New()

	// Menu button drives a native popover (no script to toggle) and wires a
	// hypermedia @-expression on an action item.
	mb := render(t, k.Buttons().MenuButton(ui.MenuButtonProps{ID: "m", Label: "Go", Items: []ui.MenuItem{{Label: "Del", On: "@post('/d')"}}}))
	for _, want := range []string{`popovertarget="m-panel"`, `popover`, `data-on:click="@post(&#39;/d&#39;)"`} {
		if !strings.Contains(mb, want) {
			t.Errorf("menu button missing %q: %s", want, mb)
		}
	}

	// Drawer is a native <dialog> with a method=dialog close (platform open/close).
	dr := render(t, k.Overlays().Drawer(ui.DrawerProps{ID: "d", Title: "T"}, txt("x")))
	if !strings.Contains(dr, "<dialog") || !strings.Contains(dr, `method="dialog"`) {
		t.Errorf("drawer should be a native dialog: %s", dr)
	}

	// Form wires its hypermedia submit, not a client action.
	fm := render(t, k.Forms().Form(ui.FormProps{Post: "/save"}, txt("f")))
	if !strings.Contains(fm, "data-on:submit=") || !strings.Contains(fm, "/save") {
		t.Errorf("form missing @post submit: %s", fm)
	}

	// Determinate progress reports its ARIA range; indeterminate omits valuenow.
	det := render(t, k.Feedback().Progress(ui.ProgressProps{Value: 5, Max: 10, Label: "L"}))
	if !strings.Contains(det, `aria-valuenow="5"`) || !strings.Contains(det, `aria-valuemax="10"`) {
		t.Errorf("determinate progress missing ARIA range: %s", det)
	}
	ind := render(t, k.Feedback().Progress(ui.ProgressProps{Label: "L"}))
	if strings.Contains(ind, "aria-valuenow") || !strings.Contains(ind, "is-indeterminate") {
		t.Errorf("indeterminate progress wrong: %s", ind)
	}

	// Toggle's checked state and switch role survive into markup.
	tg := render(t, k.Forms().Toggle(ui.ToggleProps{Name: "x", Label: "L", Checked: true}))
	if !strings.Contains(tg, `role="switch"`) || !strings.Contains(tg, "checked") {
		t.Errorf("toggle missing switch/checked: %s", tg)
	}
}
