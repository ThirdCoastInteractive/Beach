package main

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Post is one tile on the wall. A text post carries sanitized BodyHTML (from
// renderPost — never raw user input); an image post carries ImageURL, the public
// URL its bytes were stored at via storage.Store (plus CFImageID once uploads
// move to Cloudflare Images). Hidden, FlagCount, and Meta exist for moderation —
// admin eyes only; templates never render them.
type Post struct {
	ID        int64
	Kind      string // "text" | "image"
	BodyHTML  string // text posts
	ImageURL  string // image posts
	CFImageID string // image posts, Cloudflare Images id ("" until that phase)
	Author    string // handle that posted it (for Repute scoring; never shown — the wall is anon)
	Hidden    bool   // moderated off the wall; permalink 404s for non-admins
	FlagCount int    // filled on admin reads
	Meta      Meta   // capture-time request context; admin eyes only
	CreatedAt time.Time
}

// errNoPost is the stores' answer to moderating a post that isn't there.
var errNoPost = errors.New("hush: no such post")

// wall is the post store both backends implement: memStore keeps the
// database-less mode alive (zero setup, like boardwalk), pgStore is the
// production path. Context-first throughout — the pg side runs real queries.
//
// list, get, and neighbors exclude hidden posts: moderated content simply
// isn't on the wall, and its permalink reads as not found. The moderation
// methods (flag/hide/restore/purge) and the admin reads (flagged/adminList)
// are the production foundations — later phases wire their routes.
type wall interface {
	// list returns the visible wall newest-first.
	list(ctx context.Context) []Post
	// get returns a visible post by id.
	get(ctx context.Context, id int64) (Post, bool)
	// neighbors returns up to limit visible posts adjacent to id in time order —
	// the newer side or the older — newest-first, plus whether more remain.
	// IDs are assigned monotonically, so they are the timestamp cursor.
	neighbors(ctx context.Context, id int64, newer bool, limit int) ([]Post, bool)
	// add appends a text post by author, with its capture-time meta.
	add(ctx context.Context, kind, bodyHTML, author string, m Meta) (Post, error)
	// addImage appends an image post pointing at a stored object's URL.
	addImage(ctx context.Context, mediaURL, cfImageID, author string, m Meta) (Post, error)
	// vote records voter's reaction — one per voter per post, the first is
	// final — and returns the post plus whether the reaction was new.
	vote(ctx context.Context, postID int64, voter string, up bool) (Post, bool)
	// voteOf returns voter's recorded reaction to postID: +1, -1, or 0 for none.
	voteOf(ctx context.Context, postID int64, voter string) int8
	// countByAuthor returns how many posts a handle has authored.
	countByAuthor(ctx context.Context, handle string) int
	// flag records one flagger's flag (idempotent; fresh reports whether it was
	// new) and applies the hiding rules: a flag from instaTier or above hides
	// alone, quorum flags hide by agreement. hidden reports the post's state
	// after the flag.
	flag(ctx context.Context, postID int64, flagger string, flaggerTier int, instaTier, quorum int) (hidden bool, fresh bool)
	// hide takes a post off the wall, recording who did it.
	hide(ctx context.Context, id int64, by string) error
	// restore puts a hidden post back and clears its flags — the admin has
	// ruled it fine, so the old flags shouldn't re-hide it on the next one.
	restore(ctx context.Context, id int64) error
	// purge deletes a post outright, reactions and flags with it.
	purge(ctx context.Context, id int64) error
	// flagged returns posts needing review — hidden or carrying flags — newest
	// first, with FlagCount filled.
	flagged(ctx context.Context) []Post
	// adminList returns everything including hidden posts, newest first, meta
	// and FlagCount filled.
	adminList(ctx context.Context, limit int) []Post
}

// memStore is the in-memory wall — the database-less mode, explorable with
// zero setup. Posts render newest-first. There are no replies — a post stands
// alone; the only answer the wall allows is a thumb.
type memStore struct {
	mu     sync.Mutex
	posts  []Post                    // ID-ascending; Meta rides each row
	voters map[int64]map[string]int8 // post id → handle → reaction (+1/−1; one, final)
	flags  map[int64]map[string]int  // post id → flagger → tier at flag time
	hidden map[int64]string          // post id → who hid it ("flags" for the community)
	next   int64
}

func newMemStore() *memStore {
	s := &memStore{
		voters: map[int64]map[string]int8{},
		flags:  map[int64]map[string]int{},
		hidden: map[int64]string{},
		next:   1,
	}
	for _, md := range seedPosts {
		if html, ok := renderPost(md); ok {
			_, _ = s.add(context.Background(), "text", html, "", Meta{})
		}
	}
	return s
}

// fill copies a post with its moderation state filled. Callers hold s.mu.
func (s *memStore) fill(p Post) Post {
	_, p.Hidden = s.hidden[p.ID]
	p.FlagCount = len(s.flags[p.ID])
	return p
}

// find returns a pointer into s.posts, hidden or not. Callers hold s.mu.
func (s *memStore) find(id int64) *Post {
	for i := range s.posts {
		if s.posts[i].ID == id {
			return &s.posts[i]
		}
	}
	return nil
}

func (s *memStore) get(_ context.Context, id int64) (Post, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, hid := s.hidden[id]; hid {
		return Post{}, false
	}
	if p := s.find(id); p != nil {
		return s.fill(*p), true
	}
	return Post{}, false
}

func (s *memStore) add(_ context.Context, kind, bodyHTML, author string, m Meta) (Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Post{ID: s.next, Kind: kind, BodyHTML: bodyHTML, Author: author, Meta: m, CreatedAt: time.Now()}
	s.next++
	s.posts = append(s.posts, p)
	return p, nil
}

func (s *memStore) addImage(_ context.Context, mediaURL, cfImageID, author string, m Meta) (Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Post{ID: s.next, Kind: "image", ImageURL: mediaURL, CFImageID: cfImageID, Author: author, Meta: m, CreatedAt: time.Now()}
	s.next++
	s.posts = append(s.posts, p)
	return p, nil
}

func (s *memStore) vote(_ context.Context, postID int64, voter string, up bool) (Post, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := s.find(postID)
	if found == nil {
		return Post{}, false
	}
	if s.voters[postID] == nil {
		s.voters[postID] = map[string]int8{}
	}
	if s.voters[postID][voter] != 0 {
		return s.fill(*found), false
	}
	if up {
		s.voters[postID][voter] = 1
	} else {
		s.voters[postID][voter] = -1
	}
	return s.fill(*found), true
}

func (s *memStore) voteOf(_ context.Context, postID int64, voter string) int8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.voters[postID][voter]
}

// neighbors walks the ID-ordered slice past id in the asked direction,
// skipping hidden posts, and reports whether more remain past the batch.
func (s *memStore) neighbors(_ context.Context, id int64, newer bool, limit int) ([]Post, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Post
	if newer {
		// nearest above id = the smallest IDs past it; s.posts is ID-ascending
		for _, p := range s.posts {
			if _, hid := s.hidden[p.ID]; hid {
				continue
			}
			if p.ID > id {
				out = append(out, s.fill(p))
				if len(out) > limit {
					break
				}
			}
		}
	} else {
		for i := len(s.posts) - 1; i >= 0; i-- {
			p := s.posts[i]
			if _, hid := s.hidden[p.ID]; hid {
				continue
			}
			if p.ID < id {
				out = append(out, s.fill(p))
				if len(out) > limit {
					break
				}
			}
		}
	}
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

func (s *memStore) countByAuthor(_ context.Context, handle string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, p := range s.posts {
		if p.Author == handle {
			n++
		}
	}
	return n
}

// list returns the visible wall newest-first. The wall is chronological:
// reactions score people (tiers, invite chains), never placement — scores are
// hidden, and a fresh post always lands on top.
func (s *memStore) list(_ context.Context) []Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Post, 0, len(s.posts))
	for _, p := range s.posts {
		if _, hid := s.hidden[p.ID]; hid {
			continue
		}
		out = append(out, s.fill(p))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *memStore) flag(_ context.Context, postID int64, flagger string, flaggerTier int, instaTier, quorum int) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.find(postID) == nil {
		return false, false
	}
	fresh := false
	if s.flags[postID] == nil {
		s.flags[postID] = map[string]int{}
	}
	if _, dup := s.flags[postID][flagger]; !dup {
		s.flags[postID][flagger] = flaggerTier
		fresh = true
	}
	// the hiding rules: one trusted flag, or a quorum of any
	maxTier, count := 0, 0
	for _, t := range s.flags[postID] {
		count++
		if t > maxTier {
			maxTier = t
		}
	}
	if _, hid := s.hidden[postID]; !hid && (maxTier >= instaTier || count >= quorum) {
		s.hidden[postID] = "flags"
	}
	_, nowHidden := s.hidden[postID]
	return nowHidden, fresh
}

func (s *memStore) hide(_ context.Context, id int64, by string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.find(id) == nil {
		return errNoPost
	}
	if _, hid := s.hidden[id]; !hid {
		s.hidden[id] = by
	}
	return nil
}

func (s *memStore) restore(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.find(id) == nil {
		return errNoPost
	}
	delete(s.hidden, id)
	delete(s.flags, id)
	return nil
}

func (s *memStore) purge(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.posts {
		if s.posts[i].ID == id {
			s.posts = append(s.posts[:i], s.posts[i+1:]...)
			delete(s.voters, id)
			delete(s.flags, id)
			delete(s.hidden, id)
			return nil
		}
	}
	return errNoPost
}

func (s *memStore) flagged(_ context.Context) []Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Post
	for i := len(s.posts) - 1; i >= 0; i-- {
		p := s.posts[i]
		_, hid := s.hidden[p.ID]
		if hid || len(s.flags[p.ID]) > 0 {
			out = append(out, s.fill(p))
		}
	}
	return out
}

func (s *memStore) adminList(_ context.Context, limit int) []Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Post
	for i := len(s.posts) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.fill(s.posts[i]))
	}
	return out
}

// seedPosts gives a fresh boot a wall's worth of tiles. They double as a smoke
// test of the formatting subset (bold, italics, lists) and give the feed
// enough timeline that the infinite-scroll sentinels have something to pull.
var seedPosts = []string{
	"the tide came in **sideways** today",
	"things i saw at 3am:\n\n- a fox\n- a closed diner\n- *myself, reflected*",
	"_quietly posting into the void_",
	"**no gaps.** just squares, all the way down",
	"i keep a list of sounds the house makes\n\n1. the settling\n2. the pipe\n3. the *other* one",
	"someone left a **door** in the dunes. just the door.",
	"*the gulls have unionized*",
	"low tide pulled back far enough to show the old pier pilings\n\n- barnacled\n- patient",
	"**heard:** the fog comes in on little cat feet. *confirmed.*",
	"i counted the steps down to the water\n\n1. forty one\n2. forty two in shoes",
	"the lighthouse blinked twice tonight. it usually blinks once.",
	"_sand in everything i own and i regret nothing_",
	"**found:** one glove, leather, pointing north",
	"the vending machine by the boardwalk only takes *exact change* and only gives **advice**",
	"tonight the waves sound like\n\n- static\n- applause\n- static again",
	"someone wrote *hush* in the wet sand and the tide took it",
	"**day 9:** the heron still won't acknowledge me",
}
