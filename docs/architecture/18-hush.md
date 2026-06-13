# Architecture 18 — hush: gated anonymous board

[← docs index](../README.md) · prev: [Gaps](17-gaps.md) · related: [Examples](15-examples.md)

A fourth example app under `examples/hush`. The other three validate the
framework's core; hush validates that you can build **custom layouts and
client-side JS on top of a normal Beach app** without fighting the SSR grain —
and, as of Block G, that a Beach app can carry a **full production build-out**:
Postgres-backed content, community moderation, transactional email, ClickHouse
analytics, and Cloudflare media. It is a curated, anonymous content board —
*hush-hush* by design: a gapless wall of square posts, gated signup with manual
approval, and feature tiers unlocked by reputation.

It exercises the packages the trio doesn't: `storage` (Local, R2 via
`storage/s3`, and `storage/cfimages`), `mailer` (the outbox's transport),
`ch` (the analytics firehose + admin charts), and **Repute** (post scoring,
which gates profiles and invites).

Views live in one `.templ` file per surface — `shell` (document + scaffolding),
`wall`, `feed`, `gate`, `profile`, `camera`, `terms`, `adminviews`,
`analyticsviews`, `emails` — behind a **custom shell** (`templ shell`): hush links
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
- A new account can **only post**. The profile (and with it the invite mint)
  unlocks as your **Repute** score rises. Reputation does real work: it's the
  gate.
- Text posts are **512 plain characters**, with a deliberately tiny markdown
  subset — bold, italics, lists. Nothing that controls layout.
- Image posts come from a **WebGPU camera**: open the camera, the live preview
  runs through WGSL filter shaders, you grab the filtered frame. Or upload an
  image and run it through the same shaders.
- The community moderates itself by **flagging** — quietly. A flag from a
  trusted member hides a post alone; the untrusted crowd hides one by
  agreement. What a flag did is never shown; the admin suite is where hidden
  posts go to be judged.

## What it showcases (and validates)

| Concern | How hush proves it |
| --- | --- |
| Custom layout on the SSR core | the gapless square grid — pure CSS, zero JS, container-query text scaling |
| Client-side JS as an island | the WebGPU camera: an isolated client that produces a blob and hands it to a normal action; everything else is SSR |
| `storage.Store` | post images and application photos via `Put`/`URL`/`ServeHTTP`, swappable Local↔R2 (`storage/s3`) by env alone |
| `storage/cfimages` | image posts upload to Cloudflare Images and render resized variants, falling back to the file store when unconfigured |
| `mailer` + an outbox | transactional email as Postgres rows, pumped through whatever transport the env supports (Mailgun → SMTP → log) |
| `ch` | the `hush_events` firehose, batched in off the request path, aggregated into deferred admin chart widgets |
| Repute | event-sourced post scoring; score reads gate the profile and throttle invites |
| `auth` gating | application → approval → user; tiered capabilities behind `app.Can` guards |
| `beach.Bind` / Patches / hub | every action is JSON-signal bound; new tiles arrive over SSE |

## Data model

In database mode (`DATABASE_URL` set) everything lives in Postgres — the wall
included. The schema lands in four migrations (`examples/hush/migrations`),
each shape shown here as the code reads it:

```sql
-- 00001: the gate. One pending row per signup request; the chosen password is
-- held hashed until approval creates the auth user from it.
-- 00004 added email: an optional contact — where the decision notice goes,
-- and the address that follows an approved applicant in. Admin eyes only.
hush_applications (
  id            bigserial primary key,
  handle        text not null,
  pitch         text not null,                     -- 3-5 lines: what you want to post
  password_hash text not null,
  photo_key     text not null default '',          -- storage key; resolved to a URL at read time
  email         text not null default '',
  status        text not null default 'pending',   -- pending | approved | rejected
  created_at    timestamptz not null default now(),
  decided_at    timestamptz
);

-- users / roles / sessions: the framework's auth + session migrations own
-- these tables; hush references members by username (the handle).

-- 00003: the wall. body_html is already sanitized (renderPost) — never raw
-- user input. ip/country/city/user_agent are the capture-time Meta (meta.go):
-- they ride every row for moderation, and templates never render them — the
-- wall stays anonymous. hidden_at is the moderation switch: a hidden post
-- stays (with its flags) for admin review, invisible to everyone else;
-- hidden_by records who threw it ('flags' for the community).
hush_posts (
  id          bigserial primary key,               -- monotonic: doubles as the feed cursor
  kind        text not null,                       -- text | image
  body_html   text not null default '',
  media_url   text not null default '',
  cf_image_id text not null default '',            -- Cloudflare Images id; '' when stored via files
  author      text not null default '',            -- handle; '' in database-less open mode
  ip          text not null default '',
  country     text not null default '',
  city        text not null default '',
  user_agent  text not null default '',
  created_at  timestamptz not null default now(),
  hidden_at   timestamptz,
  hidden_by   text not null default ''
);
-- index (hidden_at, id DESC): the wall's one read shape — visible, newest first

-- One reaction per voter per post, the first is final — the PK is the rule
-- (INSERT ... ON CONFLICT DO NOTHING makes a repeat click a no-op).
hush_reactions (
  post_id    bigint not null references hush_posts on delete cascade,
  voter      text not null,
  weight     smallint not null,                    -- +1 / -1
  created_at timestamptz not null default now(),
  primary key (post_id, voter)
);

-- One flag per flagger per post. flagger_tier is captured at flag time — what
-- the hiding rules actually weighed (see Flagging).
hush_flags (
  post_id      bigint not null references hush_posts on delete cascade,
  flagger      text not null,
  flagger_tier int not null default 0,
  created_at   timestamptz not null default now(),
  primary key (post_id, flagger)
);

-- A member's optional email: set, verified by tokened link, worth
-- HUSH_EMAIL_POINTS of author standing once verified_at is stamped. The token
-- is stored as its sha256 — the cleartext only ever lives in the email itself.
hush_member_emails (
  user_id           bigint primary key references users on delete cascade,
  email             text not null,
  verify_token_hash bytea,
  verified_at       timestamptz,
  updated_at        timestamptz not null default now()
);

-- Transactional mail as data: handlers insert, the pump claims pending rows by
-- next_attempt_at and delivers with backoff (see Email).
hush_outbox (
  id              bigserial primary key,
  to_email        text not null,
  subject         text not null,
  text_body       text not null default '',
  html_body       text not null default '',
  template_key    text not null default '',        -- verify | invite | application-* (for reading the table)
  status          text not null default 'pending', -- pending | sent | failed
  attempts        int  not null default 0,
  next_attempt_at timestamptz not null default now(),
  last_error      text not null default '',
  created_at      timestamptz not null default now(),
  sent_at         timestamptz
);
-- index (status, next_attempt_at): the pump's read shape

-- 00002: invites — one row per minted code. The inviter→used_by edge is the
-- chain-reputation graph; counting a member's rows enforces the allowance.
hush_invites ( code text primary key, inviter text, created_at timestamptz,
               used_by text, used_at timestamptz );

-- 00002: signups ledger — accounts created outside the admin queue, so the
-- open-registration cap can count what it has spent.
hush_signups ( id bigserial primary key, handle text, via text, -- open | invite
               created_at timestamptz );

-- 00007: live policy — the gate-and-moderation knobs (one key per row), seeded
-- from the HUSH_* env on first boot, then edited at /admin/settings (see Config).
hush_settings ( key text primary key, value text, updated_at timestamptz );
```

Both backends implement one `wall` interface (model.go): `memStore` keeps the
database-less mode alive (zero setup, like boardwalk — posts, reactions, and
flags in maps), `pgStore` (pgstore.go) is the production path. The interface
is the moderation surface too: `flag`/`hide`/`restore`/`purge` plus the admin
reads (`flagged`, `adminList`) sit alongside `list`/`get`/`neighbors`, and the
visible reads all exclude both hidden and purged posts — moderated content
simply isn't on the wall, and its permalink reads as not found, while the row
survives for the admin browser.

`hush_reactions` owns the "one reaction per voter per post" rule (the PK).
**Repute owns the math**, but the engine stays in-memory: there is no
`score_cached` column and no score-ordered query, because the wall is
chronological and every score is hidden. Instead the wall's history *is* the
event log — at boot, `replayReputation` (pgstore.go) re-emits every post,
every reaction (with its chain echo), and every verified email into a fresh
engine, so scores survive restarts without a second store.

## Repute wiring

One engine per process, built at boot (reputation.go) and wrapped in a mutex —
the engine is single-goroutine, hush's handlers aren't:

```go
eng := repute.New([]repute.Scorer{
    scorers.Participation{Track: authorTrack, Kinds: []string{"post"}, Points: 1},
    scorers.Participation{Track: authorTrack, Kinds: []string{"like"}, Points: 2},
    scorers.Participation{Track: authorTrack, Kinds: []string{"dislike"}, Points: -2},
    scorers.Participation{Track: authorTrack, Kinds: []string{"email-verified"}, Points: cfg.EmailPoints},
    scorers.Participation{Track: chainTrack, Kinds: []string{"chain-like"}, Points: 1},
    scorers.Participation{Track: chainTrack, Kinds: []string{"chain-dislike"}, Points: -1},
})
```

Four event kinds on the `author` track:

- `"post"` — emitted on create (text or image). You earn standing just by
  contributing.
- `"like"` / `"dislike"` — emitted per fresh `hush_reactions` row. A 👎 scores
  negative, so standing tracks the author's like/dislike ratio, not raw volume.
- `"email-verified"` — a one-time bump worth `HUSH_EMAIL_POINTS` when a
  member's address confirms (see Email). A reachable member is worth more than
  an anonymous drive-by.

Score for gating is `eng.Score(actor, track, now)` via `rep.score(handle)`;
`tierOf(score)` maps it against the env thresholds:

| Tier | Reached when | Capability |
| --- | --- | --- |
| 0 | account approved | post only |
| 1 | score ≥ `HUSH_TIER1` (default 3) | profile unlocks; invite minting; one flag hides alone (default `HUSH_FLAG_TIER`) |
| 2 | score ≥ `HUSH_TIER2` (default 8) | reserved — DMs are the designed-but-unbuilt extension |

**Every score is hidden** on member-facing surfaces. Standing surfaces only as
what has unlocked: the profile link appearing in the account strip is tier 1
announcing itself, and the invite allowance quietly grows or shrinks with the
chain. The numbers themselves print in exactly one place — the admin member
ledger.

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
`votePost` (and in the boot replay), and the edges in `hush_invites` —
hydrated into the in-memory graph at boot (`hydrateChains`), then replayed
through the engine with everything else, so both the edges *and* the scores
survive restarts. One hop for now; propagating a decayed echo further up the
chain is the obvious extension.

Repute is a normal (private) module dependency —
`github.com/ThirdCoastInteractive/Repute` in the root go.mod; building it
needs `GOPRIVATE=…,github.com/ThirdCoastInteractive`.

## Flagging

The wall's only report control: a dim `flag` button on the focused feed card,
members only, your own post a no-op (the wall doesn't self-report). One flag
per flagger per post — the `hush_flags` PK — and the flagger's *current tier*
is captured on the row, because tier is what the hiding rules weigh:

- a flag from tier ≥ `HUSH_FLAG_TIER` (default 1) **insta-hides** — trusted
  members moderate alone;
- `HUSH_FLAG_QUORUM` (default 3) flags of any tier hide by **agreement** —
  the untrusted crowd needs to concur.

Hidden means *gone* for non-admins: not on the wall, not in the feed, the
permalink 404s. If your flag took the post down, the focused card collapses to
a one-line `hushed.` notice; otherwise only the control goes inert
(`flagged`). No counts, no progress bar — **a member never learns what their
flag did**. Hiding is reversible by design: the post keeps its row, its flags,
and its capture meta for admin review, where the ruling is `restore` (back on
the wall, flags cleared so they can't re-hide it) or `purge` (a **soft**
delete: `deleted_at`/`deleted_by` are stamped and the post leaves every public
surface and the review queue, but the row, reactions, flags, and capture meta
stay as evidence — kept in the admin browser under a `purged` badge, never
destroyed; the terms promise exactly that). Admins can also `hide`
directly from the browser pages — same switch, their handle in `hidden_by`
instead of `'flags'`.

## Admin

The pages with eyes (admin.go, views in adminviews.templ): everything the
member-facing wall refuses to print — scores, tiers, chains, vote counts, IPs,
geo, user agents — is laid out here, every route behind `app.Can("admin")`.
Without a database they degrade to a notice, like the rest of the gate.

- **`/admin` — the hub.** Four numbers, each a way into its section: pending
  applications, flagged-or-hidden posts, total posts, members.
- **`/admin/queue` — the application queue.** Each pending application in
  full — handle, pitch, the photo resolved from its storage key — with
  approve/reject forms posting to `/admin/decide`.
- **`/admin/review` — the moderation queue.** Every hidden or flagged post
  rendered whole (the admin judges the actual content, not a snippet), with
  its capture meta, who flagged it and at what tier, and the rulings: restore
  or purge.
- **`/admin/posts` — the everything browser.** The newest 500 posts including
  hidden ones, one dense row each: id, kind, a 90-char snippet, author,
  IP / geo (Cloudflare's country+city headers) / user agent, the 👍/👎 tallies
  members never see, hidden state, and inline hide/restore/purge. Exact-match
  `?author=` and `?ip=` filters narrow the window, and moderating a row 303s
  back to the same filtered view.
- **`/admin/members` — the member ledger.** Every account with the real
  numbers: roles, author score, tier, chain score, post count, email (masked
  nowhere here) + verified state, invites minted/redeemed, last-seen IP.
- **`/admin/outbox` — the mail ledger.** The last 200 `hush_outbox` rows,
  newest first: recipient, subject, template, status, attempts, and the
  `last_error` that parked anything failed.
- **`/admin/analytics` — the dashboard** (see Analytics).
- **`/admin/settings` — the live policy editor** (see Config). The gate and
  moderation knobs as a form, saved to `hush_settings` and effective on the
  next request — the numbers the wall hides from members are tuned here.

One shared admin chrome (`adminPage` + `adminNav`) strings the sections
together. The moderation action all the forms post to is `/admin/moderate`;
its `back` parameter is clamped to `/admin*` paths so the form can't be used
to redirect off-site.

## Email

Optional everywhere, by policy: every signup door (apply, open registration,
invite redemption) takes an optional address, the profile can add or change
one, and a bad address is quietly dropped (`normalizeEmail`) — never an error
a user has to see. Addresses are never shown publicly; even the masked form
(`m***@example.com`) only renders on its owner's profile.

**Verification** (verify.go): attaching an address stores it unverified with a
fresh 32-byte token and queues a verification mail; the table keeps only the
token's sha256, so a leaked dump verifies nobody. `GET /email/verify/{token}`
matches the hash against *unverified* rows only — a link works exactly once —
stamps `verified_at`, and emits `"email-verified"` into the author track,
worth `HUSH_EMAIL_POINTS` of standing (the one-use UPDATE is also what keeps
the live emit from double-counting; boot replay re-emits from `verified_at`
rows instead). Changing an address always restarts verification, and the
profile offers a resend.

**The outbox** (outbox.go): transactional email as data. Handlers never block
on SMTP — they `enqueue` a row into `hush_outbox`, best-effort by design
(every email hush sends is a courtesy; a queue failure logs and the request
proceeds). A process-lifetime pump ticks every 5s and drains batches of 10
inside a transaction: `FOR UPDATE SKIP LOCKED` is the claim, so a second
instance pumping the same table skips past in-flight rows instead of
double-sending, and a crashed send is just a row still pending. A failed send
backs off at `1m · 2^attempts`, capped at an hour, until the eighth failure
parks the row as `failed`. Boot's reputation replay never enqueues, so a
restart sends nothing twice.

**The transport** (the framework's `mailer` package): `mailer.New(ConfigFromEnv())`
picks the strongest the env supports — the Mailgun API when `MAILGUN_KEY` +
`MAILGUN_DOMAIN` are set, plain SMTP when `SMTP_HOST` is, and a `LogMailer`
otherwise, so the dev default "delivers" to the terminal and the flows work on
a fresh checkout. Each message carries a canonical text body plus an HTML half
built from templ components (emails.templ, the white shell; emails.go pairs
them), with absolute links built on `HUSH_BASE_URL`.

What sends mail:

| Event | To | Template key |
| --- | --- | --- |
| address attached (signup or profile) / resend | the member | `verify` |
| application submitted | `HUSH_ADMIN_EMAIL` (if set) | `application-received` |
| application approved | the applicant (if they gave an address) — with a sign-in link, and a verify link riding along while the address is unverified | `application-approved` |
| application rejected | the applicant (if they gave an address) | `application-rejected` |
| invite minted with the "mint & send" email field | the invitee — the code as its one-use redemption link | `invite` |

## Analytics

ClickHouse is the wall's **observation store** (analytics.go): Postgres holds
the posts and the gate (the system of record), ClickHouse holds the firehose —
one append-only row per thing that happened. Entirely optional: no
`CLICKHOUSE_DSN` means a nil conn, a no-op batcher, and a dashboard page that
says so instead of charting — analytics off is a configuration, not an error.
(A set-but-*wrong* DSN panics at boot — `ch.MustConn` — but a failed ClickHouse
migration degrades to off rather than killing the wall.)

The wiring is the `ch` package end to end: `ch.MustConn` opens the conn,
`ch.Migrate` runs `chmigrations/` (goose, same workflow as Postgres), and a
`ch.Batcher[event]` buffers rows into batched inserts off the request path —
`track()` is fire-and-forget and nil-safe, so the wall runs identically with
analytics off. One table:

```sql
hush_events (
  ts      DateTime,
  kind    LowCardinality(String),
  handle  String,         -- '' for anonymous views
  post_id Int64,          -- 0 when no post is involved
  ip      String, country LowCardinality(String), city String, ua String,
  path    String
) ENGINE = MergeTree ORDER BY (kind, ts)   -- per-kind day buckets read a contiguous range
```

Emitted kinds: `view_wall`, `view_post`, `post_text`, `post_image`, `vote_up`,
`vote_down`, `flag`, `apply`, `register`, `redeem`, `login_ok`, `login_fail`,
`logout`. The IP/geo/UA on each row mirror the moderation Meta on posts —
admin eyes only, like everything here (`login_fail` keeps the attempted
handle: brute-force runs show up as one handle hammered from one IP).

`/admin/analytics` is a grid of **deferred widgets** — the pantry dashboard
pattern: each card reserves its height and `ui.Defer` `@get`s its fragment on
intersection, so the ClickHouse aggregates never block first paint. The
widgets (each its own admin-guarded fragment route): events per day (14d,
line), posts per day and votes per day (14d, bars), top countries (30d, bars —
Cloudflare's `CF-IPCountry`, blank in dev), top IPs (30d, a table — the labels
are the data, read down into a blocklist), and the raw tail of the last 100
events. Charts ride `ch.Rows` → `ch.ToLineSeriesData`/`ToHBarSeries` → the
`chart` package's templ fragments; only the two tables are hush's own CSS.

## Media

Two independent env-driven decisions (media.go), both optional — with nothing
set, files live in a local temp dir served at `/files` and image posts are raw
stored bytes, exactly the database-less developer experience:

- **The file store** is Cloudflare R2 via `storage/s3` when all four `R2_*`
  variables are set — `PublicBaseURL` from `R2_PUBLIC_URL` makes `URL()` pure
  string concatenation against the bucket's custom domain; unset means
  presigned one-hour links, which still work. Otherwise it's `storage.Local`
  under `HUSH_FILES_DIR` (default: OS temp). Application photos and
  non-Cloudflare image posts flow through it either way, behind the same
  `storage.Store` interface.
- **Cloudflare Images** (`storage/cfimages`) exists when the three
  `CF_IMAGES_*` variables are set; nil means image posts go through the file
  store. When wired, `storeImage` uploads the bytes there and the post row
  keeps both the image id and the full variant's delivery URL (a stable
  fallback even if the client is later unwired). Variants are dashboard-side
  config: `CF_IMAGES_VARIANT` (default `public`) is the full-size source feed
  cards render, `CF_IMAGES_THUMB` (default `thumb`, a ~600px square crop) is
  what grid tiles use — Cloudflare resizes server-side, so the wall stops
  shipping full-size images into 180px squares. `postImageURL`/`postThumbURL`
  recompute delivery URLs from the id at render time, so a variant rename in
  the env applies to old posts too; posts without a `cf_image_id` fall back to
  their stored URL, and the tile's `object-fit` crops locally.

The `/files/` route is only mounted when the store is `*storage.Local` — R2
URLs are absolute, pointing at the bucket's public/custom domain (or
presigned), so there is nothing for the app to serve.

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

Pipeline (`examples/hush/static/js/camera.js`), a shader instrument — **still
capture, not video, and WebGPU, not WebGL**:

1. **Two sources, one pipeline.** The camera
   (`getUserMedia({ video })` → a per-frame copy into the source texture) or a
   picked image file (`createImageBitmap` → the same texture). The
   upload-and-edit path the docs always promised is now real: file mode works
   with no webcam at all.
2. **One WGSL fragment shader, one pass.** A uniforms struct carries the six
   grade params (brightness, contrast, saturation, temperature, grain,
   vignette), the effect flags/intensities, and a `time` clock. The pass
   samples (with VHS row-displacement and channel aberration when on), takes
   halation taps, grades, layers the effects (crushed blacks, blown
   highlights, lifted blacks, mono, duotone, negative, ordered-dither
   posterize, scanlines), then closes the vignette — all arithmetic, identity
   at zero, so the looks stack freely without rebuilding the pipeline.
3. **Nine preset looks** (`none`, `noir`, `bleach`, `dream`, `vhs`, `poster`,
   `wound`, `negative`, `crush`) are slider-set-plus-flag bundles. Picking one
   writes the sliders (so you see what it did) and swaps the flags whole; the
   six sliders then fine-tune live on top. A steady `requestAnimationFrame`
   loop drives both modes, so animated grain and scanlines stay alive even on
   a still.
4. **Capture** is `canvas.toBlob('image/jpeg', 0.9)` of the filtered canvas in
   the same frame as a render — the shader already modified the preview, so the
   canvas *is* the modified still. `POST /upload` hands the blob to the normal
   action → `storeImage` (Cloudflare Images when wired, the file store
   otherwise) → the tile renders from the thumb variant or the stored URL. The
   § 2257 certification gates the shutter (and the server refuses uncertified
   uploads regardless).

Feature detection: no `navigator.gpu` disables the shutter, hides the
instrument, and points back at the wall's plain (unfiltered) upload; WebGPU
present but no webcam still does file mode. WGSL leaves room to grow beyond
per-pixel — separable blur/bokeh, histogram tone-mapping, 3D film LUTs —
without touching capture or upload, which sit above the renderer.

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
thumbs and the flag control, and nothing else; **there are no replies**, a
post stands alone and the only answers the wall allows are a thumb and a flag
— with its **time-ordered neighbors** around it, newer above, older below,
newest-first all the way down. The link's `#p{id}`
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
GET  /                            grid    -- gapless square grid, chronological
GET  /compose                     page    -- the WebGPU camera/upload island
GET  /p/{id}                      page    -- the feed, focused on one post (neighbors both ways)
GET  /feed/newer                  frag    -- next feed batch above cursor (intersect sentinel only)
GET  /feed/older                  frag    -- next feed batch below cursor (intersect sentinel only)
POST /posts                       action  -- bind, render+count, store, emit Repute "post"    [post]
POST /p/{id}/vote                 action  -- 👍/👎 (?dir=up|down), Repute + chain echo        [post]
POST /p/{id}/flag                 action  -- record a flag, apply the hiding rules            [post]
POST /upload                      raw     -- multipart image (form or camera blob), 303 home  [post]

GET  /apply                       page    -- claim-a-seat form while open seats remain; else the application form
POST /apply                       raw     -- write the pending application (photo + optional email)
POST /register                    raw     -- claim one open seat (HUSH_OPEN_SLOTS), create account, sign in
GET  /invite/{code}               page    -- redemption form for a live code; notice for spent/unknown
POST /invite/{code}               raw     -- redeem: claim code, create account, record chain edge, sign in
POST /invites                     raw     -- mint one code (allowance-throttled; optional mint & send) [post]
GET  /login, POST /login          page/raw-- plain-form sign in (303, CSP-clean)
GET  /logout                      raw     -- revoke session, clear cookie

GET  /u/{handle}                  page    -- profile, tier-1 gated; own profile carries invites + email
GET  /email/verify/{token}        page    -- the one-use verification link (public — it arrives by mail)
POST /email/set                   raw     -- add/change your address (session checked inside)
POST /email/resend                raw     -- fresh token + verification mail for an unverified address

GET  /admin                       page    -- the hub: four numbers                            [admin]
GET  /admin/queue                 page    -- pending applications                             [admin]
POST /admin/decide                raw     -- approve → create user (+ mail); or reject (+ mail) [admin]
GET  /admin/review                page    -- hidden/flagged posts in full, restore or purge   [admin]
GET  /admin/posts                 page    -- everything browser (?author= &ip=), meta + tallies [admin]
GET  /admin/members               page    -- member ledger: the real numbers                  [admin]
POST /admin/moderate              raw     -- hide / restore / purge (soft), 303 back within /admin [admin]
GET  /admin/outbox                page    -- the transactional-mail ledger                    [admin]
GET  /admin/settings              page    -- the live policy editor                           [admin]
POST /admin/settings              raw     -- save the policy to hush_settings, effective live [admin]
GET  /admin/analytics             page    -- the dashboard of deferred widgets                [admin]
GET  /admin/analytics/{events,posts,votes,countries,ips,recent}
                                  frag    -- one widget each, live ClickHouse queries         [admin]

GET  /files/                      raw     -- storage.Local.ServeHTTP (Local store only; R2 URLs are absolute)
GET  /specimen                    page    -- the framework specimen (mounted by every example)
```

`[post]` and `[admin]` are `app.Can(...)` guards, attached at registration so
the capability story reads off the route table — and only attached in database
mode: without `DATABASE_URL` there is no principal, the wall is open, and the
admin pages degrade to a notice. Actions are `beach.Bind[T]` + `beach.Patches`
(Datastar JSON signals); the auth, signup, email, and admin forms are plain
POSTs that 303 (CSP-clean, no Datastar dependency).

## Config

Two layers. **Infrastructure, secrets, and scoring weights** are env, read once
at boot into `a.cfg` (config.go); the two example files mirror this table
(`.env.example` for `go run`, `.env.docker.example` for the stack — see
Running it for why they differ on the database). The
**gate-and-moderation policy** (tier thresholds, flag tier/quorum, invite
base/cap, open slots, admin email) lives in the `hush_settings` table
(settings.go), edited at `/admin/settings` with **no restart** — held behind an
atomic pointer (`a.live()`) so the hot-path reads stay lock-free. The `HUSH_*`
policy vars below only **seed** that table on first boot; after that the
database is authoritative (in database-less mode they are the live values). The
split is "how to reach the world and score reputation" (env) vs "how the gate
behaves" (DB).

| Variable | Meaning |
| --- | --- |
| `PORT` | listen port (default 8080) |
| `DATABASE_URL` | Postgres DSN; unset = open, in-memory, no gate |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | compose-only: the credentials the postgres service is created with and that compose's built `DATABASE_URL` uses (defaults `beach`/`beach`/`hush`) |
| `CLICKHOUSE_DSN` | ClickHouse DSN for the observation store; unset = analytics off (set-but-wrong panics at boot) |
| `HUSH_EMAIL_POINTS` | author standing granted for a verified email (default 2); env-only, baked into the Repute engine at boot |
| `HUSH_BASE_URL` | absolute origin for links inside outgoing email (default `http://localhost:8080`) |
| `HUSH_FILES_DIR` | Local file-store root when R2 is unset (default: under the OS temp dir) |
| `HUSH_TIER1` / `HUSH_TIER2` | **seed:** author scores that unlock tier 1 (profile + invites) and tier 2 (DMs); defaults 3 / 8 |
| `HUSH_FLAG_TIER` / `HUSH_FLAG_QUORUM` | **seed:** the tier whose single flag insta-hides, and how many tier-0 flags hide by agreement; defaults 1 / 3 |
| `HUSH_INVITE_BASE` / `HUSH_INVITE_MAX` | **seed:** a tier-1 member's starting invite allowance and the hard cap; defaults 3 / 6 |
| `HUSH_OPEN_SLOTS` | **seed:** open the first N registrations with no review; once spent the queue resumes (default 0) |
| `HUSH_ADMIN_EMAIL` | **seed:** where new-application notices go; blank = no admin mail |
| `MAIL_FROM_NAME` / `MAIL_FROM_ADDR` | the From header on outgoing mail |
| `MAILGUN_KEY` / `MAILGUN_DOMAIN` | Mailgun API transport — the strongest, picked first |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` | plain SMTP transport (port defaults to 587); with no transport set, the log mailer prints sends to the app log |
| `R2_ENDPOINT` | the bucket's S3 API host, no scheme (`accountid.r2.cloudflarestorage.com`) |
| `R2_BUCKET` | bucket name; one store per bucket |
| `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` | R2 API token key pair — all four R2_* set switches the file store to R2 |
| `R2_PUBLIC_URL` | public/custom-domain base for stable URLs; unset = presigned 1h links |
| `CF_IMAGES_ACCOUNT_ID` | Cloudflare account id — all three CF_IMAGES_* set routes image posts through Cloudflare Images |
| `CF_IMAGES_TOKEN` | API token with Cloudflare Images edit permission |
| `CF_IMAGES_HASH` | delivery hash, the first path segment of imagedelivery.net URLs |
| `CF_IMAGES_VARIANT` | full-size variant feed cards render (default `public`) |
| `CF_IMAGES_THUMB` | square-crop variant grid tiles render (default `thumb`) |

## Build blocks

Gated like the [roadmap](../ROADMAP.md): each block ends in something you can run
and test in a real browser (compiling and serving 200 is not done).

**Block A — board skeleton. ✅ built.** In-memory post store (database-less, like
boardwalk — zero setup to run; Postgres landed with the gate in Block D and took
the wall itself in Block G). The gapless grid renders, seeded with a few posts.
The text path (`renderPost`) is wired into `POST /posts`, which prepends the new
tile and clears the composer. The composer lives inline on `/` (the dedicated
`/compose` route hosts the camera/upload island from Block C). ◆ Gate (verified
in-browser): post text and watch it appear as a square tile; bold/italics/lists
render; the 512-character limit and the empty post are both rejected with an
inline notice.

**Block B — storage + images. ✅ built.** `storage.Local` rooted under the OS temp
dir, served at `/files` via `ServeHTTP`. Image posts come from a plain multipart
upload form (303 back to the wall — CSP-clean, no Datastar). ◆ Gate (verified
in-browser): upload an image, it `Put`s to the store, serves from `/files`, and
renders as an image tile in the gapless wall.

**Block C — WebGPU camera. ✅ built (shader instrument).** The `camera.js`
island on `/compose` is a one-pass WGSL fragment shader fed by either the
webcam or a picked photo, with nine preset looks and six live grade sliders on
uniforms (see [the camera section](#the-camera--webgpu-filter-pipeline)) →
`canvas.toBlob` → `POST /upload` → back to the wall. Feature-detects
`navigator.gpu` (falls back to the plain upload) and a missing webcam (file
mode still works). ◆ Gate (verified in-browser): file mode grades a synthesized
image through `noir` and the grayscale grainy result lands on the wall;
presets and sliders change the rendered pixels live.

**Block D — the gate. ✅ built (verified against Postgres).** DB-optional, like
pantry: set `DATABASE_URL` and the gate turns on; without it the wall runs open.
`hush_applications` migration + the framework's session/auth migrations. `/apply`
(handle, password, pitch, optional photo → `storage`) writes a pending row;
`/admin/queue` + `/admin/decide` (admin-gated) approve → create an auth `users`
row + local credential + `member` role, or reject. Plain-form `/login` /
`/logout` (303, CSP-clean). Posting (`/posts`, `/upload`, voting) is guarded by
`app.Can("post")`; the composer only renders for users who hold it,
and an account strip shows who you are + the admin queue link. Seeds an
`admin / password` account. The post wall itself stayed in-memory through this
block — Block G moved it to Postgres. ◆ Gate (verified in-browser): logged-out
wall is read-only with an invite prompt → apply as `moth` → sign in as `admin`,
approve in the queue → sign out, sign in as `moth` → post lands on the wall.

**Block E — Repute + tiers. ✅ built (verified against Postgres).** Repute is a
normal dependency (`github.com/ThirdCoastInteractive/Repute`, private). A
mutex-wrapped engine with `Participation` scorers folds `post` (+1),
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
announcing itself, the tier-1 profile (`/u/{handle}`) shows a post count, and
the invite allowance quietly grows or shrinks with the chain. The engine is
in-memory; since Block G it replays the wall's Postgres history at boot, so
scores survive restarts. ◆ Gate (verified in-browser): sign in as `moth`,
post (score 1), get 👍'd (score 3) → tier 1 → the profile link appears and
`/u/moth` renders.

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

**Block G — production: postgres wall + flags + admin + email + analytics +
media. ✅ built (verified against Postgres + ClickHouse).** The remaining
follow-ups, landed as one build-out. The wall moves to Postgres (`pgStore`
behind the same `wall` interface; posts, reactions, capture meta, and hidden
state in `hush_posts`/`hush_reactions`, Repute replayed from that history at
boot). Community flagging with the tier/quorum hiding rules and the silent
member-side UX. The admin suite: hub, review, the everything browser with
IP/geo/UA and vote tallies, the member ledger with the real numbers, and the
analytics dashboard. Optional member emails with one-use sha256-tokened
verification feeding `HUSH_EMAIL_POINTS` into the author track, delivered
through the `hush_outbox` queue (SKIP LOCKED claims, doubling backoff, failed
after 8) over the env-picked mailer transport. ClickHouse event tracking
behind `CLICKHOUSE_DSN` with deferred chart widgets. And production media: R2
via `storage/s3` and Cloudflare Images variants, both env-switched with clean
local fallbacks. ◆ Gate (verified in-browser, against the docker-compose
Postgres + ClickHouse): a restart keeps the wall, the reactions, and the
tiers; a tier-1 flag hides a post instantly (permalink 404s, the card reads
"hushed.") and three tier-0 flags hide one by agreement; `/admin/review`
restores it back onto the wall (flags cleared) and purge deletes outright;
`/admin/posts` filters by author/IP and shows the tallies, `/admin/members`
shows scores/tiers/chains/emails; applying with an address sends the admin
notice and approval sends the welcome + verify link (log mailer in dev — the
mail prints), the verify link bumps standing exactly once and the second click
says so; the analytics dashboard charts the session's views/posts/votes and
lists the source IPs.

Each block is verified in a real browser before the next — the lesson from the
trio: a rendered page is not a working feature.

## Running it

There's an env example per way to run, each self-contained and each naming the
database in exactly one place.

**Locally (`go run`):** copy `examples/hush/.env.example` to `.env` and point
`DATABASE_URL` at a Postgres (or unset it to run open/database-less);
optionally point `CLICKHOUSE_DSN` at a ClickHouse for the analytics dashboard.
That one DSN is the whole database story — `go run` connects to a database,
it doesn't create one, so there is no separate credentials block. The `.env`
loads at boot (cwd or the example dir; real env wins). The open database-less
wall is a dev convenience only and announces itself in the log; with `RELEASE`
set, a missing `DATABASE_URL` — or any database failure at boot — is fatal: a
production wall never quietly runs open.

**Containerized:** copy `examples/hush/.env.docker.example` to `.env`, then
`docker compose build --ssh default && docker compose up` (the SSH agent is
forwarded so the image build can fetch the private Repute dep, which otherwise
needs `GOPRIVATE=…,github.com/ThirdCoastInteractive`; see the
[Dockerfile](../../Dockerfile)). The stack runs Postgres and ClickHouse
**internal-only** alongside the app, with **only the web port published**. The
docker `.env` carries no `DATABASE_URL` and no localhost: it holds the Postgres
credentials, the secrets, and the policy seeds, and `docker-compose.yml` builds
the connection strings from those credentials and the service names
(`postgres:5432`, `clickhouse:9000`) in its `environment` block — so the
database connection is configured once, in the compose file, not duplicated
into the env. `HUSH_BASE_URL` comes from the `.env` (your public origin) or a
localhost default. ClickHouse's healthcheck is `clickhouse-client --query
'SELECT 1'`, which talks to its own server with no host to name.
