# saltbox — historic floorplans, rebuilt in miniature

Upload a scan of a historic floorplan — a fire-insurance sheet, a county-archive
blueprint, the pencil plan of your grandparents' farmhouse — calibrate its scale,
and trace the walls with a snapping pen. saltbox extrudes the tracing into a 3D
**dollhouse**: open-topped rooms cut from white card, the original scan lying on
each floor like period wallpaper, storeys you can lift apart, a slow orbit you
can share with a link. The pleasure is the tracing; the keepsake is the house.

(A *saltbox* is the lean-to-roofed house form of the 17th–19th-century New
England coast — a historic house that is also, conveniently, a little box.)

> **Status: plan of record — nothing below is built yet.** This README is the
> design; `mockups/` holds static HTML mockups of the four surfaces. When
> implementation starts, this doc gets rewritten section by section into the
> as-built description, the way the other app READMEs read.

## The loop

1. **Found a plan.** Create a house (title, year, place), upload the scan —
   JPEG/PNG, one sheet per storey.
2. **Calibrate.** Draw one line along something whose length you know ("this
   doorway is 3 ft", "one chain on the Sanborn grid") and type the length. That
   fixes pixels-per-metre; nudge rotation until the walls sit square.
3. **Trace.** Walls as snapping polylines over the faded scan, doors and windows
   punched onto walls, rooms named — *Parlor*, *Scullery*, *Sleeping Porch*.
4. **Stack.** One floor per sheet; onion-skin the storey below to keep the
   stairs and chimneys aligned.
5. **Dollhouse.** The tracing extrudes live: card walls, scan-papered floors, no
   roof (a dollhouse is open at the top — that is the whole charm). Orbit, lift
   the storeys apart, ghost the near walls to see in.
6. **Shelve it.** Publish to the public shelf, or keep it unlisted and send the
   link. Watchers see strokes land live while you trace.

## UI mockups

Static, self-contained HTML — open directly in a browser. They define the look:
**a drafting sheet, not a dashboard.** Aged paper, india ink, Sanborn carmine, a
title block in the corner of every surface. The built app renders these as templ
views + two client-JS islands.

| File | Surface |
| --- | --- |
| [mockups/01-shelf.html](mockups/01-shelf.html) | The shelf — public gallery of finished dollhouses, live cards |
| [mockups/02-scan.html](mockups/02-scan.html) | Scan & calibrate — upload strip, the scale line, storey assignment |
| [mockups/03-trace.html](mockups/03-trace.html) | The tracing room — tool palette, snapping pen over the faded scan, storey rail |
| [mockups/04-dollhouse.html](mockups/04-dollhouse.html) | The dollhouse — extruded card model (CSS-3D stand-in for the three.js island), storey lift, cutaway |

## Where it sits in the family

The examples each stress a slice of the framework; saltbox is a product-shaped
app (the askmuskie precedent) that picks up the two slices nothing else
exercises for real:

- **`storage` ●●** — first consumer outside hush, and the first to use the
  advertised one-line backend swap: `storage.Local` under dev, `storage/s3`
  (R2) in production, same `Store` interface, no app change.
- **`App.Socket` ●●** — first *bidirectional* production consumer (driftbottle's
  `/sockets` is a demo lane): ops upstream on the ordered `Write` lane,
  house snapshots downstream on the `WriteLatest` coalescing mailbox.
- Also exercised: hub/SSE chrome liveness, `auth` + sessions (registered
  owners), `pg` (Postgres-only stack, the booking-manager half of the stack
  rule), driftwood + specimen, and two client-JS islands (canvas 2D + three.js)
  on the SSR core — the hush/askmuskie island pattern.

## Architecture

The whole app is the two-lane doctrine from
[20-sync](../../docs/architecture/20-sync.md) drawn as a product. **SSE carries
hypermedia** — every chip, card, and panel that changes on screen arrives as a
rendered fragment over a hub topic. **The socket carries what is not
hypermedia** — the geometry document, which belongs to the canvas and the
three.js island, never to the DOM patcher.

```mermaid
flowchart LR
    subgraph browser["Browser"]
        SSR["SSR chrome — templ + Datastar<br/>shelf · house page · panels · chips"]
        TRACE["trace.js island<br/>canvas 2D pen over the scan"]
        DOLL["dollhouse.js island<br/>three.js extrusion"]
    end
    subgraph proc["saltbox process"]
        WEB["internal/web<br/>Page / Action / Stream / Raw"]
        SOCK["App.Socket /ws/h/{slug}<br/>internal/live — house rooms"]
        GEO["internal/geometry<br/>validate → apply → rev++"]
        MEDIA["internal/media<br/>keys · limits · dimensions"]
        HUB["hub<br/>topic fan-out"]
    end
    PG[("Postgres<br/>sb_houses · sb_floors (geometry jsonb)<br/>sb_scans · sb_revisions")]
    OBJ[("storage.Store<br/>Local (dev) / R2 (prod)<br/>scans + covers")]

    SSR -- "pages + Datastar actions" --> WEB
    HUB -- "SSE fragments<br/>GET /h/{slug}/live" --> SSR
    TRACE -- "op / hello frames (JSON, Write)" --> SOCK
    SOCK -- "house snapshot<br/>(WriteLatest, latest-wins)" --> TRACE
    SOCK -- "house snapshot" --> DOLL
    SSR -- "multipart scan upload (Raw)" --> MEDIA
    MEDIA --> OBJ
    SOCK --> GEO
    GEO -- "debounced persist" --> PG
    WEB --> PG
    SOCK -- "presence / saved chips<br/>(rendered fragments)" --> HUB
```

One stroke, end to end:

```mermaid
sequenceDiagram
    participant E as trace.js (owner)
    participant S as App.Socket handler
    participant G as internal/geometry
    participant P as Postgres
    participant H as hub
    participant W as watcher (dollhouse.js)

    E->>S: {t:"op", floor, base:213, ops:[set-wall …]}  (pointer-up)
    S->>G: apply(doc, ops) — validate, base check
    G-->>S: doc', rev 214
    S-->>E: {t:"ack", floor, rev:214}  (Write, ordered)
    S-->>W: {t:"house", rev:214, …}    (WriteLatest — a slow watcher skips straight here)
    S->>P: UPDATE sb_floors SET geometry, geometry_rev  (debounced ~2s)
    S->>H: publish house:slug — "saved · rev 214" chip (rendered fragment)
    H-->>E: SSE patch morphs the save chip
```

The editor's own canvas never waits on the server — it draws optimistically and
reconciles on `ack`/`house`. The server is still the only writer of record:
every op batch is validated and applied on the house's room goroutine
(single-owner, serialized), so last-write-wins never actually fires — conflicts
are designed out, per doctrine.

## App layout

The `beach new` shape, split by concern
([01-layout](../../docs/architecture/01-layout.md)):

```
cmd/saltbox/
├── main.go               wire: config → pool/migrate → storage → live rooms → web → Start
├── config.go             typed env loader (pkg/config)
├── migrations/           goose; framework auth.Migrations + session.Migrations run ahead
├── internal/
│   ├── web/              app struct, handlers, views.templ (shell, shelf, house, editor)
│   ├── store/            domain types + sqlc over the sb_* tables
│   ├── geometry/         the traced document: types, validate, apply(ops), rev —
│   │                     pure Go, zero beach imports, exhaustively unit-tested
│   ├── live/             per-house rooms: socket registry, op serialization,
│   │                     debounced persist, snapshot broadcast, hub chip publishes
│   └── media/            storage wiring: key scheme, size caps, MIME sniff, DecodeConfig dims
├── static/js/            trace.js, dollhouse.js, vendored three.js ESM (askmuskie precedent);
│                         beach-ws.js comes from the framework's embedded assets
├── docker-compose.yml    postgres
└── README.md             this file
```

## Data model

Postgres is the system of record; the object store holds bytes. Framework
identity tables (`users`, sessions, RBAC) come from `auth.Migrations` +
`session.Migrations` as their own goose sources, ahead of the app's — the
pantry/booking-manager boot shape.

```mermaid
erDiagram
    users ||--o{ sb_houses : owns
    sb_houses ||--o{ sb_scans : keeps
    sb_houses ||--o{ sb_floors : stacks
    sb_scans |o--o{ sb_floors : underlays
    sb_floors ||--o{ sb_revisions : checkpoints
```

```sql
-- saltbox domain schema: houses on the shelf, their scanned sheets, the traced
-- storeys (geometry lives here, one jsonb document per floor), and append-only
-- revision checkpoints for restore points.

-- +goose Up

-- A house: one traced building, owned by one user. slug is a short random id
-- and doubles as the capability for 'unlisted' visibility.
-- +goose StatementBegin
CREATE TABLE sb_houses (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    slug        text        NOT NULL UNIQUE,
    title       text        NOT NULL,
    summary     text        NOT NULL DEFAULT '',
    built_year  int,                                  -- best guess; NULL = unknown
    place       text        NOT NULL DEFAULT '',      -- "Galveston, TX" — display only
    visibility  text        NOT NULL DEFAULT 'private', -- private|unlisted|public
    cover_key   text        NOT NULL DEFAULT '',      -- storage key of the cover snapshot
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sb_houses_shelf_idx
    ON sb_houses (visibility, updated_at DESC) WHERE deleted_at IS NULL;
-- +goose StatementEnd

-- An uploaded sheet. Bytes live in the object store under storage_key; the row
-- is metadata. Scans outlive floor assignment (a sheet can be re-assigned).
-- +goose StatementBegin
CREATE TABLE sb_scans (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    house_id     bigint      NOT NULL REFERENCES sb_houses (id) ON DELETE CASCADE,
    storage_key  text        NOT NULL,               -- scans/<house>/<rand>.<ext>
    filename     text        NOT NULL,               -- as uploaded, display only
    content_type text        NOT NULL,               -- image/jpeg | image/png
    width_px     int         NOT NULL,
    height_px    int         NOT NULL,
    bytes        bigint      NOT NULL,
    uploaded_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- A storey. The traced document is ONE jsonb column, versioned by geometry_rev
-- (bumped on every accepted op batch; the socket protocol's base check and the
-- ?since-style resume both key on it).
-- +goose StatementBegin
CREATE TABLE sb_floors (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    house_id     bigint      NOT NULL REFERENCES sb_houses (id) ON DELETE CASCADE,
    position     int         NOT NULL,               -- 0 = ground, stacking order
    name         text        NOT NULL DEFAULT '',    -- "Ground", "First", "Attic"
    height_m     real        NOT NULL DEFAULT 2.7,   -- wall height for extrusion
    scan_id      bigint      REFERENCES sb_scans (id) ON DELETE SET NULL,
    calibration  jsonb       NOT NULL DEFAULT '{}',  -- ppm, origin, rotation, opacity
    geometry     jsonb       NOT NULL DEFAULT '{}',  -- the traced document (below)
    geometry_rev bigint      NOT NULL DEFAULT 0,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (house_id, position)
);
-- +goose StatementEnd

-- Append-only checkpoints: every ~50 revs (or 10 quiet minutes) the current
-- document is copied here; the last 20 per floor are kept. Restore copies a
-- checkpoint forward as a NEW rev — history never rewinds.
-- +goose StatementBegin
CREATE TABLE sb_revisions (
    id       bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    floor_id bigint      NOT NULL REFERENCES sb_floors (id) ON DELETE CASCADE,
    rev      bigint      NOT NULL,
    geometry jsonb       NOT NULL,
    saved_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sb_revisions_floor_idx ON sb_revisions (floor_id, rev DESC);
-- +goose StatementEnd
```

**Why one jsonb document per floor, not wall rows.** The geometry is a canvas
document: it is always loaded whole (the islands need every wall to draw one
frame), always saved whole, and never queried across houses ("all doors wider
than 900mm" is not a product question). Normalizing walls into rows would buy
per-wall UPDATE granularity nobody needs and cost a join-and-reassemble on the
hottest read. The document is validated by `internal/geometry` on every write —
jsonb here is a typed document with one writer, not a schema escape hatch.

### The geometry document

Stored in `sb_floors.geometry`, validated and mutated only by
`internal/geometry.Apply`. Coordinates are metres in plan space (calibration
maps scan pixels → metres; the document never stores pixels):

```jsonc
{
  "points":   { "p1": [0.0, 0.0], "p2": [7.3, 0.0] },       // id → [x, y] metres
  "walls":    [ { "id": "w1", "a": "p1", "b": "p2", "t": 0.18 } ],   // t = thickness
  "openings": [ { "id": "o1", "wall": "w1", "at": 0.55,      // 0..1 along the wall
                  "w": 0.90, "kind": "door",                 // door | window
                  "sill": 0.0, "head": 2.10, "swing": "in-left" } ],
  "rooms":    [ { "id": "r1", "name": "Parlor", "loop": ["p1","p2","p6","p5"] } ],
  "labels":   [ { "id": "l1", "xy": [4.1, 2.0], "text": "porch (unheated)" } ]
}
```

Walls share endpoints through the point table, so dragging a corner moves every
wall that meets it — the snap graph is the data model. Wall height comes from
the floor (`height_m`); a per-wall `h` override is allowed for the odd
half-height parapet. `sb_floors.calibration` is
`{ "ppm": 120.4, "origin": [x, y], "rot": -0.013, "opacity": 0.4 }` — enough to
place the scan under the tracing and as the floor texture, nothing more.

## The wire

### Socket — `/ws/h/{slug}` (the geometry channel)

Text JSON frames, version-tagged by `geometry_rev`. Anyone who may *view* the
house may connect; **op frames are accepted from the owner only** — a watcher
sending ops gets `err` and, on repeat, close `4403`. The room goroutine in
`internal/live` serializes all ops for a house, so the apply path is
single-writer by construction.

| Frame | Direction | Lane | Meaning |
| --- | --- | --- | --- |
| `{t:"hello", since:{floorID:rev}}` | up | — | announce; server answers with a snapshot if anything moved past `since` |
| `{t:"op", floor, base, ops:[…]}` | up (owner) | — | mutate; `base` must equal the current rev, else `err stale` + a fresh snapshot |
| `{t:"ack", floor, rev}` | down | `Write` (ordered) | op batch accepted; the editor reconciles its optimistic state |
| `{t:"err", code, msg}` | down | `Write` | refused: `stale`, `not-owner`, `invalid-op` |
| `{t:"house", rev, floors:{id:{rev,doc,calibration}}}` | down | `WriteLatest` | the full current house, latest-state-wins |

Ops are small verbs over the document — `set-point`, `set-wall`, `del-wall`,
`set-opening`, `del-opening`, `set-room`, `del-room`, `set-label`, `del-label` —
batched per pointer-up (a stroke), never per mousemove.

**Why the downstream snapshot is the whole house.** `WriteLatest` is a depth-1
mailbox per socket: the newest undelivered frame wins. Coalescing is therefore
only correct if any frame fully supersedes the one it replaces — which a
per-floor delta does not (two floors' frames would eat each other), so the
coalesced frame is the complete current house. Traced houses are tens of KB of
JSON at stroke rate, so "resend everything that matters" is cheaper than being
clever, and a watcher who stalls for a minute reconnects their eyes to *now*
with zero replay — the driftbottle `/sockets` behavior, doing product work.

Editors without a working WebSocket degrade: `trace.js` falls back to the
`POST /api/floors/{id}/geometry` action on an interval and on blur. Both paths
end in the same `internal/geometry.Apply`.

### SSE — hub topics (the hypermedia)

Everything that changes *on screen outside the islands* is a rendered fragment
over a `Stream`, per doctrine. Topic scope == authorization scope:

| Topic | Audience | Fragments |
| --- | --- | --- |
| `house:<slug>` | viewers of a public/unlisted house page | presence chip ("2 watching"), "saved · rev n" chip, title/meta morphs |
| `shelf` | the home page | a re-rendered house card when a house publishes or its cover updates |

Private houses get **no shared topic** — the only person entitled to see them
is the owner, whose own actions already patch their DOM. Unlisted rides
`house:<slug>` because the slug *is* the audience capability. Nothing
viewer-dependent is ever rendered onto a shared topic.

## Routes

| Method | Path | Shape | Guard | Role |
| --- | --- | --- | --- | --- |
| GET | `/` | Page | — | the shelf: public houses, hero, your houses when signed in |
| GET | `/h/{slug}` | Page | visibility (in handler) | house page: dollhouse island + provenance |
| GET | `/h/{slug}/edit` | Page | `Authed()` + owner | the tracing room |
| GET | `/h/{slug}/doc.json` | Raw | visibility | island bootstrap: docs + calibrations, ETag by rev |
| GET | `/h/{slug}/live` | Stream | visibility | subscribe `house:<slug>` (+ `shelf` on `/`) |
| GET | `/ws/h/{slug}` | Socket | visibility; ops owner-only | the geometry channel |
| POST | `/houses` | Action | `Authed()` | create → `Patch{Redirect: /h/{slug}/edit}` |
| POST | `/api/houses/{id}` | Action | owner | meta: title, year, place, visibility |
| POST | `/api/houses/{id}/floors` | Action | owner | add a storey |
| POST | `/api/floors/{id}` | Action | owner | name, height, scan assignment, calibration |
| POST | `/api/floors/{id}/geometry` | Action | owner | full-document save — the no-WS fallback |
| POST | `/h/{slug}/scans` | Raw | owner | multipart scan upload → 303 back to the editor |
| POST | `/h/{slug}/cover` | Raw | owner | `image/png` body: the viewer island's rendered cover |
| GET | `/files/…` | Raw | — | `storage.Local` self-serve (dev backend only) |
| GET/POST | `/login`, `/logout`, `/register` | Raw | — | plain form → 303 (the CSP-safe auth flow) |
| GET | `/specimen` | Page | — | house rule: every app mounts the specimen |

Auth is the framework kit end to end: `driftwood.AuthModal`, plain POSTs with a
303 redirect (Datastar's script-based redirect is CSP-blocked — the askmuskie/
pantry precedent), `?ae=&at=` error round-trip. No RBAC beyond ownership rows —
every registered user may build houses; an `admin` role with `saltbox:moderate`
can hide a house from the shelf, and that is the whole permission story.

## Uploads & storage

The multipart pattern is hush's, on the framework store instead of a bespoke
one. `POST /h/{slug}/scans` is a plain multipart form (Datastar actions carry
JSON signals, not files): `r.FormFile("sheet")` → sniff MIME + decode
dimensions (`image.DecodeConfig`, JPEG/PNG only at v1) → cap at
`MAX_SCAN_MB` (default 20) → `Store.Put` under `scans/<house>/<rand>.<ext>` →
insert `sb_scans` → 303 back to the editor.

```go
// dev: zero config, serves itself
files, _ := storage.NewLocal(cfg.DataDir, "/files")
app.Raw(http.MethodGet, "/files/", http.StripPrefix("/files/", files))

// prod: same interface, R2 — the one-line swap storage was built to prove
files, _ = s3.New(s3.Config{Endpoint: cfg.R2Endpoint, Bucket: cfg.R2Bucket, …})
```

Covers are the same store (`covers/<house>.png`): after a save settles, the
owner's dollhouse island renders one frame to a PNG blob and POSTs it to
`/h/{slug}/cover` — the shelf card and `og:image` come from there. No headless
browser, no server-side renderer: the one client guaranteed to have the house
loaded takes the picture.

## The dollhouse build (client)

`dollhouse.js` is a three.js island (vendored ESM, the askmuskie precedent)
that rebuilds meshes from the document — no geometry ever computed server-side.

- **Walls without CSG.** Each wall is axis-boxes, never boolean ops: an opening
  splits its wall run into left segment · lintel above (head→ceiling) · right
  segment, windows add a sill box (floor→sill). Pure `BoxGeometry`, watertight
  by construction, trivial to rebuild per `house` frame.
- **The scan is the carpet.** Each storey's slab is textured with its calibrated
  scan crop — the original drawing lies inside the model, which is the whole
  sentiment of the product. Floors without scans get plain card.
- **Dollhouse conventions.** No roof, ever. Storey lift slides upper floors
  apart along Z (the explode view). Cutaway ghosts walls whose outward normal
  faces the camera (the classic dollhouse-game trick) so orbiting always looks
  *into* rooms. Matte card material, one warm key light, soft contact shadow.
- **Rooms & labels** render as floating serif tags at room centroids, fading by
  camera distance.
- `trace.js` (canvas 2D) and `dollhouse.js` share the document parsing and the
  wall-segmentation math in one module — the editor's minimap and the viewer
  must never disagree about what a door is.

## Config

`pkg/config` env loader; missing required vars exit with the list, no degraded
mode:

| Var | Required | Default | Role |
| --- | --- | --- | --- |
| `DATABASE_URL` | ✔ | — | Postgres (system of record) |
| `PORT` | | `8080` | listen port |
| `DATA_DIR` | | `./data` | `storage.Local` root (dev backend) |
| `R2_ENDPOINT` / `R2_BUCKET` / `R2_KEY_ID` / `R2_SECRET` / `R2_PUBLIC_URL` | | — | set all → the s3 backend replaces Local |
| `MAX_SCAN_MB` | | `20` | multipart upload cap |
| `RELEASE` | | `false` | release mode (asset versioning, cookie flags) |

Target run (once built — mirrors askmuskie):

```
docker compose up -d db
go build -o bin/saltbox ./cmd/saltbox
DATABASE_URL=postgres://saltbox:saltbox@localhost:5440/saltbox PORT=8098 ./bin/saltbox
```

Migrations run on boot (goose, embedded); `make gen` after touching `.templ`.

## Milestones

Each milestone ends in a browser, per the house rule: a rendered page is not a
working feature.

**M0 — the slab.** Auth, create house, upload a sheet, calibrate, and a 3D slab
carrying the scan texture under orbit. *Accepts when:* a fresh user goes
scan → orbiting slab in under two minutes, and a known 3.6m doorway measures
3.6m ± 2% in plan space.

**M1 — tracing & extrusion.** The pen: walls/doors/windows/rooms with
axis+endpoint snapping, undo, the document persisted with revs; extrusion with
openings; storey stacking with onion-skin. *Accepts when:* the demo 1887
cottage (both storeys) traces in under 20 minutes; reload reproduces the
identical document at the same rev; door gaps show lintels; no CSG in the tree.

**M2 — the shelf & the wire.** Visibility, covers, the public shelf with live
cards; the socket channel: watch mode, presence + save chips over SSE,
`doc.json` bootstrap, the action fallback. *Accepts when:* a stroke in one
browser lands in a second within 1s of pointer-up; a stalled watcher
(WebSocketStream, the driftbottle methodology) coalesces and snaps to the
current rev on release; publishing a house morphs a card into the shelf with no
reload.

**M3 — keepsakes.** Storey lift + cutaway polish, revision restore points, GLB
export (client-side `GLTFExporter`), a print sheet — the tracing re-rendered as
a clean measured line drawing. *Accepts when:* the exported GLB opens in an
external viewer with a 2.10m door head measuring 2.10m; restoring a checkpoint
rolls the document back as a **new forward rev**.

## Deliberately not there

- **Multi-editor tracing.** One owner, one pen. Genuine multi-writer-same-field
  editing is CRDT territory — a scoped island with its own RFC, never the sync
  path ([20-sync](../../docs/architecture/20-sync.md)). Watch mode is the
  collaboration story: everyone sees, one person draws.
- **Watcher cursor dots.** The coalescing mailbox is depth-1: a cursor frame
  and a house frame would supersede *each other*, and cursors are not worth a
  second lane. Strokes appearing on pointer-up **is** the live feeling.
- **Auto-tracing / CV wall detection.** The app is a pencil, not a scanner —
  hand-tracing an old plan is the pleasure, and OCR-grade wall extraction from
  a stained 1904 scan is a research project wearing a feature's coat.
- **TIFF/PDF ingest.** Archival TIFFs and PDF plan books convert fine outside;
  v1 decodes what browsers decode (JPEG/PNG).
- **Roofs.** A dollhouse is open at the top. (Also: roof geometry from plan
  tracings is guesswork; refusing is more honest than inventing hips.)
- **Furniture, materials, textures** beyond card + the scan. The scan is the
  decoration.
- **Offline editing** — the explicit hypermedia exit condition; not our app.
- **ClickHouse.** Postgres-only, deliberately — the second app to prove the
  smaller half of the stack rule.
- **Mobile tracing.** Viewing works everywhere; the pen wants a desk.

## Open questions

- **Revision retention** — is 20 checkpoints × 50 revs the right restore
  granularity, or should the op log itself be archived (append-only, cheap)?
- **Units** — documents are metric; surface an imperial *display* toggle for
  US archive material (the data never changes)?
- **WebP/AVIF scans** — worth the decode dependency once real archives arrive?
- **Per-wall height overrides** — enough for split-levels and mezzanines, or a
  trap better refused until a real plan demands it?
- **Unlisted + crawlers** — `noindex` on unlisted pages is table stakes; is the
  slug alphabet (6 × base32) enough capability entropy, or 8?
