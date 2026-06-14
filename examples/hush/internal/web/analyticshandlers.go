package web

import (
	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/a-h/templ"
)

// analyticsPage is the admin dashboard: a grid of deferred widgets, each a
// placeholder that @gets its fragment from one of the widget PageFuncs below
// (the pantry dashboard pattern).
func (a *App) analyticsPage(c *beach.Ctx) (beach.View, error) {
	handle, _, admin := a.acct(c)
	return beach.View{Page: analyticsView(handle, admin, a.tierOf(a.rep.Score(handle)))}, nil
}

// anlWidget adapts a fragment-builder into a deferred-widget PageFunc: the
// fragment carries the placeholder's id so the patch morphs cleanly, and a
// direct navigation still renders the bare fragment instead of a 404. build
// runs per request — these are live queries, not cached fragments.
func anlWidget(id string, build func(c *beach.Ctx) (templ.Component, error)) beach.PageFunc {
	return func(c *beach.Ctx) (beach.View, error) {
		f, err := build(c)
		if err != nil {
			return beach.View{}, err
		}
		return beach.View{Page: f, Fragment: f, Target: id}, nil
	}
}

// The six widget builders delegate the queries to the analytics package and
// (for the two tables) render through the views in analyticsviews.templ.

func (a *App) anlEvents(c *beach.Ctx) (templ.Component, error) {
	return a.anl.Events(c.Context())
}

func (a *App) anlPosts(c *beach.Ctx) (templ.Component, error) {
	return a.anl.Posts(c.Context())
}

func (a *App) anlVotes(c *beach.Ctx) (templ.Component, error) {
	return a.anl.Votes(c.Context())
}

func (a *App) anlCountries(c *beach.Ctx) (templ.Component, error) {
	return a.anl.Countries(c.Context())
}

func (a *App) anlIPs(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.anl.IPs(c.Context())
	if err != nil {
		return nil, err
	}
	return ipsTable(rows), nil
}

func (a *App) anlRecent(c *beach.Ctx) (templ.Component, error) {
	rows, err := a.anl.Recent(c.Context())
	if err != nil {
		return nil, err
	}
	return recentTable(rows), nil
}
