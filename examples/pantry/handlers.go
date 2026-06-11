package main

import (
	"net/http"
	"strconv"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/auth"
	"github.com/ThirdCoastInteractive/the-beach/datastar"
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
	qty, _ := strconv.ParseFloat(c.FormValue("quantity"), 64)
	if qty == 0 {
		qty = 1
	}
	locID, _ := strconv.ParseInt(c.FormValue("location_id"), 10, 64)
	var exp *time.Time
	if v := c.FormValue("expires_at"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			exp = &t
		}
	}
	a.store.addItem(Item{
		Name:       c.FormValue("name"),
		Quantity:   qty,
		Unit:       firstNonEmpty(c.FormValue("unit"), "ea"),
		Category:   firstNonEmpty(c.FormValue("category"), "other"),
		LocationID: locID,
		ExpiresAt:  exp,
	})
	// Re-render the whole grid by its id (@fragment page.ItemGrid).
	return beach.Patches{{Fragment: a.itemGrid(c, a.store.listItems()), Target: "item-grid"}}, nil
}

// createLocation handles POST /locations.
func (a *app) createLocation(c *beach.Ctx) (beach.Patches, error) {
	a.store.addLocation(c.FormValue("name"), firstNonEmpty(c.FormValue("kind"), "other"))
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
		form := kit.Forms().Form(ui.FormProps{Post: "/login"},
			kit.Forms().Field(ui.FieldProps{Label: a.t(ctx, "login.username"), For: "username"},
				kit.Forms().TextInput(ui.TextInputProps{ID: "username", Name: "username"})),
			kit.Forms().Field(ui.FieldProps{Label: a.t(ctx, "login.password"), For: "password"},
				kit.Forms().TextInput(ui.TextInputProps{ID: "password", Name: "password", Type: "password"})),
			kit.Buttons().Button(ui.ButtonProps{Label: a.t(ctx, "login.submit"), Role: ui.RoleAccent, Type: "submit"}),
		)
		body = append(body, kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel}, form), wrapID("login-err", templ.NopComponent))
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
	username, password := r.FormValue("username"), r.FormValue("password")
	token, _, _, err := a.authn.Login(r.Context(), username, password)
	sse := datastar.NewSSE(w, r)
	if err != nil {
		msg := "Invalid username or password."
		if err == auth.ErrAccountLocked {
			msg = "Account locked. Try again later."
		}
		// Render the error alert into the #login-err slot.
		_ = sse.PatchElements(`<div id="login-err"><div class="dw-alert dw-alert-danger" role="alert">` +
			templ.EscapeString(msg) + `</div></div>`)
		return
	}
	a.setSessionCookieW(w, token)
	_ = sse.Redirect("/")
}
