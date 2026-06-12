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

## What it is

- The front page is one **gapless grid of square tiles** — every recent post, no
  gaps anywhere, text and image posts both filling perfect squares.
- Posting is **anonymous**. Accounts are gated: you apply with a handle, a photo,
  and 3–5 lines about what you want to post; an admin approves or rejects.
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

Two event kinds, both on track `author`:

- `"post"` — emitted on create. Feeds Participation: you reach tier 1 just by
  contributing.
- `"vote"` — emitted on each `post_votes` insert, `weight` from the vote.

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

Pipeline (`examples/hush/static/camera.js`), adapted from the m13 audition
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
Ordering is `score_cached DESC, created_at DESC`. New posts arrive as a prepended
tile fragment over a hub topic (the boardwalk SSE pattern).

## The detail sheet + comments (✅ built)

Clicking any tile streams a **sheet** in from the right edge — a fresh page over
the (undimmed) wall, on top, no backdrop. The slide is a **CSS View Transition**,
not a transform/opacity tween: `.hush-sheet` carries `view-transition-name:
hush-sheet`, and the SSE patch that adds it runs inside `startViewTransition`, so
`::view-transition-old/new(hush-sheet)` keyframes do the animation.

The wiring is a small framework feature: `beach.View`/`beach.Patch` gained a
`ViewTransition bool`, which maps to the Datastar SDK's `WithViewTransitions()`
and emits the `useViewTransition true` SSE dataline; the client then applies the
patch inside the View Transitions API. The tile click is just `$reply = '';
@get('/p/{id}')` — no `open` signal, no backdrop. Close is a second SSE round-trip
(`@get('/sheet/close')`) that empties the host inside a View Transition, sliding
the sheet back out. (Both Datastar view-transition paths gate on element-scoped
`startViewTransition` support; in browsers without it the patch applies instantly
— graceful degradation, no error.)

`GET /p/{id}` is the dual View: a Datastar request patches the sheet in; a direct
navigation renders the same card as a **permalink page** (`/p/5`), so every post
is linkable (the `#id` in the card meta). One handler, both surfaces.

Comments live under the card. `POST /p/{id}/comments` binds the shared `reply`
signal, runs it through the same `renderPost` text path (512-char limited
markdown), appends the rendered comment to `#comments-{id}` (`PatchAppend`, each
new comment eases in via a keyframe), and clears the box with a `Signals` patch.
In-memory for now; it moves to Postgres with the gate in Block D.

## Routes

```
GET  /                 grid    -- gapless square grid, scored + recent
GET  /apply            page    -- handle, pitch, photo (camera or upload)
POST /apply            action  -- write applications row (status=pending)
GET  /admin/queue      page    -- pending applications            [admin]
POST /admin/decide     action  -- approve → create user; or reject [admin]
GET  /compose          page    -- markdown box OR camera/upload
GET  /p/{id}           page    -- post detail: drawer card (Datastar) OR permalink page (nav)
POST /p/{id}/comments  action  -- bind reply, render+count, append to thread, clear box
POST /p/{id}/vote      action  -- record upvote, emit Repute "vote", outer-morph the count
POST /posts            action  -- bind, render+count, store, emit Repute "post"
GET  /u/{handle}       page    -- profile, tier-1 gated
GET  /dm               page    -- conversations                   [tier 2]
POST /dm/{conv}/say    action  -- message                         [tier 2]
GET  /files/           raw     -- storage.Local.ServeHTTP (post images, application photos)
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
`/logout` (303, CSP-clean). Posting (`/posts`, `/upload`, comments) is guarded by
`app.Can("post")`; the composer and reply box only render for users who hold it,
and an account strip shows who you are + the admin queue link. Seeds an
`admin / password` account. The post wall itself stays in-memory (like pantry's
domain data) — persisting posts to Postgres is the remaining follow-up. ◆ Gate
(verified in-browser): logged-out wall is read-only with an invite prompt → apply
as `moth` → sign in as `admin`, approve in the queue → sign out, sign in as `moth`
→ post lands on the wall.

**Block E — Repute + tiers. ✅ built (verified against Postgres).** Repute is a
normal dependency (`github.com/ThirdCoastInteractive/Repute`, private). A
mutex-wrapped engine with two `Participation` scorers folds `post` (+1) and
`vote` (+2) events onto an `author` track; a post credits its author, an upvote
credits the post's author. `reputation.score(handle)` reads standing;
`tierOf(score)` maps it to tiers (T1=3, T2=8). The drawer card carries an upvote
control (members only, one vote per user, outer-morph the count); the wall orders
by votes then recency. The account strip shows live `score · tier`, and the
tier-1 profile (`/u/{handle}`) is gated — below tier 1 it shows a locked notice,
at tier 1+ it shows score + post count + the tier-2 (DMs) teaser. The engine is
in-memory (matching the in-memory wall); the `eventsource/pg` adapter is the
persistent path once posts move to Postgres. ◆ Gate (verified in-browser): sign
in as `moth`, post (score 1), open it and upvote (score 3) → tier 1 → the profile
link unlocks and `/u/moth` renders the stats.

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
