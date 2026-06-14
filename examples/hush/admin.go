// The admin suite — the pages with eyes. Everything the member-facing wall
// refuses to print (scores, tiers, chains, vote counts, IPs, geo, user
// agents) is laid out here, behind app.Can("admin"): the hub (/admin), the
// moderation review (/admin/review), the everything browser (/admin/posts),
// and the member ledger (/admin/members). The views live in adminviews.templ.
// All of these read across hush's Postgres tables.
package main

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
)

// adminStats is the hub's quick read: what needs attention, and how big the
// place has gotten.
type adminStats struct {
	Pending int // applications awaiting a decision
	Flagged int // posts hidden or carrying flags
	Posts   int
	Members int
}

// adminHub is /admin: the four numbers and the way into each section.
func (a *app) adminHub(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	var st adminStats
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM hush_applications WHERE status = 'pending'`).Scan(&st.Pending)
	_ = a.pool.QueryRow(ctx,
		`SELECT count(DISTINCT p.id) FROM hush_posts p
		 LEFT JOIN hush_flags f ON f.post_id = p.id
		 WHERE p.hidden_at IS NOT NULL OR f.post_id IS NOT NULL`).Scan(&st.Flagged)
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM hush_posts`).Scan(&st.Posts)
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&st.Members)
	return beach.View{Page: adminHubPage(st)}, nil
}

// flagRow is one recorded flag as the review page lists it: who flagged, and
// the tier they held at flag time — what the hiding rules actually weighed.
type flagRow struct {
	Flagger string
	Tier    int
}

// adminReview is the moderation queue: every hidden or flagged post in full —
// the wall's flagged read, plus who flagged each one out of hush_flags.
func (a *app) adminReview(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	posts := a.store.flagged(ctx)
	flaggers := map[int64][]flagRow{}
	if rows, err := a.pool.Query(ctx,
		`SELECT post_id, flagger, flagger_tier FROM hush_flags ORDER BY created_at`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var fr flagRow
			if rows.Scan(&id, &fr.Flagger, &fr.Tier) == nil {
				flaggers[id] = append(flaggers[id], fr)
			}
		}
	}
	return beach.View{Page: a.adminReviewPage(posts, flaggers)}, nil
}

// adminPostsLimit caps the everything browser at the newest rows — dense, not
// exhaustive; the filters narrow within that window.
const adminPostsLimit = 500

// voteCount is one post's tallied reactions — numbers members never see.
type voteCount struct{ Up, Down int }

// adminPosts is the everything browser: every post including hidden ones,
// newest-first, with optional exact-match ?author= and ?ip= filters applied
// here in the handler (the store read stays one shape).
func (a *app) adminPosts(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	author := strings.TrimSpace(c.Query("author"))
	ip := strings.TrimSpace(c.Query("ip"))
	posts := a.store.adminList(ctx, adminPostsLimit)
	if author != "" || ip != "" {
		filtered := make([]Post, 0, len(posts))
		for _, p := range posts {
			if author != "" && p.Author != author {
				continue
			}
			if ip != "" && p.Meta.IP != ip {
				continue
			}
			filtered = append(filtered, p)
		}
		posts = filtered
	}
	// reaction tallies per post — the tolerant read style: errors mean zeros
	votes := map[int64]voteCount{}
	if rows, err := a.pool.Query(ctx,
		`SELECT post_id,
		        count(*) FILTER (WHERE weight > 0),
		        count(*) FILTER (WHERE weight < 0)
		 FROM hush_reactions GROUP BY post_id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var v voteCount
			if rows.Scan(&id, &v.Up, &v.Down) == nil {
				votes[id] = v
			}
		}
	}
	return beach.View{Page: a.adminPostsPage(posts, votes, author, ip)}, nil
}

// memberRow is one account as the ledger lists it: the roles and the real
// numbers (score, tier, chain) the rest of hush works so hard never to print.
type memberRow struct {
	Handle   string
	Roles    string
	Score    float64
	Tier     int
	Chain    float64
	Posts    int
	Email    string // "" when hush_member_emails has no row
	Verified bool
	Minted   int // invites minted
	Used     int // ...of which redeemed
	LastIP   string
}

// adminMembers is the member ledger: every account joined against its roles
// and optional email, then filled in from the per-author aggregates (posts,
// last-seen IP, invites) and the in-memory Repute scores.
func (a *app) adminMembers(c *beach.Ctx) (beach.View, error) {
	ctx := c.Context()
	rows, err := a.pool.Query(ctx,
		`SELECT u.username,
		        coalesce(string_agg(DISTINCT r.slug, ' '), ''),
		        coalesce(max(e.email), ''),
		        bool_or(e.verified_at IS NOT NULL)
		 FROM users u
		 LEFT JOIN user_roles ur ON ur.user_id = u.id
		 LEFT JOIN roles r ON r.id = ur.role_id
		 LEFT JOIN hush_member_emails e ON e.user_id = u.id
		 GROUP BY u.id ORDER BY u.id`)
	if err != nil {
		return beach.View{}, err
	}
	defer rows.Close()
	var members []memberRow
	for rows.Next() {
		var m memberRow
		if err := rows.Scan(&m.Handle, &m.Roles, &m.Email, &m.Verified); err != nil {
			return beach.View{}, err
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return beach.View{}, err
	}

	// the per-author aggregates, tolerant reads into maps
	postCounts := map[string]int{}
	if rows, err := a.pool.Query(ctx,
		`SELECT author, count(*) FROM hush_posts WHERE author <> '' GROUP BY author`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var h string
			var n int
			if rows.Scan(&h, &n) == nil {
				postCounts[h] = n
			}
		}
	}
	lastIPs := map[string]string{}
	if rows, err := a.pool.Query(ctx,
		`SELECT DISTINCT ON (author) author, ip FROM hush_posts WHERE author <> '' ORDER BY author, id DESC`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var h, ip string
			if rows.Scan(&h, &ip) == nil {
				lastIPs[h] = ip
			}
		}
	}
	type tally struct{ minted, used int }
	invites := map[string]tally{}
	if rows, err := a.pool.Query(ctx,
		`SELECT inviter, count(*), count(used_by) FROM hush_invites GROUP BY inviter`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var h string
			var t tally
			if rows.Scan(&h, &t.minted, &t.used) == nil {
				invites[h] = t
			}
		}
	}

	for i := range members {
		h := members[i].Handle
		members[i].Score = a.rep.score(h)
		members[i].Tier = a.tierOf(members[i].Score)
		members[i].Chain = a.rep.chainScore(h)
		members[i].Posts = postCounts[h]
		members[i].LastIP = lastIPs[h]
		members[i].Minted, members[i].Used = invites[h].minted, invites[h].used
	}
	return beach.View{Page: adminMembersPage(members)}, nil
}

// doModerate applies one admin ruling to a post — hide, restore, or purge —
// then 303s back to the page that asked (review, or the post browser with its
// filters intact). Anything outside /admin falls back to review, so the form
// can't be used to redirect off-site.
func (a *app) doModerate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	ctx := r.Context()
	admin, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	switch r.FormValue("action") {
	case "hide":
		_ = a.store.hide(ctx, id, admin)
	case "restore":
		_ = a.store.restore(ctx, id)
	case "purge":
		_ = a.store.purge(ctx, id, admin)
	}
	back := r.FormValue("back")
	if !strings.HasPrefix(back, "/admin") {
		back = "/admin/review"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// adminSettings is /admin/settings: the live gate-and-moderation policy as an
// editable form.
func (a *app) adminSettings(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: adminSettingsPage(a.live(), c.Query("saved") == "1")}, nil
}

// doSaveSettings persists the edited policy and swaps the live pointer — the
// change takes effect on the next request, no restart. A field that won't
// parse keeps its current value rather than resetting to zero.
func (a *app) doSaveSettings(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	ls := a.live()
	ls.Tier1 = formFloat(r, "tier1", ls.Tier1)
	ls.Tier2 = formFloat(r, "tier2", ls.Tier2)
	ls.FlagTier = formInt(r, "flag_tier", ls.FlagTier)
	ls.FlagQuorum = formInt(r, "flag_quorum", ls.FlagQuorum)
	ls.InviteBase = formInt(r, "invite_base", ls.InviteBase)
	ls.InviteMax = formInt(r, "invite_max", ls.InviteMax)
	ls.OpenSlots = formInt(r, "open_slots", ls.OpenSlots)
	ls.AdminEmail = strings.TrimSpace(r.FormValue("admin_email"))
	_ = a.saveSettings(r.Context(), ls)
	http.Redirect(w, r, "/admin/settings?saved=1", http.StatusSeeOther)
}

// formInt/formFloat read one numeric form field, keeping def on a bad parse.
func formInt(r *http.Request, name string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(r.FormValue(name))); err == nil {
		return n
	}
	return def
}

func formFloat(r *http.Request, name string, def float64) float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue(name)), 64); err == nil {
		return f
	}
	return def
}

// mailRow is one outbox entry as the admin page lists it — enough to see what
// went, what's waiting, and what's parked failed with its last error.
type mailRow struct {
	ID       int64
	To       string
	Subject  string
	Template string
	Status   string
	Attempts int
	LastErr  string
	Created  string
	Sent     string
}

// adminOutbox is the transactional-mail ledger: the most recent 200 rows of
// hush_outbox, newest first — failed sends are otherwise invisible outside
// the process log.
func (a *app) adminOutbox(c *beach.Ctx) (beach.View, error) {
	rows, err := a.pool.Query(c.Context(),
		`SELECT id, to_email, subject, template_key, status, attempts, last_error,
		        to_char(created_at, 'Mon DD HH24:MI'), coalesce(to_char(sent_at, 'Mon DD HH24:MI'), '')
		 FROM hush_outbox ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return beach.View{}, err
	}
	defer rows.Close()
	var mails []mailRow
	for rows.Next() {
		var m mailRow
		if err := rows.Scan(&m.ID, &m.To, &m.Subject, &m.Template, &m.Status, &m.Attempts, &m.LastErr, &m.Created, &m.Sent); err != nil {
			return beach.View{}, err
		}
		mails = append(mails, m)
	}
	if err := rows.Err(); err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: adminOutboxPage(mails)}, nil
}

// postsBack is the browser's round-trip path: moderating a row 303s back to
// the same filtered view.
func postsBack(author, ip string) string {
	q := url.Values{}
	if author != "" {
		q.Set("author", author)
	}
	if ip != "" {
		q.Set("ip", ip)
	}
	if len(q) == 0 {
		return "/admin/posts"
	}
	return "/admin/posts?" + q.Encode()
}

// snippetCap keeps the browser's content column scannable.
const snippetCap = 90

// snippet flattens a post's sanitized HTML to plain text for a table cell:
// stripPolicy (text.go) drops the tags, unescape undoes its entity escaping
// (templ re-escapes on render), and the cap truncates.
func snippet(bodyHTML string) string {
	plain := html.UnescapeString(strings.Join(strings.Fields(stripPolicy.Sanitize(bodyHTML)), " "))
	r := []rune(plain)
	if len(r) > snippetCap {
		return string(r[:snippetCap]) + "…"
	}
	return plain
}

// authorLabel names a row's author; seed posts have none.
func authorLabel(author string) string {
	if author == "" {
		return "anon"
	}
	return author
}

// metaLine is the capture-time request context as one terse line:
// ip · country · city · ua, absent parts skipped.
func metaLine(m Meta) string {
	var parts []string
	for _, s := range []string{m.IP, m.Country, m.City, m.UA} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}

// geoLabel is country · city, whichever the capture had.
func geoLabel(m Meta) string {
	switch {
	case m.Country != "" && m.City != "":
		return m.Country + " · " + m.City
	case m.Country != "":
		return m.Country
	default:
		return m.City
	}
}

// fmtScore prints a reputation number plainly — these pages are the one place
// the numbers appear at all.
func fmtScore(s float64) string {
	return strconv.FormatFloat(s, 'f', -1, 64)
}
