package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/auth"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// CRUD actions. These are the hand-written equivalents of what beach-apigen
// would emit from the @api POST annotations on the .sql files: bind the form
// params, run the write, and patch the @fragment. They are guarded by
// app.Can("pantry:write") at registration so the auth story is visible in the
// route table (see main.go).

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
		return beach.Patches{{Fragment: a.itemGrid(c, a.store.listItems()), Target: "item-grid"}}, nil
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
	a.store.addItem(Item{
		Name:       in.Name,
		Quantity:   qty,
		Unit:       firstNonEmpty(in.Unit, "ea"),
		Category:   firstNonEmpty(in.Category, "other"),
		LocationID: locID,
		ExpiresAt:  exp,
	})
	// Re-render the whole grid by its id (@fragment page.ItemGrid).
	return beach.Patches{{Fragment: a.itemGrid(c, a.store.listItems()), Target: "item-grid"}}, nil
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
		a.store.addLocation(in.Name, firstNonEmpty(in.Kind, "other"))
	}
	kit := c.Kit()
	locs := a.store.listLocations()
	rows := make([]ui.ListRow, 0, len(locs))
	for _, l := range locs {
		rows = append(rows, ui.ListRow{Title: l.Name, Meta: l.Kind})
	}
	frag := wrapID("loc-list", kit.Data().StackedList(ui.StackedListProps{Rows: rows}))
	return beach.Patches{{Fragment: frag, Target: "loc-list"}}, nil
}

// --- login ---

// loginPage renders the sign-in form. It works without a DB but disables the
// submit (auth needs Postgres), surfacing a banner instead.
func (a *app) loginPage(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	kit := c.Kit()

	body := []templ.Component{
		kit.Layout().PageHeading(ui.PageHeadingProps{Title: a.t(ctx, "login.heading")}),
	}
	if a.authn == nil {
		body = append(body, kit.Feedback().Alert(ui.AlertProps{Role: ui.RoleWarn, Title: a.t(ctx, "login.dboff")}))
	} else {
		// A plain HTML form: a normal POST the browser submits and follows the 303
		// redirect for — no JS, no Datastar, no CSP concerns.
		errBanner := ""
		switch c.Query("err") {
		case "1":
			errBanner = `<div class="dw-alert dw-alert-danger" role="alert" style="margin-block-end:1rem">Invalid username or password.</div>`
		case "locked":
			errBanner = `<div class="dw-alert dw-alert-danger" role="alert" style="margin-block-end:1rem">Account locked. Try again later.</div>`
		}
		formHTML := errBanner +
			`<form method="post" action="/login" class="dw-form">` +
			`<div class="dw-field"><label class="dw-label" for="username">` + templ.EscapeString(a.t(ctx, "login.username")) + `</label>` +
			`<input class="dw-input" type="text" id="username" name="username" autocomplete="username" required></div>` +
			`<div class="dw-field"><label class="dw-label" for="password">` + templ.EscapeString(a.t(ctx, "login.password")) + `</label>` +
			`<input class="dw-input" type="password" id="password" name="password" autocomplete="current-password" required></div>` +
			`<button class="dw-btn dw-btn-accent" type="submit">` + templ.EscapeString(a.t(ctx, "login.submit")) + `</button></form>`
		body = append(body, kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel}, templ.Raw(formHTML)))
	}

	container := kit.Layout().Container(ui.ContainerProps{}, body...)
	return beach.View{Page: a.shell(c, a.t(ctx, "login.heading"), "", container)}, nil
}

// doLogin handles POST /login. It is a Raw handler (not an ActionFunc) because
// success must both set the session cookie on the response and tell Datastar to
// navigate — datastar.SSE.Redirect — which the typed ActionFunc shape doesn't
// expose. The form is a Datastar @post, so the response is an SSE stream either
// way. Requires the authenticator (DB).
func (a *app) doLogin(w http.ResponseWriter, r *http.Request) {
	if a.authn == nil {
		http.NotFound(w, r)
		return
	}
	// Login is a plain form POST (progressive enhancement): read the form fields,
	// then redirect with a 303. No SSE, no inline script — the strict CSP forbids
	// the SDK's script-based redirect, and a real navigation is what login wants.
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
