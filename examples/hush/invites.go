// The other two ways in. The default gate is the application queue (apply.go);
// this file adds the modes you can switch on later:
//
//   - open registration: set HUSH_OPEN_SLOTS=N and the first N signups claim a
//     seat with no review. When the seats run out the queue resumes.
//   - invites: a tier-1+ member mints codes; an invite link creates an account
//     instantly. Every redemption records a chain edge (inviter → invitee),
//     and the invitee's 👍/👎 echo onto the inviter's Repute chain track — so
//     a member who invites shitheads watches their own allowance dry up.
package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/passwords"
)

// inviteAllowance is the total number of codes a member may have minted, given
// their tier and chain standing. A tier-1 member starts with cfg.InviteBase
// codes; every two points of chain standing buys another, every two lost burns
// one — a chain at −2·base zeroes it. Hard-capped at cfg.InviteMax so a
// beloved chain can't mint the whole site. Below tier 1 nobody invites.
func (a *app) inviteAllowance(tier int, chain float64) int {
	if tier < 1 {
		return 0
	}
	s := a.live()
	n := s.InviteBase + int(chain)/2
	if n < 0 {
		n = 0
	}
	if n > s.InviteMax {
		n = s.InviteMax
	}
	return n
}

// invite is one minted code as the profile panel lists it.
type invite struct {
	Code   string
	UsedBy string // empty until redeemed
}

// invitePanel is what the profile page shows a tier-1+ member about their own
// invites: the throttled allowance and the codes themselves. The chain score
// behind the allowance is never shown — like every score in hush.
type invitePanel struct {
	Show      bool
	Allowance int
	Remaining int
	Invites   []invite
}

var errHandleTaken = errors.New("handle taken")

// hydrateChains loads the invite graph (who invited whom) at boot so chain
// echoes keep working across restarts — the edges live in Postgres even though
// the scores are in-memory.
func (a *app) hydrateChains(ctx context.Context) error {
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
func (a *app) addChain(invitee, inviter string) {
	a.chainMu.Lock()
	defer a.chainMu.Unlock()
	if a.chains == nil {
		a.chains = map[string]string{}
	}
	a.chains[invitee] = inviter
}

// inviterOf returns who invited handle ("" for root members).
func (a *app) inviterOf(handle string) string {
	a.chainMu.Lock()
	defer a.chainMu.Unlock()
	return a.chains[handle]
}

// openLeft counts the unclaimed open-registration seats — 0 when the mode is
// off (the open-slots setting at 0) or the seats are spent.
func (a *app) openLeft(ctx context.Context) int {
	slots := a.live().OpenSlots
	if slots <= 0 {
		return 0
	}
	var used int
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM hush_signups WHERE via='open'`).Scan(&used)
	if left := slots - used; left > 0 {
		return left
	}
	return 0
}

// createMember creates an account — user, local credential, member role — for
// a fresh handle. Taken handles are rejected outright: registration must never
// bind a new credential to an existing user (the admin-queue path in apply.go
// keeps its own upsert semantics).
func (a *app) createMember(ctx context.Context, handle, passwordHash string) error {
	var exists bool
	_ = a.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)`, handle).Scan(&exists)
	if exists {
		return errHandleTaken
	}
	var uid int64
	if err := a.pool.QueryRow(ctx, `INSERT INTO users (username) VALUES ($1) RETURNING id`, handle).Scan(&uid); err != nil {
		return err
	}
	if _, err := a.pool.Exec(ctx, `INSERT INTO user_credentials_local (user_id, password_hash) VALUES ($1, $2)`, uid, passwordHash); err != nil {
		return err
	}
	var rid int64
	if err := a.pool.QueryRow(ctx, `SELECT id FROM roles WHERE slug = 'member'`).Scan(&rid); err == nil {
		_, _ = a.pool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, rid)
	}
	return nil
}

// signIn logs a fresh account in and lands it on the wall — registration
// flows shouldn't bounce a brand-new member to a login form.
func (a *app) signIn(w http.ResponseWriter, r *http.Request, handle, password string) {
	token, _, _, err := a.authn.Login(r.Context(), handle, password)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	a.setSessionCookieW(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// doRegister claims one open seat: the ledger insert is guarded by the cap in
// a single statement (no seat, no row), then the account is created. A taken
// handle releases the seat and bounces back with the error.
func (a *app) doRegister(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	handle := strings.TrimSpace(r.FormValue("handle"))
	password := r.FormValue("password")
	if handle == "" || password == "" {
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}
	if !agreed(r, "agree18") {
		http.Redirect(w, r, "/apply?err=agree", http.StatusSeeOther)
		return
	}
	ctx := r.Context()

	tag, err := a.pool.Exec(ctx,
		`INSERT INTO hush_signups (handle, via)
		 SELECT $1, 'open' WHERE (SELECT count(*) FROM hush_signups WHERE via = 'open') < $2`,
		handle, a.live().OpenSlots)
	if err != nil || tag.RowsAffected() == 0 {
		http.Redirect(w, r, "/apply", http.StatusSeeOther) // seats gone (or mode off)
		return
	}

	hash, err := passwords.Hash(password)
	if err == nil {
		err = a.createMember(ctx, handle, hash)
	}
	if err != nil {
		_, _ = a.pool.Exec(ctx, `DELETE FROM hush_signups WHERE handle = $1 AND via = 'open'`, handle)
		// tell the truth about which rule bit: a weak password is not a
		// taken handle
		http.Redirect(w, r, "/apply?err="+signupErrCode(err), http.StatusSeeOther)
		return
	}
	a.recordAgreement(ctx, handle, agreeTOS18, clientMeta(r))
	a.attachEmail(ctx, handle, r.FormValue("email"))
	a.track(r, "register", handle, 0)
	a.signIn(w, r, handle, password)
}

// signupErrCode maps an account-creation failure onto the form's ?err code.
func signupErrCode(err error) string {
	if errors.Is(err, passwords.ErrPasswordTooShort) {
		return "weak"
	}
	return "taken"
}

// signupErrMsg renders the form banner for a signup ?err code.
func signupErrMsg(code string) string {
	switch code {
	case "weak":
		return "password too short — 8 characters minimum."
	case "taken":
		return "that handle is taken — try another."
	case "agree":
		return "you must be 18 or older and accept the terms to contribute."
	}
	return ""
}

// invitePage renders the redemption form for a live code, or a notice for a
// spent or unknown one.
func (a *app) invitePage(c *beach.Ctx) (beach.View, error) {
	var usedBy *string
	err := a.pool.QueryRow(c.Context(), `SELECT used_by FROM hush_invites WHERE code = $1`, c.Param("code")).Scan(&usedBy)
	if err != nil {
		return beach.View{Page: noticePage("that invite doesn't exist.", ""), Status: http.StatusNotFound}, nil
	}
	if usedBy != nil {
		return beach.View{Page: noticePage("that invite was already used.", "ask your friend for another.")}, nil
	}
	return beach.View{Page: inviteFormPage(c.Param("code"), signupErrMsg(c.Query("err")))}, nil
}

// doRedeem turns a live invite into an account: claim the code atomically,
// create the member, record the chain edge, sign them in. A taken handle
// releases the claim so the code survives the retry.
func (a *app) doRedeem(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	code := r.PathValue("code")
	handle := strings.TrimSpace(r.FormValue("handle"))
	password := r.FormValue("password")
	if handle == "" || password == "" {
		http.Redirect(w, r, "/invite/"+code, http.StatusSeeOther)
		return
	}
	if !agreed(r, "agree18") {
		http.Redirect(w, r, "/invite/"+code+"?err=agree", http.StatusSeeOther)
		return
	}
	ctx := r.Context()

	var inviter string
	err := a.pool.QueryRow(ctx,
		`UPDATE hush_invites SET used_by = $1, used_at = now()
		 WHERE code = $2 AND used_by IS NULL RETURNING inviter`, handle, code).Scan(&inviter)
	if err != nil {
		http.Redirect(w, r, "/invite/"+code, http.StatusSeeOther) // spent or bogus
		return
	}

	hash, err := passwords.Hash(password)
	if err == nil {
		err = a.createMember(ctx, handle, hash)
	}
	if err != nil {
		_, _ = a.pool.Exec(ctx, `UPDATE hush_invites SET used_by = NULL, used_at = NULL WHERE code = $1`, code)
		http.Redirect(w, r, "/invite/"+code+"?err="+signupErrCode(err), http.StatusSeeOther)
		return
	}
	_, _ = a.pool.Exec(ctx, `INSERT INTO hush_signups (handle, via) VALUES ($1, 'invite')`, handle)
	a.addChain(handle, inviter)
	a.recordAgreement(ctx, handle, agreeTOS18, clientMeta(r))
	a.attachEmail(ctx, handle, r.FormValue("email"))
	a.track(r, "redeem", handle, 0)
	a.signIn(w, r, handle, password)
}

// doMint mints one invite code if the member's allowance has room, then 303s
// back to their profile, where the list lives. An optional email form value is
// the "mint & send" path: the fresh code goes straight out as an invite link.
func (a *app) doMint(w http.ResponseWriter, r *http.Request) {
	handle, canPost, _ := a.acct(&beach.Ctx{W: w, R: r})
	if handle == "" || !canPost {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	ctx := r.Context()
	tier := a.tierOf(a.rep.score(handle))
	allowance := a.inviteAllowance(tier, a.rep.chainScore(handle))
	var minted int
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM hush_invites WHERE inviter = $1`, handle).Scan(&minted)
	if minted < allowance {
		code := randHex()
		if _, err := a.pool.Exec(ctx, `INSERT INTO hush_invites (code, inviter) VALUES ($1, $2)`, code, handle); err == nil {
			if email := normalizeEmail(r.FormValue("email")); email != "" {
				a.enqueueMail(ctx, email, "invite", inviteEmail(a.cfg.BaseURL+"/invite/"+code))
			}
		}
	}
	http.Redirect(w, r, "/u/"+handle, http.StatusSeeOther)
}

// invitesFor assembles the profile panel for a member's own profile: chain
// standing, the allowance after the throttle, and every minted code.
func (a *app) invitesFor(ctx context.Context, handle string, tier int) invitePanel {
	if tier < 1 {
		return invitePanel{}
	}
	allowance := a.inviteAllowance(tier, a.rep.chainScore(handle))
	rows, err := a.pool.Query(ctx,
		`SELECT code, COALESCE(used_by, '') FROM hush_invites WHERE inviter = $1 ORDER BY created_at`, handle)
	if err != nil {
		return invitePanel{}
	}
	defer rows.Close()
	var invites []invite
	for rows.Next() {
		var inv invite
		if err := rows.Scan(&inv.Code, &inv.UsedBy); err != nil {
			return invitePanel{}
		}
		invites = append(invites, inv)
	}
	remaining := allowance - len(invites)
	if remaining < 0 {
		remaining = 0
	}
	return invitePanel{Show: true, Allowance: allowance, Remaining: remaining, Invites: invites}
}
