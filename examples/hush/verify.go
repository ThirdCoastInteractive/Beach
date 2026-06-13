// The member email lifecycle: an optional address attached at signup or from
// the profile, verified by a tokened link, worth EmailPoints of author
// standing once confirmed. Addresses are never shown publicly — even the
// masked form only ever renders on its owner's profile. The token travels in
// the email alone; the table stores its sha256 (a leaked dump verifies nobody).
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/mail"
	"strings"

	beach "github.com/ThirdCoastInteractive/the-beach"
)

// emailSection is what the profile shows its owner about their email: the
// masked address and whether it's verified, or nothing on file (the add form
// renders). Show is false on other people's profiles.
type emailSection struct {
	Show     bool
	Masked   string // "" when no address is on file
	Verified bool
}

// normalizeEmail validates a submitted address and returns it bare, "" when it
// doesn't parse. Email is optional everywhere in hush, so a bad address is
// quietly dropped — never an error the user has to see.
func normalizeEmail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return ""
	}
	return addr.Address
}

// maskEmail hides most of an address for the profile's quiet email line:
// m***@example.com. Even the owner sees only enough to recognize it.
func maskEmail(s string) string {
	at := strings.IndexByte(s, '@')
	if at <= 0 {
		return "***"
	}
	return s[:1] + "***" + s[at:]
}

// newVerifyToken returns 32 random bytes as hex. The cleartext only ever
// lives inside the verification link.
func newVerifyToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// verifyLink builds the absolute verification URL a token rides in.
func (a *app) verifyLink(token string) string {
	return a.cfg.BaseURL + "/email/verify/" + token
}

// userIDByHandle resolves a handle to its users row.
func (a *app) userIDByHandle(ctx context.Context, handle string) (int64, bool) {
	var uid int64
	err := a.pool.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, handle).Scan(&uid)
	return uid, err == nil
}

// setMemberEmail upserts a member's address as unverified under a fresh token
// and returns the cleartext token for the verification link. Changing an
// address always restarts verification — the old confirmation doesn't carry.
func (a *app) setMemberEmail(ctx context.Context, userID int64, email string) (string, error) {
	token := newVerifyToken()
	sum := sha256.Sum256([]byte(token))
	_, err := a.pool.Exec(ctx,
		`INSERT INTO hush_member_emails (user_id, email, verify_token_hash, verified_at, updated_at)
		 VALUES ($1, $2, $3, NULL, now())
		 ON CONFLICT (user_id) DO UPDATE
		 SET email = EXCLUDED.email, verify_token_hash = EXCLUDED.verify_token_hash,
		     verified_at = NULL, updated_at = now()`, userID, email, sum[:])
	return token, err
}

// attachEmail stores an optional address for a member (unverified) and queues
// the verification email — the one call registration flows and the profile's
// set form share. Empty or unparseable input is a quiet no-op.
func (a *app) attachEmail(ctx context.Context, handle, email string) {
	email = normalizeEmail(email)
	if email == "" {
		return
	}
	uid, ok := a.userIDByHandle(ctx, handle)
	if !ok {
		return
	}
	token, err := a.setMemberEmail(ctx, uid, email)
	if err != nil {
		return
	}
	a.enqueueMail(ctx, email, "verify", verifyEmail(a.verifyLink(token)))
}

// emailFor assembles the profile's email section for its owner.
func (a *app) emailFor(ctx context.Context, handle string) emailSection {
	es := emailSection{Show: true}
	var email string
	var verified bool
	if err := a.pool.QueryRow(ctx,
		`SELECT e.email, e.verified_at IS NOT NULL FROM hush_member_emails e
		 JOIN users u ON u.id = e.user_id WHERE u.username = $1`, handle).Scan(&email, &verified); err == nil {
		es.Masked = maskEmail(email)
		es.Verified = verified
	}
	return es
}

// verifyEmailPage is the link in the mail: GET /email/verify/{token}. The
// UPDATE matches the token hash against unverified rows only, so a link works
// exactly once — that's also what keeps the live "email-verified" emit from
// double-counting (boot replay re-emits from verified_at rows instead).
func (a *app) verifyEmailPage(c *beach.Ctx) (beach.View, error) {
	sum := sha256.Sum256([]byte(c.Param("token")))
	var handle string
	err := a.pool.QueryRow(c.Context(),
		`UPDATE hush_member_emails e
		 SET verified_at = now(), verify_token_hash = NULL, updated_at = now()
		 FROM users u
		 WHERE u.id = e.user_id AND e.verify_token_hash = $1 AND e.verified_at IS NULL
		 RETURNING u.username`, sum[:]).Scan(&handle)
	if err != nil {
		return beach.View{Page: noticePage("that link doesn't verify anything.", "it may have been used already — check your profile.")}, nil
	}
	a.rep.emit("email-verified", handle)
	return beach.View{Page: noticePage("email verified.", "your standing felt it.")}, nil
}

// doResendEmail regenerates the token for the signed-in member's unverified
// address and queues a fresh verification email. Already verified (or no
// address at all) is a no-op; either way, back to the profile.
func (a *app) doResendEmail(w http.ResponseWriter, r *http.Request) {
	handle, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	if handle == "" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	var uid int64
	var email string
	if err := a.pool.QueryRow(ctx,
		`SELECT e.user_id, e.email FROM hush_member_emails e
		 JOIN users u ON u.id = e.user_id
		 WHERE u.username = $1 AND e.verified_at IS NULL`, handle).Scan(&uid, &email); err == nil {
		if token, terr := a.setMemberEmail(ctx, uid, email); terr == nil {
			a.enqueueMail(ctx, email, "verify", verifyEmail(a.verifyLink(token)))
		}
	}
	http.Redirect(w, r, "/u/"+handle, http.StatusSeeOther)
}

// doSetEmail adds or changes the signed-in member's address — stored
// unverified, verification queued — then 303s back to the profile.
func (a *app) doSetEmail(w http.ResponseWriter, r *http.Request) {
	handle, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	if handle == "" {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	a.attachEmail(r.Context(), handle, r.FormValue("email"))
	http.Redirect(w, r, "/u/"+handle, http.StatusSeeOther)
}
