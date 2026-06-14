// The Postgres wall — hush's only store. Posts, reactions, flags, and the
// moderation state all live here; the app requires a database, so this is the
// store, not one of two. Post types and the boot seed are in model.go.
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the Postgres-backed wall: posts, reactions, flags, and moderation
// state.
type Store struct {
	pool *pgxpool.Pool
}

// New wraps the pool and seeds the wall on first boot: render turns each seed
// markdown into sanitized HTML (the same content.RenderPost the live composer
// uses — passed in so the store needn't import content), inserted only when
// hush_posts is empty so a redeploy never re-seeds.
func New(ctx context.Context, pool *pgxpool.Pool, render func(string) (string, bool)) (*Store, error) {
	s := &Store{pool: pool}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM hush_posts`).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		for _, md := range SeedPosts {
			if html, ok := render(md); ok {
				if _, err := s.Add(ctx, "text", html, "", Meta{}); err != nil {
					return nil, err
				}
			}
		}
	}
	return s, nil
}

// postCols is the SELECT list every post read shares; scanPost matches it.
const postCols = `p.id, p.kind, p.body_html, p.media_url, p.cf_image_id, p.author,
	p.ip, p.country, p.city, p.user_agent, p.created_at, (p.hidden_at IS NOT NULL),
	(p.deleted_at IS NOT NULL)`

// visibleWhere is the public wall's filter: not hidden, not soft-deleted.
const visibleWhere = `p.hidden_at IS NULL AND p.deleted_at IS NULL`

// pgxScanner is the Scan seam pgx.Row and pgx.Rows share.
type pgxScanner interface{ Scan(dest ...any) error }

// scanPost reads one postCols row into p; withFlags also reads a trailing
// flag-count column (the admin reads append it to postCols).
func scanPost(sc pgxScanner, p *Post, withFlags bool) error {
	dest := []any{&p.ID, &p.Kind, &p.BodyHTML, &p.ImageURL, &p.CFImageID, &p.Author,
		&p.Meta.IP, &p.Meta.Country, &p.Meta.City, &p.Meta.UA, &p.CreatedAt, &p.Hidden, &p.Deleted}
	if withFlags {
		dest = append(dest, &p.FlagCount)
	}
	return sc.Scan(dest...)
}

// queryPosts runs a postCols query and scans the lot. Read errors come back as
// an empty wall — the handlers' tolerant style, matching the rest of hush.
func (s *Store) queryPosts(ctx context.Context, withFlags bool, q string, args ...any) []Post {
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

func (s *Store) List(ctx context.Context) []Post {
	return s.queryPosts(ctx, false,
		`SELECT `+postCols+` FROM hush_posts p WHERE `+visibleWhere+` ORDER BY p.id DESC`)
}

func (s *Store) Get(ctx context.Context, id int64) (Post, bool) {
	var p Post
	err := scanPost(s.pool.QueryRow(ctx,
		`SELECT `+postCols+` FROM hush_posts p WHERE `+visibleWhere+` AND p.id = $1`, id), &p, false)
	return p, err == nil
}

// anyPost fetches a post regardless of hidden state — Vote needs the row (for
// the author credit) even when a stale page reacts to a just-hidden post.
func (s *Store) anyPost(ctx context.Context, id int64) (Post, bool) {
	var p Post
	err := scanPost(s.pool.QueryRow(ctx,
		`SELECT `+postCols+` FROM hush_posts p WHERE p.id = $1`, id), &p, false)
	return p, err == nil
}

func (s *Store) Neighbors(ctx context.Context, id int64, newer bool, limit int) ([]Post, bool) {
	q := `SELECT ` + postCols + ` FROM hush_posts p WHERE ` + visibleWhere + ` AND p.id < $1 ORDER BY p.id DESC LIMIT $2`
	if newer {
		q = `SELECT ` + postCols + ` FROM hush_posts p WHERE ` + visibleWhere + ` AND p.id > $1 ORDER BY p.id ASC LIMIT $2`
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

func (s *Store) Add(ctx context.Context, kind, bodyHTML, author string, m Meta) (Post, error) {
	p := Post{Kind: kind, BodyHTML: bodyHTML, Author: author, Meta: m}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hush_posts (kind, body_html, author, ip, country, city, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		kind, bodyHTML, author, m.IP, m.Country, m.City, m.UA).Scan(&p.ID, &p.CreatedAt)
	return p, err
}

func (s *Store) AddImage(ctx context.Context, mediaURL, cfImageID, author string, m Meta) (Post, error) {
	p := Post{Kind: "image", ImageURL: mediaURL, CFImageID: cfImageID, Author: author, Meta: m}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hush_posts (kind, media_url, cf_image_id, author, ip, country, city, user_agent)
		 VALUES ('image', $1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		mediaURL, cfImageID, author, m.IP, m.Country, m.City, m.UA).Scan(&p.ID, &p.CreatedAt)
	return p, err
}

func (s *Store) Vote(ctx context.Context, postID int64, voter string, up bool) (Post, bool) {
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

func (s *Store) VoteOf(ctx context.Context, postID int64, voter string) int8 {
	var w int16
	if err := s.pool.QueryRow(ctx,
		`SELECT weight FROM hush_reactions WHERE post_id = $1 AND voter = $2`, postID, voter).Scan(&w); err != nil {
		return 0
	}
	return int8(w)
}

func (s *Store) CountByAuthor(ctx context.Context, handle string) int {
	var n int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM hush_posts WHERE author = $1`, handle).Scan(&n)
	return n
}

func (s *Store) Flag(ctx context.Context, postID int64, flagger string, flaggerTier int, instaTier, quorum int) (bool, bool) {
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

// FlagOf reports whether flagger already flagged postID.
func (s *Store) FlagOf(ctx context.Context, postID int64, flagger string) bool {
	var ok bool
	_ = s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM hush_flags WHERE post_id = $1 AND flagger = $2)`, postID, flagger).Scan(&ok)
	return ok
}

func (s *Store) Hide(ctx context.Context, id int64, by string) error {
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
			return ErrNoPost
		}
	}
	return nil
}

func (s *Store) Restore(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE hush_posts SET hidden_at = NULL, hidden_by = '' WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoPost
	}
	// the admin ruled it fine — drop the flags so they can't re-hide it
	_, err = s.pool.Exec(ctx, `DELETE FROM hush_flags WHERE post_id = $1`, id)
	return err
}

func (s *Store) Purge(ctx context.Context, id int64, by string) error {
	// soft: the row, reactions, flags, and capture meta stay as evidence (the
	// terms promise exactly that); the post just leaves every public surface
	tag, err := s.pool.Exec(ctx,
		`UPDATE hush_posts SET deleted_at = now(), deleted_by = $2 WHERE id = $1 AND deleted_at IS NULL`, id, by)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// already purged is fine; missing is not
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hush_posts WHERE id = $1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNoPost
		}
	}
	return nil
}

func (s *Store) Flagged(ctx context.Context) []Post {
	// purged posts are settled — out of the review queue, kept only in the
	// admin browser
	return s.queryPosts(ctx, true,
		`SELECT `+postCols+`, count(f.post_id)::int
		 FROM hush_posts p LEFT JOIN hush_flags f ON f.post_id = p.id
		 WHERE p.deleted_at IS NULL AND (p.hidden_at IS NOT NULL OR f.post_id IS NOT NULL)
		 GROUP BY p.id
		 ORDER BY p.id DESC`)
}

func (s *Store) AdminList(ctx context.Context, limit int) []Post {
	return s.queryPosts(ctx, true,
		`SELECT `+postCols+`, (SELECT count(*) FROM hush_flags f WHERE f.post_id = p.id)::int
		 FROM hush_posts p ORDER BY p.id DESC LIMIT $1`, limit)
}
