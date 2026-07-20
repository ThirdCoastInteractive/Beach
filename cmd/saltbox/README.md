# saltbox — historic floorplans, rebuilt in miniature

Upload a scan of a historic floorplan — a fire-insurance sheet, a county-archive
blueprint, the pencil plan of your grandparents' farmhouse — calibrate its scale,
and trace the walls with a snapping pen. saltbox extrudes the tracing into a 3D
**dollhouse**: open-topped rooms cut from white card, the original scan lying on
each floor like period wallpaper, storeys you can lift apart, a slow orbit you
can share with a link.

The pen is assisted: a vision model **proposes most of the walls** and the pen
approves; OCR reads the sheet's printed scale and room dimensions; every length
**quantizes to the inches the original draftsman used**, so the parlor comes out
16'0" wide, not 4.87 m. And every finished tracing feeds **the archive** — a
filterable library of plans (bedrooms, bathrooms, square footage, storeys, year,
collection) built from real sheets: Sanborn surveys, pattern books, kit-home
catalogs, the shoebox in the attic.

(A *saltbox* is the lean-to-roofed house form of the 17th–19th-century New
England coast — a historic house that is also, conveniently, a little box.)

> **Status: plan of record — nothing below is built yet.** This README is the
> design; `mockups/` holds static HTML mockups of the five surfaces. When
> implementation starts, this doc gets rewritten section by section into the
> as-built description, the way the other app READMEs read.

## The loop

1. **Found a plan.** Create a house (title, year, place) and upload the scan —
   JPEG/PNG, one sheet per storey. Or skip the attic entirely: **adopt an
   untraced sheet** straight from a collection in the archive.
2. **Crop & calibrate.** A catalog page is rarely one storey: crop each plan
   region — two storeys stacked, side by side, or "Plan No. 1 / No. 2"
   variants — and assign it; the perspective rendering crops into the house's
   portrait. Then scale: OCR offers the page's printed size caption ("Width,
   21 feet 6 inches") or a dimension line ("34'-0""); otherwise draw one line
   along something whose length you know and type it — `11' 10"` or `3.6m`,
   the parser takes both. Dominant-axis detection suggests the deskew rotation.
3. **Trace.** The vision pass drafts ghost walls; you accept, redraw, or ignore
   them, then finish with the pen — walls chain and snap, doors and windows
   punch onto walls, rooms are named (*Parlor*, *Scullery*) and kinded
   (bedroom, bath) for the archive's filters.
4. **True up.** Lengths quantize to the sheet's native inches; typed and
   OCR-read dimensions become hard constraints; the solver nudges the wall
   graph onto round feet and reports the residual.
5. **Stack.** One floor per sheet; onion-skin the storey below to keep the
   stairs and chimneys aligned.
6. **Dollhouse.** The tracing extrudes live: card walls, scan-papered floors, no
   roof (a dollhouse is open at the top — that is the whole charm). Orbit, lift
   the storeys apart, ghost the near walls to see in.
7. **Shelve it.** Publish, and the house joins the shelf *and* the catalog —
   its bedrooms, baths, and square footage counted from the tracing itself.
   Watchers see strokes land live while you work.

## UI mockups

Static, self-contained HTML — open directly in a browser. They define the look:
**a drafting sheet, not a dashboard.** Aged paper, india ink, Sanborn carmine, a
title block in the corner of every surface. The built app renders these as templ
views + the client-JS islands.

| File | Surface |
| --- | --- |
| [mockups/01-shelf.html](mockups/01-shelf.html) | The shelf — public gallery of finished dollhouses, live cards |
| [mockups/02-scan.html](mockups/02-scan.html) | Scan & calibrate — upload strip, OCR scale suggestion, the scale line |
| [mockups/03-trace.html](mockups/03-trace.html) | The tracing room — pen, **ghost wall proposals**, assists panel, imperial HUD |
| [mockups/04-dollhouse.html](mockups/04-dollhouse.html) | The dollhouse — extruded card model (CSS-3D stand-in for the three.js island), storey lift |
| [mockups/05-catalog.html](mockups/05-catalog.html) | The catalog — the archive with facet filters and untraced sheets to adopt |

## Where it sits in the family

The examples each stress a slice of the framework; saltbox is a product-shaped
app (the askmuskie precedent) that picks up slices nothing else exercises for
real:

- **`storage` ●●** — first consumer outside hush, and the first to use the
  advertised one-line backend swap: `storage.Local` under dev, `storage/s3`
  (R2) in production, same `Store` interface, no app change. The archive's
  import pipeline makes it the biggest store by object count.
- **`App.Socket` ●●** — first *bidirectional* production consumer (driftbottle's
  `/sockets` is a demo lane): ops upstream on the ordered `Write` lane,
  house snapshots downstream on the `WriteLatest` coalescing mailbox.
- **`quantity` ●●** — first product consumer of the unit-system story:
  documents store metres, `quantity.Middleware` resolves the viewer's system
  (the `unit_system` cookie), every on-screen length goes through
  `quantity.Format`, and the dimension input parses through `quantity.Parse`
  (plus a small upstream addition: the compound `12' 6"` feet-inches form).
- Also exercised: hub/SSE chrome liveness, `auth` + sessions, `pg`
  (Postgres-only stack, the booking-manager half of the stack rule), driftwood
  + specimen, and heavy client-JS islands (canvas 2D + WASM inference +
  three.js) on the SSR core — the hush/askmuskie island pattern taken further.

## Architecture

The whole app is the two-lane doctrine from
[20-sync](../../docs/architecture/20-sync.md) drawn as a product. **SSE carries
hypermedia** — every chip, card, and panel that changes on screen arrives as a
rendered fragment over a hub topic. **The socket carries what is not
hypermedia** — the geometry document, which belongs to the canvas and the
three.js island, never to the DOM patcher. The vision assists live entirely
client-side: a proposal is not state until the pen accepts it, at which point
it becomes ordinary ops on the ordinary wire.

```mermaid
flowchart LR
    subgraph browser["Browser"]
        SSR["SSR chrome — templ + Datastar<br/>shelf · catalog · house · panels"]
        TRACE["trace.js island<br/>pen · proposals layer · true-up"]
        VIS["vision.js (WASM)<br/>wall U-Net · OCR · deskew"]
        DOLL["dollhouse.js island<br/>three.js extrusion"]
    end
    subgraph proc["saltbox process"]
        WEB["internal/web<br/>Page / Action / Stream / Raw"]
        SOCK["App.Socket /ws/h/{slug}<br/>internal/live — house rooms"]
        GEO["internal/geometry<br/>validate → apply → rev++ → facets"]
        MEDIA["internal/media<br/>keys · limits · dimensions"]
        HUB["hub<br/>topic fan-out"]
    end
    IMP["saltbox import (CLI, off-process)<br/>internal/archive — collections"]
    PG[("Postgres<br/>sb_houses (facets) · sb_floors<br/>sb_scans · sb_collections · sb_revisions")]
    OBJ[("storage.Store<br/>Local (dev) / R2 (prod)<br/>scans + covers")]

    VIS -- "suggestions only<br/>(ghost walls, read dims)" --> TRACE
    SSR -- "pages + Datastar actions<br/>(catalog filters = query params)" --> WEB
    HUB -- "SSE fragments<br/>GET /h/{slug}/live" --> SSR
    TRACE -- "accepted ops (JSON, Write)" --> SOCK
    SOCK -- "house snapshot<br/>(WriteLatest, latest-wins)" --> TRACE
    SOCK -- "house snapshot" --> DOLL
    SSR -- "multipart scan upload (Raw)" --> MEDIA
    MEDIA --> OBJ
    SOCK --> GEO
    GEO -- "debounced persist<br/>+ derived facet columns" --> PG
    WEB --> PG
    SOCK -- "saved / presence chips" --> HUB
    IMP --> OBJ
    IMP --> PG
```

One stroke, end to end (an accepted proposal takes exactly this path — the
model never writes):

```mermaid
sequenceDiagram
    participant E as trace.js (owner)
    participant S as App.Socket handler
    participant G as internal/geometry
    participant P as Postgres
    participant H as hub
    participant W as watcher (dollhouse.js)

    E->>S: {t:"op", floor, base:213, ops:[set-wall …]}  (pointer-up / accept)
    S->>G: apply(doc, ops) — validate, base check
    G-->>S: doc', rev 214 (+ recomputed facets)
    S-->>E: {t:"ack", floor, rev:214}  (Write, ordered)
    S-->>W: {t:"house", rev:214, …}    (WriteLatest — a slow watcher skips straight here)
    S->>P: UPDATE sb_floors + sb_houses facet columns  (debounced ~2s)
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
│                         (also the `saltbox import` subcommand entry)
├── config.go             typed env loader (pkg/config)
├── migrations/           goose; framework auth.Migrations + session.Migrations run ahead
├── internal/
│   ├── web/              app struct, handlers, views.templ (shell, shelf, catalog, house, editor)
│   ├── store/            domain types + sqlc over the sb_* tables (incl. facet queries)
│   ├── geometry/         the traced document: types, validate, apply(ops), rev, quantize/
│   │                     true-up solver, facet derivation — pure Go, zero beach imports,
│   │                     exhaustively unit-tested
│   ├── live/             per-house rooms: socket registry, op serialization,
│   │                     debounced persist, snapshot broadcast, hub chip publishes
│   ├── media/            storage wiring: key scheme, size caps, MIME sniff, DecodeConfig dims
│   └── archive/          collections + the import pipeline (TIFF→web derivative transcode,
│                         sheet metadata; the one place the batch-inference dep may live)
├── static/js/            trace.js, vision.js, dollhouse.js, vendored three.js ESM;
│                         beach-ws.js comes from the framework's embedded assets
├── static/models/        quantized ONNX wall model + OCR data, versioned, cache-immutable
├── floorplans/           the seed corpus: 929 catalog pages, 25 collections (see The archive)
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
    sb_collections ||--o{ sb_scans : gathers
    sb_houses |o--o{ sb_scans : keeps
    sb_houses ||--o{ sb_floors : stacks
    sb_scans |o--o{ sb_floors : underlays
    sb_floors ||--o{ sb_revisions : checkpoints
```

```sql
-- saltbox domain schema: collections of source sheets, houses on the shelf,
-- the traced storeys (geometry lives here, one jsonb document per floor),
-- append-only revision checkpoints, and derived facet columns for the catalog.

-- +goose Up

-- A source collection: a coherent body of sheets imported together ("Sears
-- Modern Homes 1908", "Sanborn Galveston 1904"). License is recorded per
-- collection; the import pipeline refuses sources without one.
-- +goose StatementBegin
CREATE TABLE sb_collections (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug       text        NOT NULL UNIQUE,
    title      text        NOT NULL,
    source     text        NOT NULL DEFAULT '',   -- "Library of Congress", URL in source_url below
    license    text        NOT NULL,              -- "public domain", "CC0", …
    year_from  int,
    year_to    int,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- A house: one traced building, owned by one user. slug is a short random id
-- and doubles as the capability for 'unlisted' visibility. The facet columns
-- are DERIVED — recomputed by internal/geometry on every persist, never
-- user-entered — and NULL until something is traced.
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
    native_units text       NOT NULL DEFAULT 'ft',    -- ft|m: the sheet's own units,
                                                      -- drives quantize increments
    cover_key   text        NOT NULL DEFAULT '',      -- storage key of the cover snapshot
    portrait    jsonb       NOT NULL DEFAULT '{}',    -- {scan_id, crop}: the catalog's
                                                      -- perspective rendering, shown beside
                                                      -- the dollhouse on the house page
    -- derived facets (the catalog filters on these):
    storeys     int,
    rooms       int,
    bedrooms    int,
    bathrooms   int,
    area_m2     real,                                 -- displayed via quantity.Format (sq ft / m²)
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sb_houses_shelf_idx
    ON sb_houses (visibility, updated_at DESC) WHERE deleted_at IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sb_houses_facets_idx
    ON sb_houses (bedrooms, bathrooms, storeys, built_year, area_m2)
    WHERE visibility = 'public' AND deleted_at IS NULL;
-- +goose StatementEnd

-- An uploaded or imported sheet. Bytes live in the object store under
-- storage_key; the row is metadata. house_id is NULLABLE: an archive sheet
-- exists before anyone adopts it into a house, and adoption just sets the FK.
-- +goose StatementBegin
CREATE TABLE sb_scans (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    house_id      bigint      REFERENCES sb_houses (id) ON DELETE SET NULL,
    collection_id bigint      REFERENCES sb_collections (id) ON DELETE SET NULL,
    sheet_no      text        NOT NULL DEFAULT '',    -- "sheet 12", "design No. 158"
    model_name    text        NOT NULL DEFAULT '',    -- "The Ashmore", "Beaumont"
    role          text        NOT NULL DEFAULT 'page',-- page|plan|portrait|cover — what the
                                                      -- image is (the d/p import pairing)
    source_url    text        NOT NULL DEFAULT '',    -- deep link into the source archive
    storage_key   text        NOT NULL,               -- scans/<collection|house>/<rand>.<ext>
    filename      text        NOT NULL,
    content_type  text        NOT NULL,               -- image/jpeg | image/png (web derivative)
    width_px      int         NOT NULL,
    height_px     int         NOT NULL,
    bytes         bigint      NOT NULL,
    ocr           jsonb       NOT NULL DEFAULT '{}',  -- cached OCR pass: scale note, dim strings
    uploaded_at   timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sb_scans_unadopted_idx
    ON sb_scans (collection_id, sheet_no) WHERE house_id IS NULL;
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
frame), always saved whole, and never queried across houses. Normalizing walls
into rows would buy per-wall UPDATE granularity nobody needs and cost a
join-and-reassemble on the hottest read. The one thing the catalog *does* query
across houses — bedrooms, baths, area, storeys — is exactly what the derived
facet columns on `sb_houses` denormalize at persist time. The document is
validated by `internal/geometry` on every write; jsonb here is a typed document
with one writer, not a schema escape hatch.

### The geometry document

Stored in `sb_floors.geometry`, validated and mutated only by
`internal/geometry.Apply`. Coordinates are metres in plan space (calibration
maps scan pixels → metres; the document never stores pixels — quantize makes
the metric floats *exact in inches*, e.g. 12'6" = 3.81 m):

```jsonc
{
  "points":   { "p1": [0.0, 0.0], "p2": [7.3152, 0.0] },     // id → [x, y] metres
  "walls":    [ { "id": "w1", "a": "p1", "b": "p2", "t": 0.1524 } ],   // t = thickness
  "openings": [ { "id": "o1", "wall": "w1", "at": 0.55,      // 0..1 along the wall
                  "w": 0.9144, "kind": "door",               // door | window
                  "sill": 0.0, "head": 2.1336, "swing": "in-left" } ],
  "rooms":    [ { "id": "r1", "name": "Parlor",              // the historic name — the charm
                  "kind": "parlor",                          // the facet — see the kind table
                  "loop": ["p1","p2","p6","p5"] } ],
  "labels":   [ { "id": "l1", "xy": [4.1, 2.0], "text": "porch (unheated)" } ],
  "dims":     [ { "id": "d1", "axis": "x", "a": "p1", "b": "p2",
                  "len": 7.3152, "src": "typed" } ]          // hard constraints: typed | ocr
}
```

Walls share endpoints through the point table, so dragging a corner moves every
wall that meets it — the snap graph is the data model. Wall height comes from
the floor (`height_m`); a per-wall `h` override is allowed for the odd
half-height parapet.

`sb_floors.calibration` is `{ "crop": [x, y, w, h], "ppm": 120.4,
"origin": [x, y], "rot": -0.013, "opacity": 0.4 }` — a page *region* plus
enough to place it under the tracing and as the floor texture, nothing more.
`crop` is what lets one catalog page underlay two storeys: each floor
references the same scan with its own crop (Standard Homes and 1935 Sears
stack both plans on one sheet; Hodgson prints them side by side; Aladdin and
the 1956 ranches print "Plan No. 1 / No. 2" *variants* — crop the one you are
building).

**Room kinds** power the catalog facets. The historic *name* is kept verbatim;
the *kind* is suggested from it and always overridable: Chamber / Bed Room →
`bedroom`; Bath / W.C. / Privy → `bathroom`; Scullery / Pantry / Kitchen →
`kitchen`; Parlor / Sitting Room → `parlor`; plus `dining`, `hall`, `porch`,
`closet`, `other`. Facet derivation at persist: `bedrooms` = count of kind
bedroom across floors, `bathrooms` likewise (half-baths count — it's 1890,
be glad there's plumbing), `rooms` = all kinds except porch/closet, `area_m2`
= Σ outer-loop area per floor, `storeys` = floor count.

## The seeing pencil — assists

Three assists stand between "upload" and "traced", in increasing order of
machinery. All three share one law: **an assist proposes; only the pen
commits.** Nothing an assist produces touches the document until the owner
accepts it, and acceptance turns into ordinary ops through the same
`internal/geometry.Apply` as hand tracing — the server cannot tell the
difference, and never trusts a model directly.

**Ground truth from the corpus.** The seed pages (`floorplans/`, see
[The archive](#the-archive)) are 300–650 px halftone catalog scans, not
archival TIFFs: a page mixes a perspective rendering, one or two plan regions,
a printed size caption, room dims, price, and copy — and one pixel is an inch
or more of house. At that resolution the printed numbers are load-bearing:
pixel-measuring the ~200 px plan region of a 21'6"-wide Radford cement house
cannot beat reading "Width, 21 feet 6 inches" off the caption. Quantize and
printed-dimension constraints are the accuracy mechanism, not garnish — the
pixels say *where*, the type says *how big*. (The wall model trains against
halftone screening and the occasional colour page — the 1956 ranches.)

### Quantize — the sheet's own inches

Historic American sheets are feet-and-inches documents; the tracing should be
too. `native_units` on the house (default `ft`) drives:

- **Live snapping.** The pen snaps to a 1" grid in plan space, magnetically at
  3" / 6" / 1'0" multiples, and to 0°/45°/90° runs. A traced wall lands on
  12'0", not 3.9624071 m.
- **True-up (the solver).** Axis-aligned plans make quantization a pair of 1-D
  problems: cluster point coordinates per dominant axis (points sharing a wall
  line share a cluster), then snap each cluster's coordinate so wall runs and
  room widths land on round inches — preferring whole feet, then 6", then 3",
  then 1". Typed and OCR dimensions (`dims` above) are hard constraints; the
  solve is least-squares over the remaining clusters, and the residual is
  reported honestly ("off by ⅜″ over 40 ft — old houses are like that").
  One command, previewed as a ghost overlay, applied as one op batch, undoable.
- **Display.** Stored metres; every on-screen length renders through
  `quantity.Format`, so the same document reads 16'0" or 4.88 m by the
  viewer's `unit_system` cookie. The editor defaults the cookie to the house's
  native system on first open.

### Typed and read dimensions

- **The little text input.** Click any wall (or any room edge) and type its
  length — `12' 6"`, `12ft 6in`, `3.81m` — parsed by `quantity.Parse` (the
  compound feet-inches form is the small upstream addition). The value becomes
  a `dims` constraint: the wall is *declared*, the solver holds it, and the HUD
  marks it ✓ so declared dimensions are visually distinct from measured ones.
- **OCR.** A client-side pass (Tesseract.js WASM to start — printed lettering
  only; 1880s copperplate hand is out of scope) over the scan reads three
  narrow things, each surfaced as a confirmation chip, never auto-applied:
  1. **Size captions and dimension lines** — "Width, 21 feet 6 inches; Length,
     43 feet 6 inches" (the Radford/Hodgson caption convention) or an in-plan
     dimension line reading "34'-0"" becomes a one-click calibration; a
     printed "Living Area 1,325 sq. ft." lands as an est-hint on the sheet.
  2. **The scale note** — "SCALE ⅛ IN = 1 FT" on a sheet margin, same chip.
  3. **Room dimension strings** — "16' × 13'" inside a room becomes a pair of
     `dims` constraints attached to that room's loop ("read 5 room dims ✓").
  4. **Room names** — "PARLOR" seeds the room's name and suggested kind when
     you close its loop.
  The raw OCR result is cached on `sb_scans.ocr` so re-opening the editor
  doesn't re-run it.

### Wall proposals — the model drafts, you approve

The marquee assist: a semantic-segmentation model finds the walls and drafts
most of the tracing.

- **Model.** A U-Net-class wall/opening segmenter. Starting point: train on
  CubiCasa5k-style annotated floorplans, then fine-tune on what this app
  uniquely generates — accepted historic tracings are pixel-perfect labeled
  pairs (scan + vector truth), opt-in, and nothing else like that corpus
  exists. (License caveat in open questions: CubiCasa5k is research-licensed,
  so the shipping weights may need training from public-domain plans only.)
- **Runtime.** `vision.js` runs the quantized model (~10 MB int8 ONNX, served
  from `static/models/`, immutable-cached) via WASM in the browser, tiled
  512 px windows with overlap, a sheet in ~5–15 s on a desktop — it runs while
  you calibrate. No server inference, no GPU dependency, the strict-CSP
  same-origin story intact.
- **Vectorize.** Mask → skeleton → segment tracing → merge collinear runs →
  snap to the two dominant axes (the axis histogram also yields the deskew
  angle step 2 suggests) → wall thickness from the distance transform →
  openings where the mask breaks with door-arc evidence. Output: a **proposal
  layer** of ghost-blue walls with per-run confidence.
- **The accept pass.** Proposals render dashed under the tracing. Accept all,
  lasso a region, or just draw — the pen inks over a proposal to accept it,
  which is the fastest correction loop of all. A confidence slider hides the
  model's guesses below the bar. Accepted runs become `set-point`/`set-wall`
  ops; rejected ones evaporate client-side and are never seen by the server.
- **Batch lane.** The archive importer can run the same ONNX model server-side
  to pre-compute proposal layers for whole collections (stored as sidecar JSON
  in the object store, not in the document). That is the one place a native
  onnxruntime dependency may live — confined to `internal/archive` behind the
  import subcommand; the serving binary never links it.

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
`set-opening`, `del-opening`, `set-room`, `del-room`, `set-label`, `del-label`,
`set-dim`, `del-dim` — batched per pointer-up (a stroke), per accept pass, or
per true-up, never per mousemove. Assists add no frames: proposals live
client-side until acceptance turns them into ops.

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
| `shelf` | the home page and the catalog | a re-rendered house card when a house publishes or its cover updates |

Private houses get **no shared topic** — the only person entitled to see them
is the owner, whose own actions already patch their DOM. Unlisted rides
`house:<slug>` because the slug *is* the audience capability. Nothing
viewer-dependent is ever rendered onto a shared topic.

## Routes

| Method | Path | Shape | Guard | Role |
| --- | --- | --- | --- | --- |
| GET | `/` | Page | — | the shelf: public houses, hero, your houses when signed in |
| GET | `/catalog` | Page | — | the archive: facet filters ride the query string as plain links (`?beds=3&baths=1&sqft=700-1600&storeys=2&year=1880-1920&coll=sears-1908&untraced=1`) |
| GET | `/h/{slug}` | Page | visibility (in handler) | house page: dollhouse island + provenance |
| GET | `/h/{slug}/edit` | Page | `Authed()` + owner | the tracing room |
| GET | `/h/{slug}/doc.json` | Raw | visibility | island bootstrap: docs + calibrations, ETag by rev |
| GET | `/h/{slug}/live` | Stream | visibility | subscribe `house:<slug>` (+ `shelf` on `/` and `/catalog`) |
| GET | `/ws/h/{slug}` | Socket | visibility; ops owner-only | the geometry channel |
| POST | `/houses` | Action | `Authed()` | create → `Patch{Redirect: /h/{slug}/edit}` |
| POST | `/sheets/{id}/adopt` | Action | `Authed()` | adopt an untraced archive sheet: creates the house around it → redirect to calibrate |
| POST | `/api/houses/{id}` | Action | owner | meta: title, year, place, visibility, native units |
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
can hide a house from the shelf and quarantine a collection, and that is the
whole permission story.

## The archive

The catalog is the long game: a **filterable library of historic floorplans**
where every entry is (or can become) a real 3D model, and the filters are
computed from tracings, not typed into a form.

- **Collections** are the unit of import: a coherent public-domain body of
  sheets (`sb_collections` + `sb_scans` rows with `house_id IS NULL`). The
  first 25 are already in the tree (the seed corpus, below); candidate seeds
  beyond it: Library of Congress Sanborn sheets, HABS survey drawings. License
  is recorded per collection and the importer refuses sources without one.
- **`saltbox import`** (a subcommand of the same binary, run off-process) takes
  a collection manifest (source URLs or a directory, license, year range),
  transcodes originals to web derivatives — this is where TIFF/JP2 masters get
  decoded, the browser only ever sees JPEG/PNG — uploads to the object store,
  inserts the rows, and optionally pre-computes OCR + wall-proposal sidecars
  for the tracing room to pick up instantly.
- **Adopting a sheet.** The catalog shows untraced sheets alongside finished
  houses (a toggle). "Adopt this sheet" creates a house around the scan and
  drops you straight into calibrate — the archive is a to-do list wearing a
  library's coat.
- **Facets.** `?beds=&baths=&sqft=&storeys=&year=&coll=&untraced=` — plain
  links, pantry/booking style, no client state. Backed by the partial facet
  index on `sb_houses`; untraced-sheet queries hit `sb_scans_unadopted_idx`.
  Facet values come only from accepted tracings, so a filtered result is a
  *claim about the plan*, not about someone's data entry.
- **Counts on the rail** ("Sears Modern Homes 1908 · 447") are one grouped
  query, cached with `cache.Snapshot` and invalidated on publish.

### The seed corpus (in-tree)

`floorplans/` holds the starter corpus: **929 catalog pages** from the saved
*Old Houses For Sale* catalog captures in `Y:\research\old_floorplans/` (25
saved pages, HTML + images). The import manifest for this corpus should parse
the **saved HTML pages, not just the images** — the pages carry model names,
captions, and the `d`/`p` (portrait/plan) pairing as text, so the importer
recovers `model_name`, `role`, and est-hints without OCR. Filename prefixes
map to collections:

| Prefix | Collection | Year |
| --- | --- | --- |
| `03r-` | Radford American Houses (100 designs) | c. 1903 |
| `h-` | Hodgson House Plans | 1905 |
| `r1-` / `r2-` | Radford Cement Houses, one-/two-story | 1909 |
| `16s…` | Sears Roebuck kit houses | 1916 |
| `16sterling…` | Sterling System (International Mill & Timber) | 1916 |
| `a-` | Aladdin | 1916 |
| `20<name>` | Bennett Homes | 1920 |
| `20h-m####` | Harris Homes | 1920 |
| `21f` | Fenner Ready Built (Portland) | 1921 |
| `24bw-` | Bilt-Well Homes of Comfort | 1924 |
| `25w` | Wardway (Montgomery Ward) | 1920s |
| `rm…` | Radford Home & Fireside | 1925 |
| `26<name>d/p` | Standard Homes | 1926 |
| `27b-` | Common Brick Manufacturers | 1927 |
| `se-` | Sears Honor Bilt supplement | 1928 |
| `s29…` | Sears Honor Bilt, brick veneer | c. 1929 |
| `35s…d/p` | Sears Honor-Bilt | 1935 |
| `l-…-d/p` | Liberty Homes (Lewis Mfg) | 1939 |
| `45s-` | Sterling | 1945 |
| `n<name>` | National Homes of Moderate Cost | 1949 |
| `53<name>` | Aladdin | 1953 |
| `56n-r###` | Modern Ranch Homes | 1956 |
| `gvt` | Gordon Van Tine | n.d. |
| `<name>d/p` (unprefixed) | Aladdin | 1961 — confirm vs saved page |

Licensing: everything published before 1931 is public domain outright; the
1935–1961 catalogs are almost certainly unrenewed commercial catalogs (also
PD) but each wants a renewal check before its collection goes on the public
shelf. Per-collection `license`, and the importer refuses blanks.

## Uploads & storage

The multipart pattern is hush's, on the framework store instead of a bespoke
one. `POST /h/{slug}/scans` is a plain multipart form (Datastar actions carry
JSON signals, not files): `r.FormFile("sheet")` → sniff MIME + decode
dimensions (`image.DecodeConfig`, JPEG/PNG at the web boundary) → cap at
`MAX_SCAN_MB` (default 20) → `Store.Put` under `scans/<house>/<rand>.<ext>` →
insert `sb_scans` → 303 back to the editor. Imported collection sheets take the
same path through `internal/archive` with keys under `scans/<collection>/…`.

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
- **Rooms & labels** render as floating serif tags at room centroids
  (historic name first, dimensions under it via `quantity.Format`), fading by
  camera distance.
- `trace.js` and `dollhouse.js` share the document parsing and the
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

The import subcommand is flag-driven (`saltbox import -manifest sears1908.yaml`),
not env-driven — it is tooling, not the server.

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

**M0 — the slab.** Auth, create house, upload a sheet, calibrate by hand, and a
3D slab carrying the scan texture under orbit. *Accepts when:* a fresh user
goes scan → orbiting slab in under two minutes, and a known 12' doorway wall
measures 12' ± 2% in plan space.

**M1 — tracing, quantized.** The pen with inch-grid snapping, walls/doors/
windows/rooms (names + kinds), undo, the document persisted with revs; the
typed dimension input (`quantity.Parse` + the feet-inches form); extrusion with
openings; storey stacking with onion-skin; imperial/metric display via the
`unit_system` cookie. *Accepts when:* the demo 1887 cottage (both storeys)
traces in under 20 minutes with every wall landing on round inches; a wall
declared `12' 6"` survives reload at exactly 3.81 m; door gaps show lintels;
no CSG in the tree.

**M2 — the shelf & the wire.** Visibility, covers, the public shelf with live
cards; the socket channel: watch mode, presence + save chips over SSE,
`doc.json` bootstrap, the action fallback. *Accepts when:* a stroke in one
browser lands in a second within 1s of pointer-up; a stalled watcher
(WebSocketStream, the driftbottle methodology) coalesces and snaps to the
current rev on release; publishing a house morphs a card into the shelf with no
reload.

**M3 — keepsakes.** Storey lift + cutaway polish, revision restore points, GLB
export (client-side `GLTFExporter`), a print sheet — the tracing re-rendered as
a clean measured line drawing with imperial dimension strings. *Accepts when:*
the exported GLB opens in an external viewer with a 7'0" door head measuring
2.13 m; restoring a checkpoint rolls the document back as a **new forward
rev**.

**M4 — the seeing pencil.** OCR assists (scale note, room dims, room names) and
the wall-proposal model in the browser: propose → accept pass → true-up solver
with hard constraints. *Accepts when:* on a clean pattern-book sheet the
proposal layer covers ≥ 70% of final wall length with < 5% spurious walls
after a one-minute accept pass; the OCR scale chip reads the printed scale on
at least half of a sample of LoC Sanborn sheets; nothing an assist produced
ever reached the server without an accepting op; true-up on the demo cottage
moves no point more than 1" and reports the residual.

**M5 — the archive.** Collections, `saltbox import` (TIFF/JP2 masters → web
derivatives, license manifest), pre-computed OCR/proposal sidecars, the catalog
page with facet filters and adopt-a-sheet. *Accepts when:* a 1,000-sheet
collection imports unattended; `?beds=3&baths=1&year=1900-1920` answers from
the partial index in under 100 ms; adopting a sheet lands in calibrate with the
scan already placed and its proposal sidecar loaded; every facet number on a
card is derived from a tracing, none typed.

## Deliberately not there

- **Multi-editor tracing.** One owner, one pen. Genuine multi-writer-same-field
  editing is CRDT territory — a scoped island with its own RFC, never the sync
  path ([20-sync](../../docs/architecture/20-sync.md)). Watch mode is the
  collaboration story: everyone sees, one person draws.
- **Fully-automatic tracing.** The model proposes; it never commits. A tracing
  whose every wall was accepted by a person is a *record*; an unattended model
  output is a guess wearing one's clothes. (Pre-computed sidecars for archive
  sheets are still just proposals — adoption starts the accept pass.)
- **Generative completion.** Where the scan is illegible — water stain, torn
  corner — the app refuses to invent walls. Trace what the sheet shows; leave
  the gap; note it with a label. An archive that guesses is not an archive.
- **Watcher cursor dots.** The coalescing mailbox is depth-1: a cursor frame
  and a house frame would supersede *each other*, and cursors are not worth a
  second lane. Strokes appearing on pointer-up **is** the live feeling.
- **Roofs.** A dollhouse is open at the top. (Also: roof geometry from plan
  tracings is guesswork; refusing is more honest than inventing hips.)
- **Furniture, materials, textures** beyond card + the scan. The scan is the
  decoration.
- **Offline editing** — the explicit hypermedia exit condition; not our app.
- **ClickHouse.** Postgres-only, deliberately — the catalog's facets are
  indexed columns, not analytics.
- **Mobile tracing.** Viewing works everywhere; the pen wants a desk.

## Open questions

- **Model licensing.** CubiCasa5k is research-licensed (non-commercial terms) —
  fine for prototyping the pipeline, not for shipping weights. The likely
  answer: bootstrap on it privately, ship weights trained on public-domain
  sheets + this app's own opt-in accepted tracings once the corpus exists. When
  does that corpus get big enough?
- **The training flywheel's consent.** Accepted tracings as training pairs is
  the app's structural advantage — opt-in per user, per house, or per
  collection? (Imported public-domain collections: clearly yes; personal
  attic scans: ask.)
- **Batch inference dependency.** Native onnxruntime (cgo) confined to the
  import tool vs. running the same WASM path in a worker — is the cgo exception
  worth the 10× speed on 10k-sheet collections?
- **OCR on hand lettering.** Tesseract handles printed sheets; 1880s copperplate
  defeats it. Is a dimension-string-specific recognizer (tiny vocabulary:
  digits, ′ ″ ×) worth training as part of the wall model's text head?
- **Revision retention** — is 20 checkpoints × 50 revs the right restore
  granularity, or should the op log itself be archived (append-only, cheap)?
- **Catalog thumbnails at 10k sheets** — covers are owner-posted PNGs, but
  untraced sheets need server-side thumbnails at import time; are the web
  derivatives enough or is a CF Images variant (the hush pattern) warranted?
- **Half-storeys.** "1½ storey" is a real and common historic form (the
  saltbox itself!) — does `storeys` stay an int with an attic flag, or become
  a half-step numeric?
- **Plan variants.** Aladdin and the ranch catalogs print "Plan No. 1 / Plan
  No. 2" alternates of one model on a single page. Is a variant a second
  house, a second crop on the same house, or a first-class concept the
  catalog groups by?
- **Estimated facets for untraced sheets.** Printed captions ("Living Area
  1,325 sq. ft.") could pre-fill est-hints so adoption-hunters can filter
  untraced sheets — but the facet columns' purity rule is "derived from
  tracings only." Display ests without filtering on them, or add
  clearly-marked est filters?
- **Unlisted + crawlers** — `noindex` on unlisted pages is table stakes; is the
  slug alphabet (6 × base32) enough capability entropy, or 8?
