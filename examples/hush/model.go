package main

import (
	"sort"
	"sync"
	"time"
)

// Post is one tile on the wall. A text post carries sanitized BodyHTML (from
// renderPost — never raw user input); an image post carries ImageURL, the public
// URL its bytes were stored at via storage.Store.
type Post struct {
	ID        int64
	Kind      string // "text" | "image"
	BodyHTML  string // text posts
	ImageURL  string // image posts
	Author    string // handle that posted it (for Repute scoring; never shown — the wall is anon)
	Likes     int    // filled on read from the reaction maps; never shown — scores are hidden
	Dislikes  int
	CreatedAt time.Time
}

// store is the in-memory post wall. Block A runs database-less so the app is
// explorable with zero setup, like boardwalk; the Postgres-backed store lands
// with the signup gate (Block D). Posts render newest-first. There are no
// replies — a post stands alone; the only answer the wall allows is a thumb.
type store struct {
	mu       sync.Mutex
	posts    []Post
	likes    map[int64]int             // post id → 👍 count
	dislikes map[int64]int             // post id → 👎 count
	voters   map[int64]map[string]int8 // post id → handle → reaction (+1/−1; one, final)
	next     int64
}

func newStore() *store {
	s := &store{
		likes:    map[int64]int{},
		dislikes: map[int64]int{},
		voters:   map[int64]map[string]int8{},
		next:     1,
	}
	for _, md := range seedPosts {
		if html, ok := renderPost(md); ok {
			s.add("text", html, "")
		}
	}
	return s
}

// react fills a post's reaction counts from the maps. Callers hold s.mu.
func (s *store) react(p Post) Post {
	p.Likes = s.likes[p.ID]
	p.Dislikes = s.dislikes[p.ID]
	return p
}

// get returns the post with the given id.
func (s *store) get(id int64) (Post, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.posts {
		if p.ID == id {
			return s.react(p), true
		}
	}
	return Post{}, false
}

// add appends a text post by author and returns it.
func (s *store) add(kind, bodyHTML, author string) Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Post{ID: s.next, Kind: kind, BodyHTML: bodyHTML, Author: author, CreatedAt: time.Now()}
	s.next++
	s.posts = append(s.posts, p)
	return p
}

// addImage appends an image post by author pointing at a stored object's URL.
func (s *store) addImage(imageURL, author string) Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Post{ID: s.next, Kind: "image", ImageURL: imageURL, Author: author, CreatedAt: time.Now()}
	s.next++
	s.posts = append(s.posts, p)
	return p
}

// vote records voter's reaction to postID — a 👍 (up) or a 👎 — and returns the
// post with fresh counts plus whether the reaction was new. One reaction per
// voter per post; the first one is final.
func (s *store) vote(postID int64, voter string, up bool) (Post, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var found *Post
	for i := range s.posts {
		if s.posts[i].ID == postID {
			found = &s.posts[i]
			break
		}
	}
	if found == nil {
		return Post{}, false
	}
	if s.voters[postID] == nil {
		s.voters[postID] = map[string]int8{}
	}
	if s.voters[postID][voter] != 0 {
		return s.react(*found), false
	}
	if up {
		s.voters[postID][voter] = 1
		s.likes[postID]++
	} else {
		s.voters[postID][voter] = -1
		s.dislikes[postID]++
	}
	return s.react(*found), true
}

// voteOf returns voter's recorded reaction to postID: +1, −1, or 0 for none.
func (s *store) voteOf(postID int64, voter string) int8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.voters[postID][voter]
}

// neighbors returns up to limit posts adjacent to id in time order — the
// newer side (IDs above it) or the older side (IDs below) — plus whether more
// remain past them. Results come back newest-first, the feed's display order,
// with Votes filled. IDs are assigned monotonically, so they are the
// timestamp cursor.
func (s *store) neighbors(id int64, newer bool, limit int) ([]Post, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Post
	if newer {
		// nearest above id = the smallest IDs past it; s.posts is ID-ascending
		for _, p := range s.posts {
			if p.ID > id {
				out = append(out, s.react(p))
				if len(out) > limit {
					break
				}
			}
		}
	} else {
		for i := len(s.posts) - 1; i >= 0; i-- {
			if p := s.posts[i]; p.ID < id {
				out = append(out, s.react(p))
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

// countByAuthor returns how many posts a handle has authored.
func (s *store) countByAuthor(handle string) int {
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

// list returns the wall newest-first, as a copy with counts filled. The wall
// is chronological: reactions score people (tiers, invite chains), never
// placement — scores are hidden, and a fresh post always lands on top.
func (s *store) list() []Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Post, len(s.posts))
	for i, p := range s.posts {
		out[i] = s.react(p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
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
