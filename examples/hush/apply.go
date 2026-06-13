package main

import (
	"net/http"
	"strconv"
	"strings"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/passwords"
)

// application is one pending signup application, as the admin queue shows it.
// The row stores the photo's storage key; PhotoURL is resolved from it at read
// time, so the queue renders correctly whichever store holds the bytes (a
// local /files path, an R2 public URL, or a presigned link).
type application struct {
	ID       int64
	Handle   string
	Pitch    string
	PhotoURL string
}

// applyPage is the door. While open seats remain (HUSH_OPEN_SLOTS) it renders
// the claim-a-seat form; once they're spent — or the mode is off — it falls
// back to the application queue. After a successful application POST it shows
// the submitted notice.
func (a *app) applyPage(c *beach.Ctx) (beach.View, error) {
	if c.Query("ok") == "1" {
		return beach.View{Page: noticePage("application submitted.", "an admin will review it. you'll be able to sign in once approved.")}, nil
	}
	errMsg := signupErrMsg(c.Query("err"))
	if left := a.openLeft(c.Context()); left > 0 {
		return beach.View{Page: registerFormPage(left, errMsg)}, nil
	}
	return beach.View{Page: applyFormPage(errMsg)}, nil
}

// doApply handles the application form: validate, hash the chosen password, stash
// an optional photo in the file store, and insert a pending application. An
// optional email rides the row (where the decision notice goes); the admin
// hears about the application when HUSH_ADMIN_EMAIL is set.
func (a *app) doApply(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}
	handle := strings.TrimSpace(r.FormValue("handle"))
	password := r.FormValue("password")
	pitch := strings.TrimSpace(r.FormValue("pitch"))
	email := normalizeEmail(r.FormValue("email"))
	if handle == "" || password == "" || pitch == "" {
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}
	if !agreed(r, "agree18") {
		http.Redirect(w, r, "/apply?err=agree", http.StatusSeeOther)
		return
	}
	hash, err := passwords.Hash(password)
	if err != nil {
		http.Redirect(w, r, "/apply?err="+signupErrCode(err), http.StatusSeeOther)
		return
	}

	photoKey := ""
	if f, hdr, ferr := r.FormFile("photo"); ferr == nil {
		defer f.Close()
		ct := hdr.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "image/") {
			key := "applications/" + randHex() + extFor(ct)
			if _, perr := a.files.Put(r.Context(), key, f, ct); perr == nil {
				photoKey = key
			}
		}
	}

	_, err = a.pool.Exec(r.Context(),
		`INSERT INTO hush_applications (handle, pitch, password_hash, photo_key, email) VALUES ($1, $2, $3, $4, $5)`,
		handle, pitch, hash, photoKey, email)
	if err == nil {
		a.recordAgreement(r.Context(), handle, agreeTOS18, clientMeta(r))
		if adminEmail := a.live().AdminEmail; adminEmail != "" {
			a.enqueueMail(r.Context(), adminEmail, "application-received", applicationReceivedAdmin(handle, pitch))
		}
		a.track(r, "apply", handle, 0)
	}
	http.Redirect(w, r, "/apply?ok=1", http.StatusSeeOther)
}

// queuePage loads the pending applications and hands them to adminQueuePage
// (admin only).
func (a *app) queuePage(c *beach.Ctx) (beach.View, error) {
	rows, err := a.pool.Query(c.Context(),
		`SELECT id, handle, pitch, photo_key FROM hush_applications WHERE status = 'pending' ORDER BY created_at`)
	if err != nil {
		return beach.View{}, err
	}
	defer rows.Close()

	var apps []application
	for rows.Next() {
		var ap application
		var photoKey string
		if err := rows.Scan(&ap.ID, &ap.Handle, &ap.Pitch, &photoKey); err != nil {
			return beach.View{}, err
		}
		if photoKey != "" {
			ap.PhotoURL, _ = a.files.URL(c.Context(), photoKey)
		}
		apps = append(apps, ap)
	}
	if err := rows.Err(); err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: adminQueuePage(apps)}, nil
}

// doDecide approves (creates the user + member role from the application) or
// rejects a pending application. Admin only. Either way the applicant hears
// the decision by mail — if they gave an address.
func (a *app) doDecide(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	ctx := r.Context()

	if r.FormValue("action") != "approve" {
		var handle, email string
		if err := a.pool.QueryRow(ctx,
			`UPDATE hush_applications SET status='rejected', decided_at=now()
			 WHERE id=$1 AND status='pending' RETURNING handle, email`, id).Scan(&handle, &email); err == nil && email != "" {
			a.enqueueMail(ctx, email, "application-rejected", applicationRejected(handle))
		}
		http.Redirect(w, r, "/admin/queue", http.StatusSeeOther)
		return
	}

	var handle, hash, email string
	if err := a.pool.QueryRow(ctx,
		`SELECT handle, password_hash, email FROM hush_applications WHERE id=$1 AND status='pending'`, id).Scan(&handle, &hash, &email); err != nil {
		http.Redirect(w, r, "/admin/queue", http.StatusSeeOther)
		return
	}

	var uid int64
	if err := a.pool.QueryRow(ctx,
		`INSERT INTO users (username) VALUES ($1)
		 ON CONFLICT (username) DO UPDATE SET username = EXCLUDED.username RETURNING id`, handle).Scan(&uid); err != nil {
		http.Redirect(w, r, "/admin/queue", http.StatusSeeOther)
		return
	}
	var hasCred bool
	_ = a.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_credentials_local WHERE user_id=$1)`, uid).Scan(&hasCred)
	if !hasCred {
		_, _ = a.pool.Exec(ctx, `INSERT INTO user_credentials_local (user_id, password_hash) VALUES ($1, $2)`, uid, hash)
	}
	var rid int64
	if err := a.pool.QueryRow(ctx, `SELECT id FROM roles WHERE slug='member'`).Scan(&rid); err == nil {
		_, _ = a.pool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, rid)
	}

	// The application email follows the member in: stored unverified with a
	// verify link in the welcome — unless this user (the upsert can land on an
	// existing one) already verified that very address, which stays earned.
	if email != "" {
		verifyLink := ""
		var existing string
		var verified bool
		err := a.pool.QueryRow(ctx,
			`SELECT email, verified_at IS NOT NULL FROM hush_member_emails WHERE user_id=$1`, uid).Scan(&existing, &verified)
		if err != nil || existing != email || !verified {
			if token, terr := a.setMemberEmail(ctx, uid, email); terr == nil {
				verifyLink = a.verifyLink(token)
			}
		}
		a.enqueueMail(ctx, email, "application-approved", applicationApproved(handle, a.cfg.BaseURL+"/login", verifyLink))
	}

	_, _ = a.pool.Exec(ctx, `UPDATE hush_applications SET status='approved', decided_at=now() WHERE id=$1`, id)
	http.Redirect(w, r, "/admin/queue", http.StatusSeeOther)
}
