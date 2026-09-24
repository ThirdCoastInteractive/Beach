package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/analytics"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/pantry/internal/store"
)

// CRUD actions. These are the hand-written equivalents of what beach-apigen
// would emit from the @api POST annotations on the .sql files: bind the form
// params, run the write, and patch the @fragment. They are guarded by
// app.Can("pantry:write") at registration so the auth story is visible in the
// route table (see app.go).

// createItem handles POST /items: it appends a row and re-renders the item grid.
func (a *app) createItem(c *beach.Ctx) (beach.Patches, error) {
	// The form binds each field to a same-named signal, so Datastar posts them as
	// JSON — read them from the signals, not the form.
	// A number input binds to a numeric signal, so quantity arrives as a JSON
	// number; the text/select fields arrive as strings.
	in, err := beach.Bind[struct {
		Name       string  `json:"name"`
		Quantity   float64 `json:"quantity"`
		Unit       string  `json:"unit"`
		Category   string  `json:"category"`
		LocationID string  `json:"location_id"`
		ExpiresAt  string  `json:"expires_at"`
	}](c)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		// Nothing to add — just re-render the current grid and clear the form.
		items, err := a.store.ListItems(c.Context())
		if err != nil {
			return nil, err
		}
		return beach.Patches{{Fragment: a.itemGrid(items), Target: "item-grid"}}, nil
	}
	qty := in.Quantity
	if qty == 0 {
		qty = 1
	}
	locID, _ := strconv.ParseInt(in.LocationID, 10, 64)
	var exp *time.Time
	if in.ExpiresAt != "" {
		if t, err := time.Parse("2006-01-02", in.ExpiresAt); err == nil {
			exp = &t
		}
	}
	cat := firstNonEmpty(in.Category, "other")
	if err := a.store.AddItem(c.Context(), store.Item{
		Name:       in.Name,
		Quantity:   qty,
		Unit:       firstNonEmpty(in.Unit, "ea"),
		Category:   cat,
		LocationID: locID,
		ExpiresAt:  exp,
	}); err != nil {
		return nil, err
	}
	a.anl.Track(c.R, analytics.Event{Kind: "item_added", ItemName: in.Name, Category: cat, Quantity: qty})
	// Re-render the whole grid by its id (@fragment page.ItemGrid).
	items, err := a.store.ListItems(c.Context())
	if err != nil {
		return nil, err
	}
	return beach.Patches{{Fragment: a.itemGrid(items), Target: "item-grid"}}, nil
}

// createLocation handles POST /locations.
func (a *app) createLocation(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}](c)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) != "" {
		if err := a.store.AddLocation(c.Context(), in.Name, firstNonEmpty(in.Kind, "other")); err != nil {
			return nil, err
		}
		a.anl.Track(c.R, analytics.Event{Kind: "location_added", ItemName: in.Name})
	}
	locs, err := a.store.ListLocations(c.Context())
	if err != nil {
		return nil, err
	}
	return beach.Patches{{Fragment: a.locList(locs), Target: "loc-list"}}, nil
}

// --- login ---

// loginPage renders the sign-in form (views.templ loginView).
func (a *app) loginPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	return beach.View{Page: a.loginView(authed, c.Query("err"))}, nil
}

// doLogin handles POST /login. It is a Raw handler (not an ActionFunc) because
// login is a plain form POST (progressive enhancement): read the form fields,
// then redirect with a 303. No SSE, no inline script — the strict CSP forbids
// the SDK's script-based redirect, and a real navigation is what login wants.
func (a *app) doLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token, _, _, err := a.authn.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		dest := "/login?err=1"
		if err == auth.ErrAccountLocked {
			dest = "/login?err=locked"
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	a.setSessionCookieW(w, token)
	http.Redirect(w, r, "/items", http.StatusSeeOther)
}
