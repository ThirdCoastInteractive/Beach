// Package web is hush's HTTP layer: the app/server struct the handlers hang
// off, every handler (the wall, the feed, the signup-gate flows, the admin
// suite), and every .templ view. It depends on the domain packages (store,
// content, reputation, media, mail, analytics, authx) and wires them into
// routes; nothing in those packages depends back on web.
package web

import (
	"context"
	"sync"

	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/analytics"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/auth"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/mail"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/media"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/reputation"
	"github.com/ThirdCoastInteractive/the-beach/examples/hush/internal/store"

	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config is the env-driven knobs the web layer reads directly off itself: the
// scoring weight baked into Repute (EmailPoints), the absolute-link base
// (BaseURL), and the seat counter (the rest of the gate policy lives in
// reputation.Settings). main builds this from its loaded env config.
type Config struct {
	BaseURL     string
	EmailPoints float64
}

// App holds the wired dependencies the handlers close over. The wall and the
// whole gate — applications, accounts, roles, sessions, the live policy — are
// Postgres-backed; a database is required, there is no in-memory mode.
type App struct {
	cfg      Config
	store    *store.Store
	media    *media.Media
	rep      *reputation.Repute
	settings *reputation.Settings
	outbox   *mail.Outbox
	anl      *analytics.Analytics
	auth     *authx.Auth
	pool     *pgxpool.Pool
	release  bool

	// The invite graph (invitee → inviter), in-memory, hydrated from
	// hush_invites at boot — the chain echoes read it on every vote.
	chainMu sync.Mutex
	chains  map[string]string
}

// Deps is everything main constructs and hands the web layer. Keeping it one
// struct keeps main's wiring readable and the App fields unexported.
type Deps struct {
	Config    Config
	Store     *store.Store
	Media     *media.Media
	Rep       *reputation.Repute
	Settings  *reputation.Settings
	Outbox    *mail.Outbox
	Analytics *analytics.Analytics
	Auth      *authx.Auth
	Pool      *pgxpool.Pool
	Release   bool
}

// New assembles the App from its constructed dependencies.
func New(d Deps) *App {
	return &App{
		cfg:      d.Config,
		store:    d.Store,
		media:    d.Media,
		rep:      d.Rep,
		settings: d.Settings,
		outbox:   d.Outbox,
		anl:      d.Analytics,
		auth:     d.Auth,
		pool:     d.Pool,
		release:  d.Release,
	}
}

// live returns the current gate-and-moderation policy snapshot, lock-free.
func (a *App) live() reputation.Live { return a.settings.Live() }

// tierOf maps an author-track score to a capability tier against the live
// thresholds.
func (a *App) tierOf(score float64) int { return reputation.TierOf(score, a.live()) }

// acct summarizes the viewer: their handle (empty if not signed in), whether
// they may post, and whether they are an admin.
func (a *App) acct(c *beach.Ctx) (handle string, canPost, admin bool) {
	if p, ok := c.Principal(); ok && p != nil {
		return p.Username, p.Can("post"), p.Can("admin")
	}
	return "", false, false
}

// hydrateChains loads the invite graph (who invited whom) at boot so chain
// echoes keep working across restarts — the edges live in Postgres even though
// the scores are in-memory.
func (a *App) hydrateChains(ctx context.Context) error {
	rows, err := a.pool.Query(ctx, `SELECT inviter, used_by FROM hush_invites WHERE used_by IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var inviter, invitee string
		if err := rows.Scan(&inviter, &invitee); err != nil {
			return err
		}
		a.addChain(invitee, inviter)
	}
	return rows.Err()
}

// addChain records invitee → inviter.
func (a *App) addChain(invitee, inviter string) {
	a.chainMu.Lock()
	defer a.chainMu.Unlock()
	if a.chains == nil {
		a.chains = map[string]string{}
	}
	a.chains[invitee] = inviter
}

// inviterOf returns who invited handle ("" for root members).
func (a *App) inviterOf(handle string) string {
	a.chainMu.Lock()
	defer a.chainMu.Unlock()
	return a.chains[handle]
}
