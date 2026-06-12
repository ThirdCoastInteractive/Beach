package main

import (
	"strconv"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
)

// Page handlers. Markup lives in views.templ; these shape the data and pick
// the component. Each returns a full document built by (*app).shellView.

// dashboardPage is the home route. It renders without a DB: four chart
// widgets behind ui.Defer, each fed by sample data through its own widget
// PageFunc (see dashboard.go).
func (a *app) dashboardPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	return beach.View{Page: a.dashboardView(authed)}, nil
}

// itemsPage lists inventory from the in-memory store, so it works without a
// database.
func (a *app) itemsPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	// Database-free there is no principal, so the single local user can manage
	// their own pantry; with auth on, the pantry:write permission gates the form.
	canWrite := a.authn == nil || a.principalCan(c, "pantry:write")
	return beach.View{Page: a.itemsView(authed, canWrite, a.store.listItems(), a.store.listLocations())}, nil
}

// locationsPage lists storage locations.
func (a *app) locationsPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	return beach.View{Page: a.locationsView(authed, a.principalCan(c, "pantry:write"), a.store.listLocations())}, nil
}

// listsPage renders the shopping lists and their lines.
func (a *app) listsPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	return beach.View{Page: a.listsView(authed, a.store.listLists())}, nil
}

// --- view-data helpers (called from views.templ) ---

// itemMeta is an item card's meta line: quantity, unit, and location.
func itemMeta(it Item) string {
	meta := strconvF(it.Quantity) + " " + it.Unit
	if it.Location != "" {
		meta += " · " + it.Location
	}
	return meta
}

// locOptions shapes the storage locations into select options.
func locOptions(locs []Location) []driftwood.Option {
	opts := make([]driftwood.Option, 0, len(locs))
	for _, l := range locs {
		opts = append(opts, driftwood.Option{Value: strconv.FormatInt(l.ID, 10), Label: l.Name})
	}
	return opts
}

// locRows shapes the storage locations into stacked-list rows.
func locRows(locs []Location) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(locs))
	for _, l := range locs {
		rows = append(rows, driftwood.ListRow{Title: l.Name, Meta: l.Kind})
	}
	return rows
}

// listRows shapes one shopping list's lines, badged in-cart or to-buy.
func listRows(l ShoppingList) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(l.Items))
	for _, li := range l.Items {
		badge := &driftwood.BadgeProps{Label: "to buy", Role: driftwood.RoleQuiet}
		if li.Checked {
			badge = &driftwood.BadgeProps{Label: "in cart", Role: driftwood.RoleGood}
		}
		rows = append(rows, driftwood.ListRow{Title: li.Name, Meta: strconvF(li.Quantity), Badge: badge})
	}
	return rows
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
