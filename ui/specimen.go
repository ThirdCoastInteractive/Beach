package ui

import (
	"context"
	"html"
	"io"
	"strconv"

	"github.com/ThirdCoastInteractive/the-beach/chart"
	"github.com/a-h/templ"
)

// SpecimenPage renders the toolkit's living specimen: a single (intentionally
// large, over-budget) page that exercises the whole catalog against a given kit.
// It is the visual contract for a kit — register a new theme, point a browser at
// /specimen, and every token, icon, component state, and chart is on one screen.
//
// It returns the page BODY as a templ.Component (not a full document): ui cannot
// import the driftwood shell without an import cycle, so the caller wraps the
// result in its kit's Page shell (which links /static/css/stellar.css). Sections
// render in order: a token sheet (palette swatches proving -on contrast), an icon
// grid, every Core component in representative states, then the 18-chart gallery.
//
// Everything cosmetic comes from Stellar CSS variables via inline style only
// (var(--neutral-1..12)/-on/-dim, var(--primary-*) … var(--named-info-*),
// var(--chart-qualitative-*)); no hex/OKLCH literal appears here.
func SpecimenPage(kit Kit) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		sections := []templ.Component{
			kit.Layout().PageHeading(PageHeadingProps{
				Title:    "driftwood specimen",
				Subtitle: "Every token, icon, component state, and chart on one page.",
			}),
			specimenTokenSheet(kit),
			specimenIconGrid(kit),
			specimenComponents(kit),
			specimenGallery(kit),
		}
		body := kit.Layout().Container(ContainerProps{}, sections...)
		return body.Render(ctx, w)
	})
}

// --- token sheet -------------------------------------------------------------

// paletteTones is the set of tones we swatch per palette: a light step, a mid
// step, and a strong step — enough to prove the ramp and its -on ink contrast.
var paletteTones = []int{2, 6, 9, 11}

// palettes is every Stellar palette the rules sanction for swatching.
var palettes = []struct{ name, label string }{
	{"neutral", "Neutral"},
	{"primary", "Primary"},
	{"secondary", "Secondary"},
	{"tertiary", "Tertiary"},
	{"error", "Error"},
	{"named-success", "Success"},
	{"named-warning", "Warning"},
	{"named-info", "Info"},
}

// specimenTokenSheet renders one row of swatches per palette. Each swatch fills
// with var(--<palette>-<n>) and writes its label in var(--<palette>-<n>-on), so
// the rendered contrast is itself the proof the -on ink pairs correctly.
func specimenTokenSheet(kit Kit) templ.Component {
	rows := make([]templ.Component, 0, len(palettes)+1)
	for _, p := range palettes {
		swatches := make([]templ.Component, 0, len(paletteTones))
		for _, n := range paletteTones {
			tone := p.name + "-" + strconv.Itoa(n)
			style := "background:var(--" + tone + ");color:var(--" + tone + "-on);" +
				"padding:.6rem .5rem;border-radius:var(--radius-2,.4rem);" +
				"flex:1 1 0;text-align:center;font-size:var(--font-size-0,.85rem);"
			swatches = append(swatches, rawEl("div", style, "Aa "+strconv.Itoa(n)))
		}
		label := rawEl("div",
			"min-width:7rem;align-self:center;font-weight:600;color:var(--neutral-12);",
			p.label)
		rowKids := []templ.Component{label, wrapEl(
			"div", "display:flex;gap:.4rem;flex:1 1 0;", swatches...)}
		rows = append(rows, wrapEl("div",
			"display:flex;gap:.75rem;margin-bottom:.5rem;align-items:stretch;", rowKids...))
	}
	inner := []templ.Component{
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Token sheet"}),
	}
	inner = append(inner, rows...)
	return kit.Layout().Card(CardProps{Surface: SurfacePanel}, inner...)
}

// --- icon grid ---------------------------------------------------------------

// specimenIcons is a representative set used when no icons have been recorded yet
// (UsedIcons is empty on a fresh process). It also seeds the recorder so the
// subsetter keeps these glyphs.
var specimenIcons = []string{
	"plus", "minus", "check", "x", "search", "gear", "trash", "pencil",
	"arrow-up", "arrow-down", "arrow-left", "arrow-right", "chevron-down",
	"home", "user", "users", "bell", "star", "heart", "calendar", "clock",
	"info", "warning", "error", "success", "download", "upload", "link",
	"menu", "filter", "refresh", "lock",
}

// specimenIconGrid renders the icons ui.UsedIcons has collected so far, falling
// back to a representative set so the grid is never empty on a cold page.
func specimenIconGrid(kit Kit) templ.Component {
	names := UsedIcons()
	if len(names) == 0 {
		names = specimenIcons
	}
	cells := make([]templ.Component, 0, len(names))
	for _, n := range names {
		cell := wrapEl("div",
			"display:flex;flex-direction:column;align-items:center;gap:.35rem;"+
				"padding:.6rem;border:1px solid var(--neutral-6);border-radius:var(--radius-2,.4rem);"+
				"background:var(--neutral-2);color:var(--neutral-2-on);"+
				"font-size:var(--font-size--1,.75rem);",
			I(n, IconSize(SizeLg), IconLabel(n)),
			rawEl("span", "color:var(--neutral-11);", n),
		)
		cells = append(cells, cell)
	}
	grid := wrapEl("div",
		"display:grid;grid-template-columns:repeat(auto-fill,minmax(6rem,1fr));gap:.5rem;",
		cells...)
	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Icons"}),
		grid,
	)
}

// --- components --------------------------------------------------------------

// specimenComponents renders every Core component the kit exposes in a handful of
// representative states, grouped by sub-interface.
func specimenComponents(kit Kit) templ.Component {
	blocks := []templ.Component{
		specimenButtons(kit),
		specimenForms(kit),
		specimenData(kit),
		specimenFeedback(kit),
		specimenNav(kit),
		specimenOverlays(kit),
		specimenMedia(kit),
		specimenMessaging(kit),
	}
	return wrapEl("div", "display:flex;flex-direction:column;gap:1rem;", blocks...)
}

// roleSet is the accent palette walked for buttons/badges/alerts so every Role
// appears at least once.
var roleSet = []struct {
	role  Role
	label string
}{
	{RoleAccent, "Accent"},
	{RoleNeutral, "Neutral"},
	{RoleDanger, "Danger"},
	{RoleGood, "Good"},
	{RoleWarn, "Warn"},
	{RoleInfo, "Info"},
	{RoleGhost, "Ghost"},
	{RoleQuiet, "Quiet"},
}

func specimenButtons(kit Kit) templ.Component {
	b := kit.Buttons()
	btns := make([]templ.Component, 0, len(roleSet)+4)
	for _, r := range roleSet {
		btns = append(btns, b.Button(ButtonProps{Label: r.label, Role: r.role, LeadIcon: "check"}))
	}
	btns = append(btns,
		b.Button(ButtonProps{Label: "Small", Role: RoleAccent, Size: SizeSm}),
		b.Button(ButtonProps{Label: "Large", Role: RoleAccent, Size: SizeLg}),
		b.Button(ButtonProps{Label: "Disabled", Role: RoleAccent, Disabled: true}),
		b.Button(ButtonProps{Label: "Loading", Role: RoleAccent, Loading: "$busy", TailIcon: "refresh"}),
	)

	group := b.ButtonGroup(ButtonGroupProps{Label: "View"},
		b.Button(ButtonProps{Label: "Day"}),
		b.Button(ButtonProps{Label: "Week"}),
		b.Button(ButtonProps{Label: "Month"}),
	)
	icons := wrap("display:flex;gap:.4rem;align-items:center;",
		b.IconButton(IconButtonProps{Icon: "pencil", Label: "Edit"}),
		b.IconButton(IconButtonProps{Icon: "trash", Label: "Delete", Role: RoleDanger}),
		b.IconButton(IconButtonProps{Icon: "search", Label: "Search", Role: RoleAccent}),
	)
	menu := b.MenuButton(MenuButtonProps{
		ID: "spec-menu", Label: "Actions", Icon: "chevron-down", Role: RoleAccent,
		Items: []MenuItem{
			{Label: "Rename", Icon: "pencil"},
			{Label: "Duplicate", Icon: "plus"},
			{Label: "Delete", Icon: "trash"},
		},
	})

	row := wrap("display:flex;flex-wrap:wrap;gap:.5rem;align-items:center;", btns...)
	row2 := wrapEl("div", "display:flex;flex-wrap:wrap;gap:1rem;align-items:center;margin-top:.75rem;",
		group, icons, menu)
	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Buttons"}), row, row2)
}

func specimenForms(kit Kit) templ.Component {
	f := kit.Forms()
	fields := []templ.Component{
		f.Field(FieldProps{Label: "Email", For: "spec-email", Help: "We never share it."},
			f.TextInput(TextInputProps{ID: "spec-email", Name: "email", Type: "email", Placeholder: "you@example.com"})),
		f.Field(FieldProps{Label: "Password", For: "spec-pw", Error: "Too short."},
			f.TextInput(TextInputProps{ID: "spec-pw", Name: "pw", Type: "password", Value: "abc", Error: true})),
		f.Field(FieldProps{Label: "Bio", For: "spec-bio"},
			f.Textarea(TextareaProps{ID: "spec-bio", Name: "bio", Rows: 3, Value: "Hello from the beach."})),
		f.Field(FieldProps{Label: "Plan", For: "spec-plan"},
			f.Select(SelectProps{ID: "spec-plan", Name: "plan", Placeholder: "Choose…",
				Options: []Option{
					{Value: "free", Label: "Free"},
					{Value: "pro", Label: "Pro", Selected: true},
					{Value: "team", Label: "Team"},
				}})),
		f.Field(FieldProps{Label: "Options"},
			f.Checkbox(CheckboxProps{Name: "tos", Label: "Accept terms", Checked: true}),
			f.RadioGroup(RadioGroupProps{Name: "tier", Inline: true, Options: []RadioOption{
				{Value: "a", Label: "Bronze", Checked: true},
				{Value: "b", Label: "Silver"},
				{Value: "c", Label: "Gold"},
			}}),
			f.Toggle(ToggleProps{Name: "notify", Label: "Email me", Description: "Weekly digest", Checked: true}),
		),
		f.InputGroup(InputGroupProps{
			Input:  TextInputProps{Name: "q", Placeholder: "Search…"},
			Button: ButtonProps{Label: "Go", Role: RoleAccent, LeadIcon: "search", Type: "submit"},
		}),
		f.FormError(FormErrorProps{Title: "Could not save", Messages: []string{
			"Email is required.", "Password is too short.",
		}}),
	}
	form := f.Form(FormProps{Post: "/specimen", TwoCol: false}, fields...)
	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Forms"}), form)
}

func specimenData(kit Kit) templ.Component {
	d := kit.Data()
	table := d.Table(TableProps{
		Columns: []Column{{Header: "Service"}, {Header: "Owner"}, {Header: "Draw (kW)"}},
		Rows: [][]string{
			{"api", "platform", "6.4"},
			{"web", "frontend", "4.1"},
			{"auth", "identity", "2.0"},
		},
	})
	emptyTable := d.Table(TableProps{
		Columns: []Column{{Header: "Service"}, {Header: "Owner"}},
		Empty:   "No services yet.",
	})
	stats := wrap("display:flex;flex-wrap:wrap;gap:1rem;",
		d.Stat(StatProps{Label: "Active users", Value: "12,402", Delta: Delta{Value: "4.2%", Dir: "up"}}),
		d.Stat(StatProps{Label: "Latency p95", Value: "182ms", Delta: Delta{Value: "1.1%", Dir: "down"}}),
		d.Stat(StatProps{Label: "Errors", Value: "0.03%", Delta: Delta{Value: "0", Dir: ""}}),
	)
	badges := wrap("display:flex;flex-wrap:wrap;gap:.4rem;align-items:center;",
		d.Badge(BadgeProps{Label: "Accent", Role: RoleAccent}),
		d.Badge(BadgeProps{Label: "Good", Role: RoleGood, Dot: true}),
		d.Badge(BadgeProps{Label: "Warn", Role: RoleWarn, Dot: true}),
		d.Badge(BadgeProps{Label: "Danger", Role: RoleDanger}),
		d.Badge(BadgeProps{Label: "Info", Role: RoleInfo}),
		d.Badge(BadgeProps{Label: "Neutral"}),
	)
	desc := d.DescList(DescListProps{TwoCol: true, Items: []DescItem{
		{Term: "Region", Value: "us-east-1"},
		{Term: "Redundancy", Value: "N+1"},
		{Term: "Status", Value: "Healthy"},
	}})
	stacked := d.StackedList(StackedListProps{Rows: []ListRow{
		{Title: "Ada Lovelace", Meta: "ada@beach.dev", Badge: &BadgeProps{Label: "Online", Role: RoleGood, Dot: true}},
		{Title: "Alan Turing", Meta: "alan@beach.dev", Badge: &BadgeProps{Label: "Away", Role: RoleWarn, Dot: true}},
	}})

	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Data"}),
		stats, badges,
		kit.Layout().Divider(DividerProps{Label: "Tables"}),
		table, emptyTable,
		kit.Layout().Divider(DividerProps{Label: "Lists & descriptions"}),
		desc, stacked,
	)
}

func specimenFeedback(kit Kit) templ.Component {
	fb := kit.Feedback()
	alerts := []templ.Component{
		fb.Alert(AlertProps{Role: RoleGood, Title: "Saved"}, txt("Your changes were saved.")),
		fb.Alert(AlertProps{Role: RoleWarn, Title: "Heads up"}, txt("Capacity is at 82%.")),
		fb.Alert(AlertProps{Role: RoleDanger, Title: "Failed", Dismissible: true}, txt("Could not reach the host.")),
		fb.Alert(AlertProps{Role: RoleInfo, Title: "FYI"}, txt("A new version is available.")),
	}
	toasts := wrap("display:flex;flex-wrap:wrap;gap:.5rem;",
		fb.Toast(ToastProps{Role: RoleGood, Title: "Done", Message: "Deploy finished."}),
		fb.Toast(ToastProps{Role: RoleDanger, Title: "Error", Message: "Build broke."}),
	)
	progress := wrap("display:flex;flex-direction:column;gap:.6rem;",
		fb.Progress(ProgressProps{Value: 65, Max: 100, Label: "Upload", Role: RoleAccent}),
		fb.Progress(ProgressProps{Max: 0, Label: "Working"}),
	)
	misc := wrap("display:flex;flex-wrap:wrap;gap:1.5rem;align-items:center;",
		fb.Spinner(SpinnerProps{Size: SizeLg}),
		fb.Skeleton(SkeletonProps{Width: "12rem", Height: "1rem"}),
		fb.EmptyState(EmptyStateProps{Icon: "search", Message: "No results found."}),
	)
	inner := []templ.Component{kit.Layout().SectionHeading(SectionHeadingProps{Label: "Feedback"})}
	inner = append(inner, alerts...)
	inner = append(inner, toasts, progress, misc)
	return kit.Layout().Card(CardProps{Surface: SurfacePanel}, inner...)
}

func specimenNav(kit Kit) templ.Component {
	n := kit.Nav()
	navbar := n.Navbar(NavbarProps{Brand: "the-beach", Items: []NavItem{
		{Label: "Home", Href: "#", Active: true, Icon: "home"},
		{Label: "Docs", Href: "#"},
		{Label: "Specimen", Href: "/specimen"},
	}})
	tabs := n.Tabs(TabsProps{Name: "spec-tabs", Tabs: []Tab{
		{ID: "t1", Label: "Overview", Active: true},
		{ID: "t2", Label: "Activity"},
		{ID: "t3", Label: "Settings"},
	}})
	crumbs := n.Breadcrumbs(BreadcrumbsProps{Items: []NavItem{
		{Label: "Home", Href: "#"},
		{Label: "Racks", Href: "#"},
		{Label: "A1", Active: true},
	}})
	page := n.Pagination(PaginationProps{Page: 2, Pages: 7, GetBase: "/specimen"})
	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Navigation"}),
		navbar, crumbs, tabs, page,
	)
}

func specimenOverlays(kit Kit) templ.Component {
	o := kit.Overlays()
	modal := o.Modal(ModalProps{ID: "spec-modal", Title: "Confirm delete"},
		txt("This cannot be undone."))
	drawer := o.Drawer(DrawerProps{ID: "spec-drawer", Title: "Filters", Side: "right"},
		txt("Drawer body."))
	popover := o.Popover(PopoverProps{ID: "spec-pop", Label: "More", Icon: "chevron-down"},
		txt("Popover panel content."))
	tooltip := o.Tooltip(TooltipProps{Text: "Helpful hint"},
		kit.Buttons().Button(ButtonProps{Label: "Hover me"}))
	note := rawEl("p", "color:var(--neutral-11);font-size:var(--font-size--1,.8rem);",
		"Modal and drawer render closed (native <dialog>); shown here for markup completeness.")
	row := wrap("display:flex;flex-wrap:wrap;gap:1rem;align-items:center;", popover, tooltip)
	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Overlays"}),
		row, note, modal, drawer,
	)
}

func specimenMedia(kit Kit) templ.Component {
	m := kit.Media()
	avatars := wrap("display:flex;gap:.5rem;align-items:center;",
		m.Avatar(AvatarProps{Initials: "AL", Alt: "Ada Lovelace", Size: SizeLg}),
		m.Avatar(AvatarProps{Initials: "AT", Alt: "Alan Turing", Status: "online"}),
		m.Avatar(AvatarProps{Initials: "GH", Alt: "Grace Hopper", Size: SizeSm}),
	)
	box := wrapEl("div", "max-width:18rem;",
		m.AspectBox(AspectBoxProps{Ratio: RatioWide},
			rawEl("div",
				"display:flex;align-items:center;justify-content:center;height:100%;"+
					"background:var(--neutral-3);color:var(--neutral-3-on);", "16:9 reserved space")))
	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Media"}),
		avatars, box,
	)
}

func specimenMessaging(kit Kit) templ.Component {
	mg := kit.Messaging()
	list := mg.MessageList(MessageListProps{Messages: []Message{
		{Author: "Ada", Body: "Is the cluster up?", At: "10:01"},
		{Author: "You", Body: "Yes, all green.", Own: true, At: "10:02"},
		{Body: "Ada joined the channel", System: true, At: "10:00"},
	}})
	composer := mg.Composer(ComposerProps{Name: "msg", Placeholder: "Message…", Post: "/specimen"})
	pills := wrap("display:flex;gap:.5rem;flex-wrap:wrap;",
		mg.PresencePill(PresencePillProps{State: "connected", Label: "Connected"}),
		mg.PresencePill(PresencePillProps{State: "reconnecting", Label: "Reconnecting"}),
		mg.PresencePill(PresencePillProps{State: "paired", Label: "Paired"}),
	)
	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Messaging"}),
		pills, list, composer,
	)
}

// --- chart gallery -----------------------------------------------------------

// specimenGallery renders chart.GallerySVGs() — every chart type with sample
// data — as a responsive grid of titled cards.
func specimenGallery(kit Kit) templ.Component {
	items := chart.GallerySVGs()
	cells := make([]templ.Component, 0, len(items))
	for _, it := range items {
		title := rawEl("h3",
			"margin:0 0 .4rem;font-size:var(--font-size-0,.95rem);color:var(--neutral-12);", it.Title)
		svg := rawHTML(string(it.SVG))
		cell := wrapEl("div",
			"padding:.75rem;border:1px solid var(--neutral-6);border-radius:var(--radius-2,.4rem);"+
				"background:var(--neutral-1);color:var(--neutral-1-on);overflow:hidden;",
			title, svg)
		cells = append(cells, cell)
	}
	grid := wrapEl("div",
		"display:grid;grid-template-columns:repeat(auto-fill,minmax(20rem,1fr));gap:1rem;",
		cells...)
	return kit.Layout().Card(CardProps{Surface: SurfacePanel},
		kit.Layout().SectionHeading(SectionHeadingProps{Label: "Chart gallery"}),
		grid,
	)
}

// --- local render helpers ----------------------------------------------------
//
// These mirror noop.go's el/txt but take an inline style string (the sanctioned
// way to reach a Stellar variable from this catalog file without a kit class).

// styledEl renders a tag with an inline style attribute and inner components.
func styledEl(tag, style string, inner ...templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		open := "<" + tag
		if style != "" {
			open += ` style="` + html.EscapeString(style) + `"`
		}
		open += ">"
		if _, err := io.WriteString(w, open); err != nil {
			return err
		}
		for _, c := range inner {
			if c == nil {
				continue
			}
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "</"+tag+">")
		return err
	})
}

// wrapEl is styledEl with component children (alias for readability).
func wrapEl(tag, style string, inner ...templ.Component) templ.Component {
	return styledEl(tag, style, inner...)
}

// rawEl renders a tag with inline style around a single escaped text body.
func rawEl(tag, style, text string) templ.Component {
	return styledEl(tag, style, txt(text))
}

// wrap is wrapEl with the tag fixed to <div> — the common case.
func wrap(style string, inner ...templ.Component) templ.Component {
	return wrapEl("div", style, inner...)
}

// rawHTML writes a trusted HTML string verbatim (chart SVG is already escaped/
// generated by the chart package).
func rawHTML(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}
