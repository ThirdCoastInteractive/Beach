package main

import (
	"context"
	"io"
	"strconv"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
	"github.com/a-h/templ"
)

// shell wraps a page body in the driftwood document plus the app sidebar/topbar.
// active is the nav slug to highlight.
func (a *app) shell(c *beach.Ctx, title, active string, body ...templ.Component) templ.Component {
	ctx := c.Context()
	kit := c.Kit()

	_, authed := c.User()
	topItems := []ui.NavItem{}
	if authed {
		topItems = append(topItems, ui.NavItem{Label: "Log out", Href: "/logout"})
	} else {
		topItems = append(topItems, ui.NavItem{Label: a.t(ctx, "nav.login"), Href: "/login"})
	}
	topbar := kit.Nav().Navbar(ui.NavbarProps{
		Brand: a.t(ctx, "app.title"),
		Items: topItems,
	})

	sidebar := kit.Nav().SidebarNav(ui.SidebarNavProps{
		Sections: []ui.SidebarSection{{
			Items: []ui.NavItem{
				{Label: a.t(ctx, "nav.dashboard"), Href: "/", Icon: "chart", Active: active == "dash"},
				{Label: a.t(ctx, "nav.items"), Href: "/items", Icon: "box", Active: active == "items"},
				{Label: a.t(ctx, "nav.locations"), Href: "/locations", Icon: "pin", Active: active == "loc"},
				{Label: a.t(ctx, "nav.lists"), Href: "/lists", Icon: "list", Active: active == "lists"},
			},
		}},
	})

	page := kit.Layout().AppShell(ui.AppShellProps{Topbar: topbar, Sidebar: sidebar}, body...)
	return driftwood.Page(driftwood.PageProps{
		Title:       title + " — " + a.t(ctx, "app.title"),
		Description: a.t(ctx, "app.tagline"),
	}, page)
}

// dashboardPage is the home route. It renders without a DB: four chart widgets
// behind ui.Defer, each fed by sample data through its own widget PageFunc.
func (a *app) dashboardPage(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	kit := c.Kit()

	heading := kit.Layout().PageHeading(ui.PageHeadingProps{
		Title:    a.t(ctx, "dash.heading"),
		Subtitle: a.t(ctx, "dash.subtitle"),
	})

	widget := func(headingKey, id, get, height string) templ.Component {
		return kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: a.t(ctx, headingKey)},
			ui.Defer(ui.DeferProps{ID: id, Get: get, Height: height}),
		)
	}

	grid := kit.Layout().Grid(ui.GridProps{Cols: 2},
		widget("dash.spend", "w-spend", "/widgets/spend", "240px"),
		widget("dash.category", "w-category", "/widgets/category", "240px"),
		widget("dash.expiry", "w-expiry", "/widgets/expiry", "180px"),
		widget("dash.waste", "w-waste", "/widgets/waste", "220px"),
	)

	note := kit.Feedback().Alert(ui.AlertProps{Role: ui.RoleInfo, Title: a.t(ctx, "dash.sample")})

	body := kit.Layout().Container(ui.ContainerProps{}, heading, grid, note)
	return beach.View{Page: a.shell(c, a.t(ctx, "dash.heading"), "dash", body)}, nil
}

// widget builds a deferred chart-widget PageFunc. svg is computed per request;
// the fragment re-uses the placeholder's id so the patch morphs cleanly.
func widgetPage(id string, svg func(c *beach.Ctx) string) beach.PageFunc {
	return func(c *beach.Ctx) (beach.View, error) {
		frag := rawDiv(id, svg(c))
		// Deferred widgets are only ever fetched by Datastar (data-on-intersect),
		// but a direct navigation should still render the bare SVG rather than 404.
		return beach.View{Page: frag, Fragment: frag, Target: id}, nil
	}
}

// rawDiv wraps inner HTML in a div carrying id, as a templ component.
func rawDiv(id, inner string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<div id="`+id+`" class="pantry-widget">`+inner+`</div>`)
		return err
	})
}

// itemsPage lists inventory as a responsive card grid with photos (ui image +
// aspect-ratio: no layout shift). It renders from the in-memory store, so it
// works without a database.
func (a *app) itemsPage(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	kit := c.Kit()
	items := a.store.listItems()

	heading := kit.Layout().PageHeading(ui.PageHeadingProps{
		Title:    a.t(ctx, "items.heading"),
		Subtitle: a.t(ctx, "items.subtitle", len(items)),
	})

	var body templ.Component
	if len(items) == 0 {
		body = kit.Feedback().EmptyState(ui.EmptyStateProps{Icon: "box", Message: a.t(ctx, "items.empty")})
	} else {
		body = a.itemGrid(c, items)
	}

	canWrite := a.principalCan(c, "pantry:write")
	var form templ.Component = templ.NopComponent
	if canWrite {
		form = a.addItemForm(c)
	}

	container := kit.Layout().Container(ui.ContainerProps{}, heading, form, body)
	return beach.View{Page: a.shell(c, a.t(ctx, "items.heading"), "items", container)}, nil
}

// itemGrid renders the item cards (id="item-grid" so a create-action patch can
// append/replace it).
func (a *app) itemGrid(c *beach.Ctx, items []Item) templ.Component {
	kit := c.Kit()
	ctx := c.Context()
	cells := make([]templ.Component, 0, len(items))
	for _, it := range items {
		photo := kit.Media().Image(ui.ImageProps{
			Src:   firstNonEmpty(it.PhotoURL, "/static/img/placeholder.svg"),
			Alt:   it.Name,
			Ratio: ui.RatioPhoto,
			Fit:   ui.FitCover,
		})
		meta := strconvF(it.Quantity) + " " + it.Unit
		if it.Location != "" {
			meta += " · " + it.Location
		}
		var badge templ.Component = templ.NopComponent
		if expiringSoon(it) {
			badge = kit.Data().Badge(ui.BadgeProps{Label: a.t(ctx, "items.expires.soon"), Role: ui.RoleWarn, Dot: true})
		}
		card := kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel},
			photo,
			kit.Layout().CardHeading(ui.CardHeadingProps{Title: it.Name, Meta: meta, Action: badge}),
		)
		cells = append(cells, card)
	}
	grid := kit.Data().GridList(ui.GridListProps{Cols: 3}, cells...)
	return wrapID("item-grid", grid)
}

// addItemForm is the create-item form (hypermedia @post). Only rendered for
// pantry:write principals.
func (a *app) addItemForm(c *beach.Ctx) templ.Component {
	kit := c.Kit()
	ctx := c.Context()

	locOpts := []ui.Option{}
	for _, l := range a.store.listLocations() {
		locOpts = append(locOpts, ui.Option{Value: strconv.FormatInt(l.ID, 10), Label: l.Name})
	}

	form := kit.Forms().Form(ui.FormProps{Post: "/items", TwoCol: true},
		kit.Forms().Field(ui.FieldProps{Label: a.t(ctx, "items.field.name"), For: "name"},
			kit.Forms().TextInput(ui.TextInputProps{ID: "name", Name: "name", Placeholder: "Bananas"})),
		kit.Forms().Field(ui.FieldProps{Label: a.t(ctx, "items.field.qty"), For: "quantity"},
			kit.Forms().TextInput(ui.TextInputProps{ID: "quantity", Name: "quantity", Type: "number", Value: "1"})),
		kit.Forms().Field(ui.FieldProps{Label: a.t(ctx, "items.field.location"), For: "location_id"},
			kit.Forms().Select(ui.SelectProps{ID: "location_id", Name: "location_id", Options: locOpts})),
		kit.Forms().Field(ui.FieldProps{Label: a.t(ctx, "items.field.expiry"), For: "expires_at"},
			kit.Forms().TextInput(ui.TextInputProps{ID: "expires_at", Name: "expires_at", Type: "date"})),
		kit.Buttons().Button(ui.ButtonProps{Label: a.t(ctx, "items.add"), Role: ui.RoleAccent, Type: "submit", LeadIcon: "plus"}),
	)
	return kit.Layout().Card(ui.CardProps{Surface: ui.SurfaceWell, Heading: a.t(ctx, "items.add")}, form)
}

// locationsPage lists storage locations.
func (a *app) locationsPage(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	kit := c.Kit()
	locs := a.store.listLocations()

	heading := kit.Layout().PageHeading(ui.PageHeadingProps{Title: a.t(ctx, "loc.heading")})

	var list templ.Component
	if len(locs) == 0 {
		list = kit.Feedback().EmptyState(ui.EmptyStateProps{Icon: "pin", Message: a.t(ctx, "loc.empty")})
	} else {
		rows := make([]ui.ListRow, 0, len(locs))
		for _, l := range locs {
			rows = append(rows, ui.ListRow{Title: l.Name, Meta: l.Kind})
		}
		list = kit.Data().StackedList(ui.StackedListProps{Rows: rows})
	}

	var form templ.Component = templ.NopComponent
	if a.principalCan(c, "pantry:write") {
		form = kit.Forms().Form(ui.FormProps{Post: "/locations"},
			kit.Forms().Field(ui.FieldProps{Label: a.t(ctx, "loc.heading"), For: "name"},
				kit.Forms().TextInput(ui.TextInputProps{ID: "name", Name: "name", Placeholder: "Garage shelf"})),
			kit.Buttons().Button(ui.ButtonProps{Label: a.t(ctx, "items.add"), Role: ui.RoleAccent, Type: "submit"}),
		)
	}

	body := kit.Layout().Container(ui.ContainerProps{}, heading, form, wrapID("loc-list", list))
	return beach.View{Page: a.shell(c, a.t(ctx, "loc.heading"), "loc", body)}, nil
}

// listsPage renders the shopping lists and their lines.
func (a *app) listsPage(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	kit := c.Kit()
	lists := a.store.listLists()

	heading := kit.Layout().PageHeading(ui.PageHeadingProps{Title: a.t(ctx, "lists.heading")})

	var body templ.Component
	if len(lists) == 0 {
		body = kit.Feedback().EmptyState(ui.EmptyStateProps{Icon: "list", Message: a.t(ctx, "lists.empty")})
	} else {
		cards := make([]templ.Component, 0, len(lists))
		for _, l := range lists {
			rows := make([]ui.ListRow, 0, len(l.Items))
			for _, li := range l.Items {
				badge := &ui.BadgeProps{Label: "to buy", Role: ui.RoleQuiet}
				if li.Checked {
					badge = &ui.BadgeProps{Label: "in cart", Role: ui.RoleGood}
				}
				rows = append(rows, ui.ListRow{Title: li.Name, Meta: strconvF(li.Quantity), Badge: badge})
			}
			cards = append(cards, kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: l.Name},
				kit.Data().StackedList(ui.StackedListProps{Rows: rows})))
		}
		body = kit.Layout().Grid(ui.GridProps{Cols: 2}, cards...)
	}

	container := kit.Layout().Container(ui.ContainerProps{}, heading, body)
	return beach.View{Page: a.shell(c, a.t(ctx, "lists.heading"), "lists", container)}, nil
}

// --- small helpers ---

func wrapID(id string, inner templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div id="`+id+`">`); err != nil {
			return err
		}
		if inner != nil {
			if err := inner.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func strconvF(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// expiringSoon reports whether an item expires within a week.
func expiringSoon(it Item) bool {
	if it.ExpiresAt == nil {
		return false
	}
	return it.ExpiresAt.Before(time.Now().AddDate(0, 0, 7))
}

// principalCan reports whether the request principal holds permission. Without a
// configured PrincipalResolver (no DB) every request is anonymous and this is
// false, so write forms are hidden — read-only degradation.
func (a *app) principalCan(c *beach.Ctx, perm string) bool {
	p, _ := c.Principal()
	return p.Can(perm)
}
