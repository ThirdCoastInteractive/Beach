package main

import (
	"net/http"
	"strconv"
	"strings"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/passwords"
	"github.com/a-h/templ"
)

// applyPage renders the signup application form: a handle, a password, 3–5 lines
// about what you want to post, and an optional photo. Submitting creates a
// pending application an admin reviews. Without a DB there are no accounts.
func (a *app) applyPage(c *beach.Ctx) (beach.View, error) {
	if a.authn == nil {
		return beach.View{Page: noticePage("applications need the database.", "set DATABASE_URL to enable the gate.")}, nil
	}
	if c.Query("ok") == "1" {
		return beach.View{Page: noticePage("application submitted.", "an admin will review it. you'll be able to sign in once approved.")}, nil
	}
	form := `<form method="post" action="/apply" enctype="multipart/form-data" class="hush-form">` +
		`<input class="hush-input" type="text" name="handle" placeholder="handle" autocomplete="username" required>` +
		`<input class="hush-input" type="password" name="password" placeholder="password" autocomplete="new-password" required>` +
		`<textarea class="hush-input" name="pitch" rows="4" placeholder="what do you want to post? (3–5 lines)" required></textarea>` +
		`<label class="hush-hint">a photo of you (optional)</label>` +
		`<input class="hush-file" type="file" name="photo" accept="image/*">` +
		`<div class="hush-form-row"><a class="hush-link" href="/login">already approved? sign in →</a>` +
		`<button class="hush-post" type="submit">apply</button></div></form>`
	return beach.View{Page: formPage("apply to post", templ.Raw(form))}, nil
}

// doApply handles the application form: validate, hash the chosen password, stash
// an optional photo in the file store, and insert a pending application.
func (a *app) doApply(w http.ResponseWriter, r *http.Request) {
	if a.authn == nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}
	handle := strings.TrimSpace(r.FormValue("handle"))
	password := r.FormValue("password")
	pitch := strings.TrimSpace(r.FormValue("pitch"))
	if handle == "" || password == "" || pitch == "" {
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}
	hash, err := passwords.Hash(password)
	if err != nil {
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
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

	_, _ = a.pool.Exec(r.Context(),
		`INSERT INTO hush_applications (handle, pitch, password_hash, photo_key) VALUES ($1, $2, $3, $4)`,
		handle, pitch, hash, photoKey)
	http.Redirect(w, r, "/apply?ok=1", http.StatusSeeOther)
}

// queuePage lists pending applications with approve/reject controls (admin only).
func (a *app) queuePage(c *beach.Ctx) (beach.View, error) {
	if a.authn == nil {
		return beach.View{Page: noticePage("the queue needs the database.", "")}, nil
	}
	rows, err := a.pool.Query(c.Context(),
		`SELECT id, handle, pitch, photo_key FROM hush_applications WHERE status = 'pending' ORDER BY created_at`)
	if err != nil {
		return beach.View{}, err
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString(`<div class="hush-top"><header class="hush-head"><h1 class="hush-title">hush</h1>`)
	b.WriteString(`<p class="hush-sub">pending applications</p></header>`)
	b.WriteString(`<p class="hush-back"><a class="hush-link" href="/">← back to the wall</a></p>`)

	n := 0
	for rows.Next() {
		var id int64
		var handle, pitch, photo string
		if err := rows.Scan(&id, &handle, &pitch, &photo); err != nil {
			return beach.View{}, err
		}
		n++
		idStr := strconv.FormatInt(id, 10)
		b.WriteString(`<div class="hush-appcard">`)
		if photo != "" {
			b.WriteString(`<img class="hush-appphoto" src="/files/` + templ.EscapeString(photo) + `" alt="">`)
		}
		b.WriteString(`<div class="hush-apphandle">` + templ.EscapeString(handle) + `</div>`)
		b.WriteString(`<div class="hush-apppitch">` + templ.EscapeString(pitch) + `</div>`)
		b.WriteString(`<div class="hush-form-row">`)
		b.WriteString(`<form method="post" action="/admin/decide"><input type="hidden" name="id" value="` + idStr + `">` +
			`<input type="hidden" name="action" value="reject"><button class="hush-post hush-post--ghost" type="submit">reject</button></form>`)
		b.WriteString(`<form method="post" action="/admin/decide"><input type="hidden" name="id" value="` + idStr + `">` +
			`<input type="hidden" name="action" value="approve"><button class="hush-post" type="submit">approve</button></form>`)
		b.WriteString(`</div></div>`)
	}
	if n == 0 {
		b.WriteString(`<p class="hush-sub">nothing pending.</p>`)
	}
	b.WriteString(`</div>`)
	return beach.View{Page: shell("hush — queue", templ.Raw(b.String()))}, nil
}

// doDecide approves (creates the user + member role from the application) or
// rejects a pending application. Admin only.
func (a *app) doDecide(w http.ResponseWriter, r *http.Request) {
	if a.authn == nil {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	ctx := r.Context()

	if r.FormValue("action") != "approve" {
		_, _ = a.pool.Exec(ctx, `UPDATE hush_applications SET status='rejected', decided_at=now() WHERE id=$1 AND status='pending'`, id)
		http.Redirect(w, r, "/admin/queue", http.StatusSeeOther)
		return
	}

	var handle, hash string
	if err := a.pool.QueryRow(ctx,
		`SELECT handle, password_hash FROM hush_applications WHERE id=$1 AND status='pending'`, id).Scan(&handle, &hash); err != nil {
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
	_, _ = a.pool.Exec(ctx, `UPDATE hush_applications SET status='approved', decided_at=now() WHERE id=$1`, id)
	http.Redirect(w, r, "/admin/queue", http.StatusSeeOther)
}
