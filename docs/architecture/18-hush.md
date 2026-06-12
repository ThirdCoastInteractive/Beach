# Architecture 18 — hush: gated anonymous board

[← docs index](../README.md) · prev: [Gaps](17-gaps.md) · related: [Examples](15-examples.md)

A fourth example app under `examples/hush`. The other three validate the
framework's core; hush validates that you can build **custom layouts and
client-side JS on top of a normal Beach app** without fighting the SSR grain. It
is a curated, anonymous content board — *hush-hush* by design: a gapless wall of
square posts, gated signup with manual approval, and feature tiers unlocked by
reputation.

It exercises two packages the trio doesn't: `storage` (post images + application
photos) and **Repute** (post scoring, which gates DMs and profiles).

Views live in `pages.templ` behind a **custom shell** (`templ shell`): hush links
the framework stylesheet (so `dw-*` surfaces still resolve) *and* its own
`static/css/hush.css`, and loads `datastar.js` but deliberately **not**
`chart.js` — proof that an app can replace the driftwood page chrome and skip
the chart interaction layer without losing the framework. The camera island is
the one other script (`static/js/camera.js`, an external module — CSP-clean).

## What it is

- The front page is one **gapless grid of square tiles** — every recent post, no
  gaps anywhere, text and image posts both filling perfect squares. Every tile is
  a real link: opening a post **morphs the square into a single-column feed**
  anchored on it, and the post's time-ordered neighbors stream in as you scroll
  either way. The user is on rails — the browser's own back button pops you out.
- Posting is **anonymous**. Accounts are gated, with three doors: you apply with
  a handle, a photo, and 3–5 lines about what you want to post (an admin approves
  or rejects); you claim one of **N open seats** while `HUSH_OPEN_SLOTS` has any
  left (the switch-on-later mode — once spent, the queue resumes); or you redeem
  a member's **invite link**, which makes your inviter's reputation answer for
  yours.
- A new account can **only post**. DMs and profile customization unlock as your
  **Repute** score rises. Reputation does real work: it's the gate.
- Text posts are **512 plain characters**, with a deliberately tiny markdown
  subset — bold, italics, lists. Nothing that controls layout.
- Image posts come from a **WebGPU camera**: open the camera, the live preview
  runs through WGSL filter shaders, you grab the filtered frame. Or upload an
  image and run it through the same shaders.

## What it showcases (and validates)

| Concern | How hush proves it |
| --- | --- |
| Custom layout on the SSR core | the gapless square grid — pure CSS, zero JS, container-query text scaling |
| Client-side JS as an island | the WebGPU camera: an isolated client that produces a blob and hands it to a normal action; everything else is SSR |
| `storage.Store` | post images and application photos via `Put`/`URL`/`ServeHTTP`, swappable Local↔R2 |
| Repute | event-sourced post scoring; score reads gate `app.Can("dm")` / `app.Can("profile:edit")` |
| `auth` gating | application → approval → user; tiered capabilities behind `app.Can` guards |
| `beach.Bind` / Patches / hub | every action is JSON-signal bound; new tiles arrive over SSE |

## Data model

```sql
-- The gate. One row per signup request.
applications (
  id          uuid primary key,
  handle      text unique not null,   -- desired anon handle
  pitch       text not null,          -- 3-5 lines: what you want to post
  photo_key   text not null,          -- storage.Store key, submitted proof
  status      text not null,          -- pending | approved | rejected
  created_at  timestamptz not null,
  decided_at  timestamptz,
  decided_by  uuid                    -- admin user id
);

-- Users exist only after approval. auth owns this table; hush references it.
-- Anonymous by default: a handle and nothing that identifies a person.
users (
  id          uuid primary key,
  handle      text unique not null,
  created_at  timestamptz not null
);

posts (
  id           uuid primary key,
  author_id    uuid not null references users,
  kind         text not null,         -- text | image
  body_md      text,                  -- raw markdown (<=512 plain chars)
  body_html    text,                  -- rendered + sanitized, cached
  media_key    text,                  -- storage.Store key for image posts
  created_at   timestamptz not null,
  score_cached double precision not null default 0  -- Repute, denormalized for ORDER BY
);

-- hush's domain table for votes: a real row with the one-vote-per-user rule.
-- Also the trigger to emit a Repute event (see below).
post_votes (
  id         bigserial primary key,
  post_id    uuid not null references posts,
  voter_id   uuid not null references users,
  weight     smallint not null,       -- +1 / -1
  created_at timestamptz not null,
  unique (post_id, voter_id)
);

-- DMs. Unlocked at tier 2.
conversations ( id uuid primary key, a_id uuid, b_id uuid, created_at timestamptz );
messages      ( id uuid primary key, conversation_id uuid, sender_id uuid, body text, created_at timestamptz );

-- Invites (built, 00002): one row per minted code. The inviter→used_by edge is
-- the chain-reputation graph; counting a member's rows enforces the allowance.
hush_invites ( code text primary key, inviter text, created_at timestamptz,
               used_by text, used_at timestamptz );

-- Signups ledger (built, 00002): accounts created outside the admin queue, so
-- the open-registration cap can count what it has spent.
hush_signups ( id bigserial primary key, handle text, via text, created_at timestamptz );
```

`post_votes` is hush's domain table — it owns the "one vote per user per post"
constraint. **Repute owns the math** on its own append-only log
(`eventsource/pg`): on each vote insert and each post create, hush also calls
`eng.Apply(...)`. `score_cached` is Repute's answer folded back onto the post row
so the grid orders tiles without asking the engine per tile.

## Repute wiring

One engine per process, installed at boot over the same pool:

```go
eng := repute.New(
    []repute.Scorer{
        scorers.Participation{}, // rewards posting at all — unlocks tier 1
        scorers.WeightedWin{},   // up/down votes as wins/losses
    },
    repute.WithDecay(...),       // recent activity weighted heavier
    repute.WithStore(pgsource),  // eventsource/pg over the app pool
)
```

Three event kinds, all on track `author`:

- `"post"` — emitted on create. Feeds Participation: you reach tier 1 just by
  contributing.
- `"like"` / `"dislike"` — emitted on each `post_votes` insert by `weight`
  (+1 → like, −1 → dislike). A 👎 scores negative, so standing tracks the
  author's like/dislike ratio, not raw volume.

Score for gating comes from `eng.Leaderboard("author", asOf)` → the author's
reputation vector. Tiers:

| Tier | Reached when | Capability |
| --- | --- | --- |
| 0 | account approved | post only |
| 1 | score ≥ `T1` | customize profile (the custom-layout showcase) |
| 2 | score ≥ `T2` | DMs |

Gating is a `func (a *app) tier(ctx, userID) int` helper backing
`app.Can("profile:edit")` and `app.Can("dm")` guards — the same `app.Can` seam
the pantry uses, so the capability story stays visible in the route table.

### Chain reputation — invites answer for their invitees

A second track, `chain`, scores the company you bring. Every invite redemption
records an edge (inviter → invitee); from then on, each 👍 on an invitee's
post also emits a half-weight `chain-like` (+1) to the inviter, each 👎 a
`chain-dislike` (−1). The chain score then **throttles the invite allowance**:

```go
allowance = clamp(inviteBase + chain/2, 0, inviteMax)   // base 3, max 6
```

Invite shitheads and your allowance dries up; invite people whose posts land
and it grows. The math lives in `inviteAllowance` (invites.go), the echo in
`votePost`, and the edges in `hush_invites` — Postgres-persistent, hydrated
into memory at boot, so chains survive restarts even while scores (in-memory,
like the wall) reset. One hop for now; propagating a decayed echo further up
the chain is the obvious extension.

`T1`, `T2`, and the decay half-life are config. Repute's go.mod is local and
unpublished like the-beach, so hush's go.mod carries
`replace github.com/ThirdCoastInteractive/Repute => ../../../repute` (or an
absolute dev path).

## The text path — 512 plain chars, tiny markdown

goldmark renders the subset; bluemonday both sanitizes and gives us the plain
text to count. Layout markup simply isn't in the allow-list, so there are no
extra validation rules to maintain.

```go
var mdPolicy = bluemonday.NewPolicy().
    AllowElements("em", "strong", "ul", "ol", "li", "p", "br") // bold, italics, lists. nothing else.

// render sanitizes raw markdown to safe HTML and reports whether it fits in 512
// plain runes. Any UTF-8 is accepted and rendered as far as it can be.
func render(raw string) (html string, ok bool) {
    var buf bytes.Buffer
    _ = goldmark.Convert([]byte(raw), &buf)
    safe := mdPolicy.SanitizeBytes(buf.Bytes())                 // drop headings/links/img/etc.
    plain := strings.TrimSpace(bluemonday.StrictPolicy().Sanitize(string(safe)))
    if n := utf8.RuneCountInString(plain); n == 0 || n > 512 {
        return "", false
    }
    return string(safe), true
}
```

The count is on stripped plain text, so `**bold**` costs 4 characters, not 8.

## The camera — WebGPU filter pipeline

A client-side island that produces a blob and hands it to a normal action.
Everything around it is SSR; this is the "client JS on top of Beach" showcase.

Pipeline (`examples/hush/static/js/camera.js`), adapted from the m13 audition
recorder but **still capture, not video, and WebGPU, not WebGL**:

1. `navigator.mediaDevices.getUserMedia({ video })` → hidden `<video>` preview.
   No audio track.
2. Each frame is imported as a WebGPU external texture and drawn through a
   **WGSL render pipeline**: LUT + brightness/contrast/saturation/temperature/
   grain/vignette, the recorder's preset set translated GLSL→WGSL. Output to a
   canvas.
3. **`grab()`** replaces the recorder's start/stop: `canvas.toBlob('image/jpeg',
   0.9)` of the filtered canvas. The shader already modified the frame live, so
   the filtered canvas *is* the modified still — no post-step. This is "capture
   the raw image, then modify": modification happens in the live pipeline.
4. Upload: `POST /posts` hands the blob to a normal action →
   `store.Put(ctx, "posts/<id>.jpg", blob, "image/jpeg")` → the tile renders from
   `store.URL(...)`.

The **upload-and-edit** path feeds an uploaded file into the *same* WebGPU
pipeline as a texture, so camera and editor share one set of WGSL shaders — one
island, two entry points.

WGSL gives the compute path room to grow filters beyond per-pixel — separable
blur/bokeh, histogram tone-mapping, 3D film LUTs — without changing `grab()` or
the upload flow, which sit above the renderer.

Feature detection is a one-liner: if `navigator.gpu` is absent the camera button
is disabled and plain image upload (unfiltered) still works, so a device without
WebGPU can still post — it just doesn't get the live filters.

## The gapless square grid

Pure CSS, no JS. The showcase layout.

```css
.hush-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 0;                                   /* no gaps. anywhere. */
}
.hush-tile { aspect-ratio: 1 / 1; overflow: hidden; position: relative; }
.hush-tile img { width: 100%; height: 100%; object-fit: cover; }
.hush-tile--text {                          /* text posts fill the square too */
  display: grid; place-items: center; padding: 1rem;
  container-type: inline-size;              /* body scales to the tile */
}
.hush-tile--text p { font-size: clamp(0.8rem, 6cqi, 1.4rem); }
```

`auto-fill` + `gap:0` + `aspect-ratio:1/1` gives the seamless wall; container
queries scale text bodies to fill their square instead of leaving dead space.
The wall is **chronological** — newest first, text and image posts alike;
reactions score people (tiers, invite chains), never placement, so a fresh
post always lands on top. New posts arrive as a prepended tile fragment over
a hub topic (the boardwalk SSE pattern).

## The feed (✅ built)

Opening a post is a **real navigation**: every tile links to `/p/{id}#p{id}`,
so the browser owns push/pop state — back pops you to the grid, forward
replays, and permalinks are just URLs. The post's page is a single centered
**feed**: the focused post expanded into its detail card — media, meta, the
thumbs, and nothing else; **there are no replies**, a post stands alone and
the only answer the wall allows is a thumb — with its **time-ordered
neighbors** around it, newer above, older below, newest-first all the way
down. The link's `#p{id}`
fragment lands the viewport on the focused card even with newer neighbors
rendered above it (native scroll-to-fragment, no script). The user is on
rails: neighbor cards are links to their own pages, the title is the way back
to the wall, and that's every control there is.

The feed is **infinite both ways**. The page pre-renders `feedSpan` neighbors
per direction; past each end sits an invisible sentinel
(`data-on-intersect__once`, the Defer pattern) whose `@get` hits
`/feed/newer|older?cursor={id}` — post IDs are monotonic, so the ID is the
timestamp cursor (a `kind=` filter would ride the same query string). The
Datastar-only handler replaces the sentinel (`PatchReplace`) with the next
`feedChunk` cards under a `display: contents` wrapper, plus a fresh sentinel
while timeline remains — the feed extends itself until both ends run dry, and
the sentinels remove themselves.

Navigations are **cross-document View Transitions with no crossfade and no
pop-in** — no script, just names. `@view-transition { navigation: auto }` opts
both documents in, and the naming strategy differs by page. On the **grid**
only the clicked tile is named (`data-vt="hush-post-<id>"`, lifted into a real
`view-transition-name` by a `:focus` rule with typed `attr()` — a clicked link
holds focus while the navigation snapshots), so leaving the wall morphs one
hero square, not seventeen. On **feed pages** every card is named inline:
consecutive posts share most of their neighbors, so a feed→feed step pairs the
overlap and the column **slides** to its new positions instead of reloading.
The chrome (`.hush-head`, `.hush-acct`) is named too, so it holds still across
pages. Everything else moves as two coherent planes: the old page — root plus
any old-only named pieces, all riding the same animation — paints on top
(`z-index`) and is shoved outward past the edges (`scale(1.1)`) as the hero
expands into place, while the next page stands ready beneath with **no
entrance animation** (`::view-transition-new(*):only-child { animation: none }`
— nothing fades in, it is revealed). On a back-pop the old page contracts away
instead (`:active-view-transition-type(backwards)`), so going back feels like
coming back. `prefers-reduced-motion` drops it all to an instant swap, and
browsers without cross-document transitions or typed `attr()` just navigate —
graceful degradation, no error.


## Routes

```
GET  /                 grid    -- gapless square grid, scored + recent
GET  /apply            page    -- claim-a-seat form while open seats remain; else the application form
POST /apply            action  -- write applications row (status=pending)
POST /register         action  -- claim one open seat (HUSH_OPEN_SLOTS), create account, sign in
GET  /invite/{code}    page    -- redemption form for a live code; notice for spent/unknown
POST /invite/{code}    action  -- redeem: claim code, create account, record chain edge, sign in
POST /invites          action  -- mint one code (member, inside the chain-throttled allowance)
GET  /admin/queue      page    -- pending applications            [admin]
POST /admin/decide     action  -- approve → create user; or reject [admin]
GET  /compose          page    -- markdown box OR camera/upload
GET  /p/{id}           page    -- the feed, focused on one post (neighbors both ways)
GET  /feed/newer       frag    -- next feed batch above cursor (intersect sentinel only)
GET  /feed/older       frag    -- next feed batch below cursor (intersect sentinel only)
POST /p/{id}/vote      action  -- record a 👍/👎 (?dir=up|down), emit Repute "like"/"dislike", outer-morph your mark
POST /posts            action  -- bind, render+count, store, emit Repute "post"
GET  /u/{handle}       page    -- profile, tier-1 gated
GET  /dm               page    -- conversations                   [tier 2]
POST /dm/{conv}/say    action  -- message                         [tier 2]
GET  /files/           raw     -- storage.Local.ServeHTTP (post images, application photos)
GET  /specimen         page    -- the framework specimen (mounted by every example)
```

All actions are `beach.Bind[T]` + `beach.Patches` (Datastar JSON signals), matching
the pantry corrections. Guards are `app.Can(...)` at registration so the
capability story reads off the route table.

## Build blocks

Gated like the [roadmap](../ROADMAP.md): each block ends in something you can run
and test in a real browser (compiling and serving 200 is not done).

**Block A — board skeleton. ✅ built.** In-memory post store (database-less, like
boardwalk — zero setup to run; Postgres lands with the gate in Block D). The
gapless grid renders, seeded with a few posts. The text path (`renderPost`) is
wired into `POST /posts`, which prepends the new tile and clears the composer.
The composer lives inline on `/` (the dedicated `/compose` route is reserved for
the camera/upload island in Block C). ◆ Gate (verified in-browser): post text and
watch it appear as a square tile; bold/italics/lists render; the 512-character
limit and the empty post are both rejected with an inline notice.

**Block B — storage + images. ✅ built.** `storage.Local` rooted under the OS temp
dir, served at `/files` via `ServeHTTP`. Image posts come from a plain multipart
upload form (303 back to the wall — CSP-clean, no Datastar). ◆ Gate (verified
in-browser): upload an image, it `Put`s to the store, serves from `/files`, and
renders as an image tile in the gapless wall.

**Block C — WebGPU camera. ✅ built (capture needs a real camera to verify).** The
`camera.js` island on `/compose`: `getUserMedia` → a WGSL render pipeline
(brightness/contrast/saturation/temperature + presets none/noir/bleach/dream over
a `texture_external`) → `canvas.toBlob` → `POST /upload` (the Block B endpoint) →
back to the wall. Feature-detects `navigator.gpu` and falls back to the plain
upload. ◆ Gate: the page renders, CSS + WebGPU init verified in-browser; the live
filtered capture is verified on a device with an actual camera.

**Block D — the gate. ✅ built (verified against Postgres).** DB-optional, like
pantry: set `DATABASE_URL` and the gate turns on; without it the wall runs open.
`hush_applications` migration + the framework's session/auth migrations. `/apply`
(handle, password, pitch, optional photo → `storage`) writes a pending row;
`/admin/queue` + `/admin/decide` (admin-gated) approve → create an auth `users`
row + local credential + `member` role, or reject. Plain-form `/login` /
`/logout` (303, CSP-clean). Posting (`/posts`, `/upload`, voting) is guarded by
`app.Can("post")`; the composer only renders for users who hold it,
and an account strip shows who you are + the admin queue link. Seeds an
`admin / password` account. The post wall itself stays in-memory (like pantry's
domain data) — persisting posts to Postgres is the remaining follow-up. ◆ Gate
(verified in-browser): logged-out wall is read-only with an invite prompt → apply
as `moth` → sign in as `admin`, approve in the queue → sign out, sign in as `moth`
→ post lands on the wall.

**Block E — Repute + tiers. ✅ built (verified against Postgres).** Repute is a
normal dependency (`github.com/ThirdCoastInteractive/Repute`, private). A
mutex-wrapped engine with three `Participation` scorers folds `post` (+1),
`like` (+2), and `dislike` (−2) events onto an `author` track — a post credits
its author, a 👍 credits the post's author, a 👎 dings them, so standing tracks
the like/dislike ratio rather than raw volume. `reputation.score(handle)` reads
standing; `tierOf(score)` maps it to tiers (T1=3, T2=8). **Every score is
hidden.** The focused feed card carries the thumbs control (`ui.Icon`
`thumb-up`/`thumb-down` glyphs — the framework icon system, no emoji) but no
counts and no ratio, ever; the only readable state is your own mark, which
inverts once you've reacted (one reaction per user, the first is final;
outer-morph). The wall is chronological. Standing surfaces only as **what has
unlocked**: the profile link appearing in the account strip is tier 1
announcing itself, the tier-1 profile (`/u/{handle}`) shows a post count and
the DM line, and the invite allowance quietly grows or shrinks with the chain.
The engine is in-memory (matching the in-memory wall); the `eventsource/pg`
adapter is the persistent path once posts move to Postgres. ◆ Gate (verified
in-browser): sign in as `moth`, post (score 1), get 👍'd (score 3) → tier 1 →
the profile link appears and `/u/moth` renders.

**Block F — the other doors: open seats, invites, chain reputation. ✅ built
(verified against Postgres).** `HUSH_OPEN_SLOTS=N` opens the first N
registrations with no review — the wall's gate line advertises the seats, the
`/apply` page becomes a claim form (handle + password → account + session in
one POST, the seat claimed by a cap-guarded ledger insert), and when the seats
are spent the application queue resumes by itself. Tier-1+ members mint invite
codes from their profile (allowance = `inviteBase + chain/2`, clamped 0–6);
`/invite/{code}` redeems one atomically into an account and records the chain
edge. From then on the invitee's 👍/👎 echo half-weight onto the inviter's
`chain` track — the throttle. ◆ Gate (verified in-browser): with 2 seats, claim
one as `moth` → post, get 👍'd to tier 1 → mint `/invite/…` → redeem as `crow`
→ crow posts twice, both 👎'd → moth's chain drops to −2 and the invite panel
reads "1 of 2 minted" where it read "1 of 3" (the chain score itself is never
shown — the shrinking allowance is how you find out); `gull` claims the last
seat, `/apply` falls back to the application form, and the spent invite link
reports itself used.

Each block is verified in a real browser before the next — the lesson from the
trio: a rendered page is not a working feature.

## Running it

Locally (`go run`): copy `examples/hush/.env.example` to `.env` and point
`DATABASE_URL` at a Postgres (or unset it to run open/database-less). Building the
private Repute dep needs `GOPRIVATE=…,github.com/ThirdCoastInteractive`. Or use
the self-contained `examples/hush/docker-compose.yml` — `docker compose build
--ssh default && docker compose up` (the SSH agent is forwarded so the image build
can fetch Repute; see the [Dockerfile](../../Dockerfile)). Each example carries its
own `docker-compose.yml` + `.env.example`.
