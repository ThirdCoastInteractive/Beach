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
	Votes     int    // filled on read from the votes map
	CreatedAt time.Time
}

// Comment is one reply under a post. Like a post, its BodyHTML is sanitized by
// renderPost — never raw input.
type Comment struct {
	ID        int64
	PostID    int64
	BodyHTML  string
	CreatedAt time.Time
}

// store is the in-memory post wall. Block A runs database-less so the app is
// explorable with zero setup, like boardwalk; the Postgres-backed store lands
// with the signup gate (Block D). Posts render newest-first; comments oldest-first
// under their post.
type store struct {
	mu       sync.Mutex
	posts    []Post
	comments map[int64][]Comment
	votes    map[int64]int             // post id → upvote count
	voters   map[int64]map[string]bool // post id → set of handles that voted
	next     int64
	nextC    int64
}

func newStore() *store {
	s := &store{
		comments: map[int64][]Comment{},
		votes:    map[int64]int{},
		voters:   map[int64]map[string]bool{},
		next:     1,
		nextC:    1,
	}
	for _, md := range seedPosts {
		if html, ok := renderPost(md); ok {
			s.add("text", html, "")
		}
	}
	// A couple of seed replies so a freshly opened card isn't empty.
	if first := s.list(); len(first) > 0 {
		newest := first[0].ID
		if html, ok := renderPost("*welcome to the wall.*"); ok {
			s.addComment(newest, html)
		}
		if html, ok := renderPost("click anything. it opens."); ok {
			s.addComment(newest, html)
		}
	}
	return s
}

// get returns the post with the given id.
func (s *store) get(id int64) (Post, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.posts {
		if p.ID == id {
			p.Votes = s.votes[id]
			return p, true
		}
	}
	return Post{}, false
}

// listComments returns a post's comments (oldest-first) as a copy.
func (s *store) listComments(postID int64) []Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.comments[postID]
	out := make([]Comment, len(src))
	copy(out, src)
	return out
}

// addComment appends a comment to a post and returns it.
func (s *store) addComment(postID int64, bodyHTML string) Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	cm := Comment{ID: s.nextC, PostID: postID, BodyHTML: bodyHTML, CreatedAt: time.Now()}
	s.nextC++
	s.comments[postID] = append(s.comments[postID], cm)
	return cm
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

// vote records an upvote of postID by voter (one per voter per post) and returns
// the new count, the post's author (for scoring), and whether the vote was new.
func (s *store) vote(postID int64, voter string) (count int, author string, ok bool) {
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
		return 0, "", false
	}
	if s.voters[postID] == nil {
		s.voters[postID] = map[string]bool{}
	}
	if s.voters[postID][voter] {
		return s.votes[postID], found.Author, false
	}
	s.voters[postID][voter] = true
	s.votes[postID]++
	return s.votes[postID], found.Author, true
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

// list returns posts ranked by votes then recency, as a copy with Votes filled.
func (s *store) list() []Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Post, len(s.posts))
	for i, p := range s.posts {
		p.Votes = s.votes[p.ID]
		out[i] = p
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Votes != out[j].Votes {
			return out[i].Votes > out[j].Votes // most-upvoted first
		}
		return out[i].ID > out[j].ID // then newest
	})
	return out
}

// seedPosts gives a fresh boot a few tiles so the wall isn't empty. They double
// as a smoke test of the formatting subset: bold, italics, and a list.
var seedPosts = []string{
	"the tide came in **sideways** today",
	"things i saw at 3am:\n\n- a fox\n- a closed diner\n- *myself, reflected*",
	"_quietly posting into the void_",
	"**no gaps.** just squares, all the way down",
	"i keep a list of sounds the house makes\n\n1. the settling\n2. the pipe\n3. the *other* one",
}
