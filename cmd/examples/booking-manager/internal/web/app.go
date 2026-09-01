// Package web is booking-manager's HTTP surface: the app DI container plus
// every page, action, and the login flow. Handlers and views are methods on
// the app struct so they share one set of wired dependencies without
// mass-exporting or import cycles; main.go constructs the app and registers
// its routes.
package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/auth"
	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/session"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/specimen"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/locks"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/notify"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// SessionCookie is the name of booking-manager's session cookie.
const SessionCookie = "booking_session"

// app holds the wired dependencies the handlers close over. Postgres is
// always present (the system of record); guest messaging and the smart lock
// are behind interfaces whose transports config picked at boot.
type app struct {
	store    *store.Store
	authn    *auth.Authenticator
	sessions *session.Store
	pool     *pgxpool.Pool
	guests   *notify.Notifier
	locks    locks.Provider
	release  bool
}

// New wires the booking-manager web app from its already-constructed
// dependencies. It is the DI container the handlers and views hang off;
// main.go builds the stores and transports and passes them in.
func New(st *store.Store, authn *auth.Authenticator, sessions *session.Store, pool *pgxpool.Pool, guests *notify.Notifier, lock locks.Provider, release bool) *app {
	return &app{
		store:    st,
		authn:    authn,
		sessions: sessions,
		pool:     pool,
		guests:   guests,
		locks:    lock,
		release:  release,
	}
}

// Routes registers every page, action, and the login flow. The landing page
// and the inquiry intake are public — guests use them; everything behind the
// desk is guarded per-permission, so the auth story is visible right here.
func (a *app) Routes(app *beach.App) {
	// Public: the guest-facing landing page and intake form.
	app.Page("/", a.homePage)
	app.Action("/inquire", a.createInquiry)
	app.Page("/login", a.loginPage)

	// The living driftwood + chart showcase, handy while restyling the app.
	app.Page("/specimen", func(c *beach.Ctx) (beach.View, error) {
		return beach.View{Page: specimen.Page()}, nil
	})

	// The desk: read pages for anyone signed in with bookings:read.
	read := []beach.Guard{app.Can("bookings:read")}
	app.Page("/dashboard", a.dashboardPage, read...)
	app.Page("/properties", a.propertiesPage, read...)
	app.Page("/properties/{id}", a.propertyPage, read...)
	app.Page("/inquiries", a.inquiriesPage, read...)
	app.Page("/bookings", a.bookingsPage, read...)
	app.Page("/inventory", a.inventoryPage, read...)

	// Property management.
	propWrite := []beach.Guard{app.Can("properties:write")}
	app.Action("/properties", a.createProperty, propWrite...)
	app.Action("/properties/{id}/codes", a.addKeyCode, propWrite...)
	app.Action("/codes/{id}/toggle", a.toggleKeyCode, propWrite...)

	// Guest pipeline: quoting, converting, and working bookings.
	bookWrite := []beach.Guard{app.Can("bookings:write")}
	app.Action("/inquiries/{id}/status", a.setInquiryStatus, bookWrite...)
	app.Action("/inquiries/{id}/book", a.bookInquiry, bookWrite...)
	app.Action("/bookings", a.createBooking, bookWrite...)
	app.Action("/bookings/{id}/status", a.setBookingStatus, bookWrite...)

	// Staffing: staff can see the board and punch the clock; managing the
	// pipeline and the schedule takes staffing:write.
	staffRead := []beach.Guard{app.Can("staffing:read")}
	staffWrite := []beach.Guard{app.Can("staffing:write")}
	app.Page("/hiring", a.hiringPage, staffRead...)
	app.Page("/staff", a.staffPage, staffRead...)
	app.Action("/applicants", a.createApplicant, staffWrite...)
	app.Action("/applicants/{id}/stage", a.setApplicantStage, staffWrite...)
	app.Action("/applicants/{id}/hire", a.hireApplicant, staffWrite...)
	app.Action("/staff", a.createStaff, staffWrite...)
	app.Action("/shifts", a.createShift, staffWrite...)
	app.Action("/staff/{id}/clock", a.clockToggle, staffRead...)

	// Inventory: staff keep the counts honest.
	invWrite := []beach.Guard{app.Can("inventory:write")}
	app.Action("/supplies", a.createSupply, invWrite...)
	app.Action("/supplies/{id}/adjust", a.adjustSupply, invWrite...)

	// Login: Raw POST so it can set the cookie and answer with a real 303.
	app.Raw(http.MethodPost, "/login", a.doLogin)
	app.Raw(http.MethodGet, "/logout", a.logout)
}

// PrincipalResolver turns a session user id into a rich auth.Principal by
// reading roles+permissions back from the RBAC tables. Wired as
// cfg.Principals so c.Principal()/Can() work in guards and handlers.
func (a *app) PrincipalResolver(ctx context.Context, userID int64) (*beach.Principal, error) {
	var username string
	if err := a.pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username); err != nil {
		return nil, err
	}
	roles, err := scanStrings(ctx, a.pool,
		`SELECT r.slug FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1 ORDER BY r.slug`, userID)
	if err != nil {
		return nil, err
	}
	perms, err := scanStrings(ctx, a.pool,
		`SELECT DISTINCT rp.permission FROM user_roles ur JOIN role_permissions rp ON rp.role_id = ur.role_id WHERE ur.user_id = $1 ORDER BY rp.permission`, userID)
	if err != nil {
		return nil, err
	}
	return &beach.Principal{UserID: userID, Username: username, Roles: roles, Permissions: perms}, nil
}

func scanStrings(ctx context.Context, pool *pgxpool.Pool, q string, args ...any) ([]string, error) {
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// principalCan reports whether the request principal holds permission. An
// anonymous (signed-out) request has no permissions, so write forms stay
// hidden until login.
func (a *app) principalCan(c *beach.Ctx, perm string) bool {
	p, _ := c.Principal()
	return p.Can(perm)
}

// pathID reads a numeric {id} path parameter.
func pathID(c *beach.Ctx) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return id
}

// logout revokes the session and clears the cookie.
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(SessionCookie); err == nil {
		_ = a.sessions.Revoke(r.Context(), ck.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// setSessionCookieW writes the session cookie on a raw response.
func (a *app) setSessionCookieW(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.release,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(14 * 24 * time.Hour),
	})
}
