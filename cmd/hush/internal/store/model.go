// The shared post type and the boot seed. The store itself is Postgres-only
// (pgstore.go) — hush requires a database, so there's no second implementation
// to abstract over.
package store

import (
	"errors"
	"time"
)

// Post is one print on the wall. A text post carries sanitized BodyHTML (from
// content.RenderPost — never raw user input); an image post carries ImageURL,
// the public URL its bytes were stored at, plus CFImageID when the upload went
// through Cloudflare Images (the media helpers pick the right source per
// surface). A layout post carries Layout (the studio composition as JSON) and
// still fills ImageURL/CFImageID from its first piece so admin thumbs work.
// Pin* is the print's place on the shared board; HasPin is false until someone
// drags it, and the renderer then scatters from the id. Hidden, FlagCount, and
// Meta exist for moderation — admin eyes only; templates never render them.
type Post struct {
	ID        int64
	Kind      string // "text" | "image" | "layout"
	BodyHTML  string // text posts
	ImageURL  string // image posts, or a layout's first piece
	CFImageID string // image posts / first layout piece ("" when stored via files)
	Layout    []byte // studio composition JSON; "{}" for text/image
	HasPin    bool
	PinX      float64
	PinY      float64
	PinRot    float64
	PinW      float64
	Author    string // handle that posted it (for Repute scoring; never shown — the wall is anon)
	Hidden    bool   // moderated off the wall; permalink 404s for non-admins
	Deleted   bool   // purged (soft) — gone everywhere but the admin browser
	FlagCount int    // filled on admin reads
	Meta      Meta   // capture-time request context; admin eyes only
	CreatedAt time.Time
}

// ErrNoPost is the store's answer to moderating a post that isn't there.
var ErrNoPost = errors.New("hush: no such post")

// SeedPosts gives a fresh boot a wall's worth of tiles. They double as a smoke
// test of the formatting subset (bold, italics, lists) and give the feed
// enough timeline that the infinite-scroll sentinels have something to pull.
var SeedPosts = []string{
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
