// The Postgres wall. Same interface as the in-memory store (model.go), but
// posts, reactions, and flags survive restarts — production mode, switched on
// by the same DATABASE_URL that enables the gate.
package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgStore struct {
	pool *pgxpool.Pool
}

// newPGStore wraps the pool and seeds the wall on first boot: the same
// seedPosts the in-memory store uses, inserted only when hush_posts is empty
// so a redeploy never re-seeds.
func newPGStore(ctx context.Context, pool *pgxpool.Pool) (*pgStore, error) {
	s := &pgStore{pool: pool}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM hush_posts`).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		for _, md := range seedPosts {
			if html, ok := renderPost(md); ok {
				if _, err := s.add(ctx, "text", html, "", Meta{}); err != nil {
					return nil, err
				}
			}
		}
	}
	return s, nil
}

// postCols is the SELECT list every post read shares; scanPost matches it.
const postCols = `p.id, p.kind, p.body_html, p.media_url, p.cf_image_id, p.author,
	p.ip, p.country, p.city, p.user_agent, p.created_at, (p.hidden_at IS NOT NULL)`

// pgxScanner is the Scan seam pgx.Row and pgx.Rows share.
type pgxScanner interface{ Scan(dest ...any) error }

// scanPost reads one postCols row into p; withFlags also reads a trailing
// flag-count column (the admin reads append it to postCols).
func scanPost(sc pgxScanner, p *Post, withFlags bool) error {
	dest := []any{&p.ID, &p.Kind, &p.BodyHTML, &p.ImageURL, &p.CFImageID, &p.Author,
		&p.Meta.IP, &p.Meta.Country, &p.Meta.City, &p.Meta.UA, &p.CreatedAt, &p.Hidden}
	if withFlags {
		dest = append(dest, &p.FlagCount)
	}
	return sc.Scan(dest...)
}

// queryPosts runs a postCols query and scans the lot. Read errors come back as
// an empty wall — the handlers' tolerant style, matching the rest of hush.
func (s *pgStore) queryPosts(ctx context.Context, withFlags bool, q string, args ...any) []Post {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Post
	for rows.Next() {
		var p Post
		if err := scanPost(rows, &p, withFlags); err != nil {
			return nil
		}
		out = append(out, p)
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}

func (s *pgStore) list(ctx context.Context) []Post {
	return s.queryPosts(ctx, false,
		`SELECT `+postCols+` FROM hush_posts p WHERE p.hidden_at IS NULL ORDER BY p.id DESC`)
}

func (s *pgStore) get(ctx context.Context, id int64) (Post, bool) {
	var p Post
	err := scanPost(s.pool.QueryRow(ctx,
		`SELECT `+postCols+` FROM hush_posts p WHERE p.hidden_at IS NULL AND p.id = $1`, id), &p, false)
	return p, err == nil
}

// anyPost fetches a post regardless of hidden state — vote needs the row (for
// the author credit) even when a stale page reacts to a just-hidden post.
func (s *pgStore) anyPost(ctx context.Context, id int64) (Post, bool) {
	var p Post
	err := scanPost(s.pool.QueryRow(ctx,
		`SELECT `+postCols+` FROM hush_posts p WHERE p.id = $1`, id), &p, false)
	return p, err == nil
}

func (s *pgStore) neighbors(ctx context.Context, id int64, newer bool, limit int) ([]Post, bool) {
	q := `SELECT ` + postCols + ` FROM hush_posts p WHERE p.hidden_at IS NULL AND p.id < $1 ORDER BY p.id DESC LIMIT $2`
	if newer {
		q = `SELECT ` + postCols + ` FROM hush_posts p WHERE p.hidden_at IS NULL AND p.id > $1 ORDER BY p.id ASC LIMIT $2`
	}
	out := s.queryPosts(ctx, false, q, id, limit+1)
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	if newer { // collected ascending; the feed renders newest-first
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, more
}

func (s *pgStore) add(ctx context.Context, kind, bodyHTML, author string, m Meta) (Post, error) {
	p := Post{Kind: kind, BodyHTML: bodyHTML, Author: author, Meta: m}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hush_posts (kind, body_html, author, ip, country, city, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		kind, bodyHTML, author, m.IP, m.Country, m.City, m.UA).Scan(&p.ID, &p.CreatedAt)
	return p, err
}

func (s *pgStore) addImage(ctx context.Context, mediaURL, cfImageID, author string, m Meta) (Post, error) {
	p := Post{Kind: "image", ImageURL: mediaURL, CFImageID: cfImageID, Author: author, Meta: m}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hush_posts (kind, media_url, cf_image_id, author, ip, country, city, user_agent)
		 VALUES ('image', $1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		mediaURL, cfImageID, author, m.IP, m.Country, m.City, m.UA).Scan(&p.ID, &p.CreatedAt)
	return p, err
}

func (s *pgStore) vote(ctx context.Context, postID int64, voter string, up bool) (Post, bool) {
	p, ok := s.anyPost(ctx, postID)
	if !ok {
		return Post{}, false
	}
	weight := -1
	if up {
		weight = 1
	}
	// one reaction per voter per post, the first is final — the PK enforces it,
	// DO NOTHING makes a repeat click a no-op instead of an error
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO hush_reactions (post_id, voter, weight) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		postID, voter, weight)
	if err != nil {
		return p, false
	}
	return p, tag.RowsAffected() == 1
}

func (s *pgStore) voteOf(ctx context.Context, postID int64, voter string) int8 {
	var w int16
	if err := s.pool.QueryRow(ctx,
		`SELECT weight FROM hush_reactions WHERE post_id = $1 AND voter = $2`, postID, voter).Scan(&w); err != nil {
		return 0
	}
	return int8(w)
}

func (s *pgStore) countByAuthor(ctx context.Context, handle string) int {
	var n int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM hush_posts WHERE author = $1`, handle).Scan(&n)
	return n
}

func (s *pgStore) flag(ctx context.Context, postID int64, flagger string, flaggerTier int, instaTier, quorum int) (bool, bool) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO hush_flags (post_id, flagger, flagger_tier) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		postID, flagger, flaggerTier)
	if err != nil { // FK violation: no such post
		return false, false
	}
	fresh := tag.RowsAffected() == 1
	// the hiding rules: one trusted flag, or a quorum of any
	_, _ = s.pool.Exec(ctx,
		`UPDATE hush_posts SET hidden_at = now(), hidden_by = 'flags'
		 WHERE id = $1 AND hidden_at IS NULL
		   AND (SELECT coalesce(max(flagger_tier), -1) >= $2 OR count(*) >= $3
		        FROM hush_flags WHERE post_id = $1)`,
		postID, instaTier, quorum)
	var hidden bool
	_ = s.pool.QueryRow(ctx, `SELECT hidden_at IS NOT NULL FROM hush_posts WHERE id = $1`, postID).Scan(&hidden)
	return hidden, fresh
}

// flagOf reports whether flagger already flagged postID.
func (s *pgStore) flagOf(ctx context.Context, postID int64, flagger string) bool {
	var ok bool
	_ = s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM hush_flags WHERE post_id = $1 AND flagger = $2)`, postID, flagger).Scan(&ok)
	return ok
}

func (s *pgStore) hide(ctx context.Context, id int64, by string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE hush_posts SET hidden_at = now(), hidden_by = $2 WHERE id = $1 AND hidden_at IS NULL`, id, by)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// already hidden is fine; missing is not
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hush_posts WHERE id = $1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errNoPost
		}
	}
	return nil
}

func (s *pgStore) restore(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE hush_posts SET hidden_at = NULL, hidden_by = '' WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoPost
	}
	// the admin ruled it fine — drop the flags so they can't re-hide it
	_, err = s.pool.Exec(ctx, `DELETE FROM hush_flags WHERE post_id = $1`, id)
	return err
}

func (s *pgStore) purge(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM hush_posts WHERE id = $1`, id) // reactions + flags cascade
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoPost
	}
	return nil
}

func (s *pgStore) flagged(ctx context.Context) []Post {
	return s.queryPosts(ctx, true,
		`SELECT `+postCols+`, count(f.post_id)::int
		 FROM hush_posts p LEFT JOIN hush_flags f ON f.post_id = p.id
		 WHERE p.hidden_at IS NOT NULL OR f.post_id IS NOT NULL
		 GROUP BY p.id
		 ORDER BY p.id DESC`)
}

func (s *pgStore) adminList(ctx context.Context, limit int) []Post {
	return s.queryPosts(ctx, true,
		`SELECT `+postCols+`, (SELECT count(*) FROM hush_flags f WHERE f.post_id = p.id)::int
		 FROM hush_posts p ORDER BY p.id DESC LIMIT $1`, limit)
}

// replayReputation rebuilds the in-memory Repute engine from Postgres at boot:
// every post emits "post", every reaction emits "like"/"dislike" (plus the
// half-weight chain echo to the author's inviter — hydrateChains must have run
// first), and every verified member email emits "email-verified". The engine
// stays in-memory; the wall's history is its event log.
func (a *app) replayReputation(ctx context.Context) error {
	authors, err := scanStrings(ctx, a.pool, `SELECT author FROM hush_posts ORDER BY id`)
	if err != nil {
		return err
	}
	for _, author := range authors {
		a.rep.emit("post", author)
	}

	rows, err := a.pool.Query(ctx,
		`SELECT p.author, r.weight FROM hush_reactions r
		 JOIN hush_posts p ON p.id = r.post_id ORDER BY r.created_at, r.post_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var author string
		var weight int16
		if err := rows.Scan(&author, &weight); err != nil {
			return err
		}
		kind, chainKind := "like", "chain-like"
		if weight < 0 {
			kind, chainKind = "dislike", "chain-dislike"
		}
		a.rep.emit(kind, author)
		if inviter := a.inviterOf(author); inviter != "" {
			a.rep.emit(chainKind, inviter)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	verified, err := scanStrings(ctx, a.pool,
		`SELECT u.username FROM hush_member_emails e
		 JOIN users u ON u.id = e.user_id WHERE e.verified_at IS NOT NULL`)
	if err != nil {
		return err
	}
	for _, handle := range verified {
		a.rep.emit("email-verified", handle)
	}
	return nil
}
