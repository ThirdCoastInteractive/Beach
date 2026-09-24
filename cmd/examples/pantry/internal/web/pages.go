package web

import (
	"strconv"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/analytics"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/store"
)

// Page handlers. Markup lives in views.templ; these shape the data and pick
// the component. Each returns a full document built by (*app).shellView.

// dashboardPage is the home route: four deferred chart widgets behind ui.Defer,
// each running its own query (CH activity line + Postgres inventory widgets).
func (a *app) dashboardPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	return beach.View{Page: a.dashboardView(authed)}, nil
}

// itemsPage lists inventory from Postgres. Viewing an item also lands an
// item_viewed event in the firehose, so the activity line has read traffic.
func (a *app) itemsPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	canWrite := a.principalCan(c, "pantry:write")
	items, err := a.store.ListItems(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	locs, err := a.store.ListLocations(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	a.anl.Track(c.R, analytics.Event{Kind: "item_viewed"})
	return beach.View{Page: a.itemsView(authed, canWrite, items, locs)}, nil
}

// locationsPage lists storage locations.
func (a *app) locationsPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	locs, err := a.store.ListLocations(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.locationsView(authed, a.principalCan(c, "pantry:write"), locs)}, nil
}

// listsPage renders the shopping lists and their lines.
func (a *app) listsPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	lists, err := a.store.ListLists(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.listsView(authed, lists)}, nil
}

// --- view-data helpers (called from views.templ) ---

// itemMeta is an item card's meta line: quantity, unit, and location.
func itemMeta(it store.Item) string {
	meta := strconvF(it.Quantity) + " " + it.Unit
	if it.Location != "" {
		meta += " · " + it.Location
	}
	return meta
}

// locOptions shapes the storage locations into select options.
func locOptions(locs []store.Location) []driftwood.Option {
	opts := make([]driftwood.Option, 0, len(locs))
	for _, l := range locs {
		opts = append(opts, driftwood.Option{Value: strconv.FormatInt(l.ID, 10), Label: l.Name})
	}
	return opts
}

// locRows shapes the storage locations into stacked-list rows.
func locRows(locs []store.Location) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(locs))
	for _, l := range locs {
		rows = append(rows, driftwood.ListRow{Title: l.Name, Meta: l.Kind})
	}
	return rows
}

// listRows shapes one shopping list's lines, badged in-cart or to-buy.
func listRows(l store.ShoppingList) []driftwood.ListRow {
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
func expiringSoon(it store.Item) bool {
	if it.ExpiresAt == nil {
		return false
	}
	return it.ExpiresAt.Before(time.Now().AddDate(0, 0, 7))
}

// principalCan reports whether the request principal holds permission. An
// anonymous (signed-out) request has no permissions, so write forms stay hidden
// until login.
func (a *app) principalCan(c *beach.Ctx, perm string) bool {
	p, _ := c.Principal()
	return p.Can(perm)
}
