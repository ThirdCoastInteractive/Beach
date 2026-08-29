package driftwood

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestCatalogComponentsRender smoke-renders every catalog component added on top
// of the Core set: non-empty markup, no hardcoded color, and the key structural
// hooks (native popover/dialog, hypermedia @-attrs) where they matter.
func TestCatalogComponentsRender(t *testing.T) {
	side := Sidebar(SidebarNavProps{Sections: []SidebarSection{
		{Label: "Main", Items: []NavItem{{Label: "Home", Href: "/", Active: true, Icon: "home"}}},
	}})
	cases := map[string]renderCase{
		"shell":      {c: Shell(AppShellProps{Topbar: text("bar"), Sidebar: side}), children: []templ.Component{text("main")}},
		"shellstack": {c: Shell(AppShellProps{Topbar: text("bar")}), children: []templ.Component{text("main")}},
		"cardhead":   {c: CardHeading(CardHeadingProps{Title: "Holdings", Meta: "updated 1m", Action: text("x")})},
		"grid":       {c: Grid(GridProps{Cols: 4, Gap: "1rem"}), children: []templ.Component{text("a"), text("b")}},
		"split":      {c: Split(SplitProps{List: text("list"), Detail: text("detail")})},
		"sidebar":    {c: side},
		"btngroup":   {c: ButtonGroup(ButtonGroupProps{Label: "View"}), children: []templ.Component{Button(ButtonProps{Label: "Day"})}},
		"segmented":  {c: Segmented(SegmentedProps{Name: "Range", Segments: []Segment{{Value: "d", Label: "Day", Active: true}, {Value: "w", Label: "Week", On: "@get('/r/w')"}}})},
		"fieldset":   {c: Fieldset(FieldsetProps{Legend: "Contact", Help: "who to call", Disabled: true}), children: []templ.Component{text("fields")}},
		"selectgrp":  {c: Select(SelectProps{Name: "board", Label: "Board", Placeholder: "Pick", Groups: []OptionGroup{{Label: "Long", Options: []Option{{Value: "log", Label: "Log", Selected: true}}}}})},
		"menubtn":    {c: MenuButton(MenuButtonProps{ID: "m", Label: "Actions", Items: []MenuItem{{Label: "Edit", Href: "/e"}, {Label: "Delete", On: "@post('/d')"}}})},
		"form":       {c: Form(FormProps{Post: "/save", TwoCol: true}), children: []templ.Component{text("fields")}},
		"checkbox":   {c: Checkbox(CheckboxProps{Name: "agree", Label: "I agree", Checked: true})},
		"radio":      {c: RadioGroup(RadioGroupProps{Name: "plan", Label: "Plan", Inline: true, Options: []RadioOption{{Value: "a", Label: "A", Checked: true}, {Value: "b", Label: "B"}}})},
		"toggle":     {c: Toggle(ToggleProps{Name: "live", Label: "Live", Description: "stream updates"})},
		"inputgrp":   {c: InputGroup(InputGroupProps{Input: TextInputProps{Name: "q", Label: "Search", Placeholder: "Search"}, Button: ButtonProps{Label: "Go", Type: "submit"}})},
		"formerr":    {c: FormError(FormErrorProps{Title: "Fix these", Messages: []string{"Email required", "Name too long"}})},
		"desclist":   {c: DescList(DescListProps{TwoCol: true, Items: []DescItem{{Term: "Name", Value: "Oats"}}})},
		"stacked":    {c: StackedList(StackedListProps{Rows: []ListRow{{Title: "Row", Meta: "meta", Badge: &BadgeProps{Label: "New", Role: RoleGood}}}})},
		"gridlist":   {c: GridList(GridListProps{Cols: 3}), children: []templ.Component{text("card")}},
		"toast":      {c: Toast(ToastProps{Role: RoleGood, Title: "Saved", Message: "All set"})},
		"progress":   {c: Progress(ProgressProps{Value: 3, Max: 10, Label: "Upload"})},
		"progind":    {c: Progress(ProgressProps{Label: "Loading"})},
		"drawer":     {c: Drawer(DrawerProps{ID: "d", Title: "Filters", Side: "left"}), children: []templ.Component{text("body")}},
		"popover":    {c: Popover(PopoverProps{ID: "p", Label: "Info"}), children: []templ.Component{text("panel")}},
		"figure":     {c: Figure(FigureProps{Image: ImageProps{Src: "/a.avif", Alt: "x", Ratio: RatioPhoto}, Caption: "A caption"})},
		"erralert":   {c: ErrorAlert("err-1", true, "Save failed", "Try again")},
		"livetoggle": {c: LiveToggle(LiveToggleProps{ID: "live", Stream: "/live", Label: "Board"})},
		"livestream": {c: LiveStream(LiveStreamProps{ID: "hand", Stream: "/hand"})},
		"video":      {c: Video(VideoProps{Src: "/v.mp4", Poster: "/v.jpg", Ratio: RatioWide, Label: "Site tour", Tracks: []Track{{Src: "/v.en.vtt", Lang: "en", Label: "English", Default: true}}})},
		"mediaobj":   {c: MediaObject(MediaObjectProps{Media: Avatar(AvatarProps{Initials: "HS"}), Body: text("said something"), Actions: text("x")})},
		"confirm":    {c: Confirm(ConfirmProps{ID: "del", Title: "Delete booking?", Message: "The guest is notified and this cannot be undone.", ConfirmLabel: "Delete booking", Post: "/bookings/1/delete", Hazard: true})},
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

func TestCatalogStructuralDetails(t *testing.T) {
	// Menu button drives a native popover (no script to toggle) and wires a
	// hypermedia @-expression on an action item.
	mb := render(t, MenuButton(MenuButtonProps{ID: "m", Label: "Go", Items: []MenuItem{{Label: "Del", On: "@post('/d')"}}}))
	for _, want := range []string{`popovertarget="m-panel"`, `popover`, `data-on:click="@post(&#39;/d&#39;)"`} {
		if !strings.Contains(mb, want) {
			t.Errorf("menu button missing %q: %s", want, mb)
		}
	}

	// Drawer is a native <dialog> with a method=dialog close (platform open/close).
	dr := render(t, Drawer(DrawerProps{ID: "d", Title: "T"}), text("x"))
	if !strings.Contains(dr, "<dialog") || !strings.Contains(dr, `method="dialog"`) {
		t.Errorf("drawer should be a native dialog: %s", dr)
	}

	// Form wires its hypermedia submit, not a client action.
	fm := render(t, Form(FormProps{Post: "/save"}), text("f"))
	if !strings.Contains(fm, "data-on:submit=") || !strings.Contains(fm, "/save") {
		t.Errorf("form missing @post submit: %s", fm)
	}

	// Determinate progress reports its ARIA range; indeterminate omits valuenow.
	det := render(t, Progress(ProgressProps{Value: 5, Max: 10, Label: "L"}))
	if !strings.Contains(det, `aria-valuenow="5"`) || !strings.Contains(det, `aria-valuemax="10"`) {
		t.Errorf("determinate progress missing ARIA range: %s", det)
	}
	ind := render(t, Progress(ProgressProps{Label: "L"}))
	if strings.Contains(ind, "aria-valuenow") || !strings.Contains(ind, "is-indeterminate") {
		t.Errorf("indeterminate progress wrong: %s", ind)
	}

	// Toggle's checked state and switch role survive into markup.
	tg := render(t, Toggle(ToggleProps{Name: "x", Label: "L", Checked: true}))
	if !strings.Contains(tg, `role="switch"`) || !strings.Contains(tg, "checked") {
		t.Errorf("toggle missing switch/checked: %s", tg)
	}

	// Segmented is a single-select toggle group: role=group, aria-pressed per
	// segment (true on the active one), and an @-expression wired on click.
	seg := render(t, Segmented(SegmentedProps{Name: "Range", Segments: []Segment{
		{Value: "d", Label: "Day", Active: true},
		{Value: "w", Label: "Week", On: "@get('/r/w')"},
	}}))
	for _, want := range []string{`role="group"`, `aria-label="Range"`, `aria-pressed="true"`, `aria-pressed="false"`, "data-on:click="} {
		if !strings.Contains(seg, want) {
			t.Errorf("segmented missing %q: %s", want, seg)
		}
	}

	// Grouped Select renders <optgroup> with a disabled option inside.
	sel := render(t, Select(SelectProps{Name: "board", Groups: []OptionGroup{
		{Label: "Long", Options: []Option{{Value: "log", Label: "Log"}, {Value: "gun", Label: "Gun", Disabled: true}}},
	}}))
	for _, want := range []string{`<optgroup label="Long">`, `disabled`, "</optgroup>"} {
		if !strings.Contains(sel, want) {
			t.Errorf("grouped select missing %q: %s", want, sel)
		}
	}

	// Disabled Select carries the native disabled attribute on the control.
	dsel := render(t, Select(SelectProps{Name: "x", Disabled: true, Options: []Option{{Value: "a", Label: "A"}}}))
	if !strings.Contains(dsel, "<select") || !strings.Contains(dsel, "disabled") {
		t.Errorf("disabled select missing disabled attr: %s", dsel)
	}

	// Numeric input threads step/min/max/inputmode/autocomplete/spellcheck through.
	num := render(t, Input(TextInputProps{Name: "qty", Type: "number", Min: "1", Max: "9", Step: "1", InputMode: "numeric", Autocomplete: AutocompleteOff, Spellcheck: "false"}))
	for _, want := range []string{`type="number"`, `min="1"`, `max="9"`, `step="1"`, `inputmode="numeric"`, `autocomplete="off"`, `spellcheck="false"`} {
		if !strings.Contains(num, want) {
			t.Errorf("numeric input missing %q: %s", want, num)
		}
	}

	// Fieldset is a real <fieldset>/<legend>; Disabled disables the whole group.
	fs := render(t, Fieldset(FieldsetProps{Legend: "Contact", Disabled: true}), text("body"))
	for _, want := range []string{"<fieldset", `class="dw-fieldset"`, "disabled", "<legend", "Contact", "body"} {
		if !strings.Contains(fs, want) {
			t.Errorf("fieldset missing %q: %s", want, fs)
		}
	}
}

func TestErrorViews(t *testing.T) {
	// ErrorPage is a complete document built on the Page shell.
	ep := render(t, ErrorPage(404, "Not found", "That page washed away."))
	for _, want := range []string{
		"<!doctype html>", // templ normalizes the doctype to lowercase
		"404",
		"Not found",
		"That page washed away.",
		"</html>",
	} {
		if !strings.Contains(ep, want) {
			t.Errorf("error page missing %q: %s", want, ep)
		}
	}

	// ErrorAlert keeps a stable id (so retries morph, not stack) and grades
	// severity into the toast classes.
	danger := render(t, ErrorAlert("err-save", true, "Save failed", "Try again"))
	for _, want := range []string{`id="err-save"`, "dw-toast-danger", `role="alert"`, "Save failed", "Try again"} {
		if !strings.Contains(danger, want) {
			t.Errorf("danger error alert missing %q: %s", want, danger)
		}
	}
	warn := render(t, ErrorAlert("err-soft", false, "Slow link", ""))
	if !strings.Contains(warn, "dw-toast-warn") || strings.Contains(warn, "dw-toast-danger") {
		t.Errorf("warn error alert graded wrong: %s", warn)
	}
}
