# askmuskie

askmuskie.com — a shared, server-authoritative glass tank where a talking
muskellunge swims, takes questions from the live visitors gathered around it, and
speaks "profound" wisdom in a retro klattsch formant voice. He's also a little
Neopet: hunger, energy, and happiness tick in real time and drive his behavior
(he naps when tired, grazes when hungry, perks up when fed or asked). There is no
LLM — the wisdom is a flat file.

It runs in two modes:

- **No database** (the default): an anonymous pond. Visitors get a generated name
  and a default avatar derived from their session id; they can walk around, ask,
  and feed. The whole live world lives in memory, so a restart resets it.
- **With a database** (`DATABASE_URL` set): the *game*. Visitors can register and
  log in, customize their avatar (shape / body color / skin), chat (ToS-gated),
  and earn stats — wisdom received, food fed, chat messages — that rank them on a
  charts leaderboard. The pond itself is still in-memory; only accounts, chat
  history, and stats are persisted.

## Run

Anonymous pond, no dependencies:

```
go build -o bin/askmuskie ./cmd/askmuskie
PORT=8099 ./bin/askmuskie          # defaults to :8080
```

The full game needs Postgres. A local one is in `docker-compose.yml` (host port
5439):

```
docker compose up -d db
go build -o bin/askmuskie ./cmd/askmuskie
DATABASE_URL=postgres://askmuskie:askmuskie@localhost:5439/askmuskie PORT=8099 ./bin/askmuskie
```

Migrations run automatically on boot (goose, embedded in the binary). Open it in
two browsers (or one normal + one private window) to see the shared pond: both
visitors appear around the tank, the same Muskie swims for everyone, asking
enqueues you, and you watch each other walk around.

## How it works

- **`internal/pond`** is the authoritative sim: one goroutine (a ~20 Hz tick +
  command channel, no locks) owns the Muskie's position/heading inside the tank,
  a behavior FSM (`roam` → `approach` an asker at the near glass → `speak` → back
  to roam; `graze` dropped food when idle; `sleep` near the bottom when energy
  runs low), the living stats (hunger/energy/happiness), the FIFO speak queue
  (food buys priority), presence, the food economy, and each visitor's floor
  position and avatar appearance. A nil-safe `Recorder` lets it report wisdom-served
  and food-fed events to the stats writer for logged-in visitors.
- **Transport** is a raw JSON SSE stream (`GET /pond/stream`) fed by the hub — the
  framework's Datastar `Stream` only emits HTML patches, so the realtime game
  state uses the `app.Raw` escape hatch. Commands come back over `POST /pond/ask`,
  `POST /pond/food`, and `POST /pond/move`. Identity is the `am_sid` cookie
  (anonymous presence) bridged, when logged in, to the account via the session.
- **`static/js/muskie.js`** is the client: three.js renders the snapshot, drawing
  the glass tank, the Muskie inside, the other visitors as avatars around it,
  floating food, and the Neopet-style stat bars. Controls are **third-person**:
  the camera targets *you* (head height) and orbits behind, so "forward" is always
  into the screen — drag to look, scroll/pinch to zoom. **WASD moves
  camera-relative** and your avatar turns to face its travel direction (its face
  shows the heading); **clicking the floor walks you there**, with the camera
  easing in behind so you never walk off-screen. Your own avatar is
  client-predicted and your position is sent back throttled; the Muskie and the
  crowd are **snapshot-interpolated** (rendered slightly behind real time and eased
  between frames) so motion is smooth despite the low tick rate. Only the visitor
  being spoken to hears the klattsch voice (real audio-driven jaw); everyone else
  sees a muted jaw chatter + the quote.
- **`internal/speech`** converts each quote to ARPABET phonemes (embedded CMU
  Pronouncing Dictionary + a letter-to-sound fallback) for klattsch, which has no
  text front end of its own.

## Accounts, chat, and the leaderboard (database mode)

- **Auth** is built from reusable framework pieces: `pkg/auth.RegisterUser`
  (validate → hash → insert `users` + local credentials in one transaction) and
  `driftwood.AuthModal` (a login/register modal demoed in `/specimen`).
  `internal/authx` wires the session store, principal resolver, and cookie. Under
  the strict CSP, the auth forms are **plain POSTs with a 303 redirect** (not
  Datastar `@post`, whose inline-script redirect the CSP blocks); errors round-trip
  through a `?ae=&at=` query that reopens the modal.
- **Chat** (`internal/chat`) is logged-in-only (= ToS-accepted). It keeps an
  in-memory ring of recent messages for an instant backlog on connect, inserts to
  Postgres asynchronously, rate-limits per user, and broadcasts over a hub `chat`
  topic. "Be mean, just not illegal" — see `/tos`.
- **Stats** (`internal/stats`) is write-behind: `Wisdom`/`Fed`/`Chat` are
  non-blocking and a background loop batches the increments into
  `askmuskie_players` every couple of seconds.
- **Leaderboard** (`GET /leaderboard`) renders three top-10 `chart.LayoutHBar`
  bar charts (most wisdom, most generous, most talkative).
- **Avatar customization**: the "Customize" button opens a modal with three swatch
  grids (`POST /me/avatar` → update `askmuskie_players` → `pond.SetAvatar`, so the
  new look shows live for everyone). See *Avatars* below.

## Avatars

`static/js/shapes.js` is the body catalog — **20 curated solids**: the 5 Platonic
solids (three.js built-ins), 6 Archimedean and 5 prism/antiprism solids built as
the convex hull of a coordinate-formula vertex set (`ConvexGeometry`), and 4
pyramids/bipyramids. Every geometry is normalized to a common size. The head
carries a **face** (two eyes + a snout on the local +Z front) so an avatar's
heading is readable as it turns to face its travel direction.

- **Body color**: a 14-swatch RYBitten-flavored palette, picked by index, with a
  small deterministic per-user HSL jitter so two visitors who pick the same swatch
  still look individual. No color picker — swatch grid only.
- **Skin color** (the head): emoji yellow + the 5 Fitzpatrick tones + purple /
  blue / green alien.

`BODY_PALETTE` / `SKIN_PALETTE` lengths and the shape keys must stay in sync with
`internal/pond` (`bodyPaletteLen` / `skinPaletteLen`), which derives a default
appearance from the session id for anonymous visitors.

## Scale

It's built to take a few thousand concurrent visitors on one box. The trick is the
snapshot protocol: instead of broadcasting the whole roster to everyone every tick,
the world goes out as four frames over the one SSE stream —

- **hot** (shared, ~10 Hz, small): the Muskie + stats + current quote.
- **cold** (shared, ~1.5 Hz, capped): tank size, total count, a *sampled* ≤100 of
  the crowd *with their appearance* (shape/colors), food, queue.
- **moves** (shared, ~6–7 Hz, compact `[shortID, x, z]`): the sampled crowd's floor
  positions, so others animate smoothly between the heavier cold frames.
- **self** (per-visitor, ~1.5 Hz, small): your own food, position, appearance, and
  queue slot.

Chat adds a `chat` broadcast frame and a `chatlog` backlog frame sent on connect.
The client draws the crowd as capped individual avatar groups built from the
sampled roster (only your own avatar gets a name label), so the frame rate holds
even with a packed tank.

Load-test it with `cmd/askmuskie-load`:

```
go run ./cmd/askmuskie-load -url http://localhost:8099 -n 2000 -ramp 20s -dur 30s -ask 0.02 -food 0.05
```

Measured on one dev box: **1,000 visitors held at ~104 ms hot-frame p99, 0 errors**
(steady ~20k frames/s aggregate) with the moves frame in the mix; 2,000 held flat
at ~105 ms p99, ~237 MB server RAM, ~1 core. The raw SSE is uncompressed (RAM is
the limiter on one box, not loopback bandwidth); per-connection gzip is the lever
if a real deploy is bandwidth-bound.

## The oracle (optional LLM voice)

By default the Muskie speaks from `data/quotes.txt`. Set `MUSKIE_LLM_URL` (and
`MUSKIE_LLM_MODEL`) and a local Ollama sidecar voices him instead — one aphorism
per ask, generated from the **live tank state** (crowd size, his hunger/energy/
mood) and, for logged-in visitors, an **optional message** plus their lifetime
stats ("you who have fed me 51 times"). Unset the URL and it's flat-file as before.

How it's wired (`internal/oracle` + `internal/pond`):

- Generation is **async, off the sim goroutine**: it fires when the Muskie starts
  swimming over (`beginApproach`); the result lands on a channel the sim drains.
  He hovers at the glass up to a few seconds (`oracleGraceMs`) waiting for the
  line, then speaks it — or falls back to a flat-file quote. The swim-over hides
  the latency and the sim never blocks on the network.
- Output flows through the existing G2P → klattsch, unchanged.
- **Voices:** the model also picks a **voice** (Ollama *structured output* —
  `format` is a JSON schema `{aphorism, voice:enum}`, so even a small model can't
  emit garbage or an invalid name). The chosen voice rides the hot frame and the
  client (`muskie.js` `VOICES`) maps it to a klattsch directive prefix
  (`b`=pitch, `r`=rate, `s`=formant scale, `g`=effort, `h`=breath, `v`/`m`=vibrato/
  tremolo, `t`=tilt), so e.g. `doom` drops him to ~60 Hz and slows him down,
  `shrill` lifts him to ~220 Hz. The enum in `oracle.Voices` MUST match the
  `VOICES` keys. A flat-file fallback uses the default voice.

Safety posture — **the visitor message is untrusted**:

- Fencing the message in the prompt is the *weakest* layer and is not relied on.
  The load-bearing controls hold even if the model is jailbroken: a hard
  **output-token cap** (a jailbroken Muskie still emits ≤1 line), **sanitize + an
  output filter** that falls back to a flat-file quote on a veto, and **messages
  gated to logged-in** (accountable, rate-limited) visitors. Anonymous asks still
  get state-aware wisdom, just no custom message.
- Use a **stock** model, not an abliterated one — its native refusals are part of
  the backstop. The built-in filter is minimal; plug a guard model (Llama Guard /
  ShieldGemma) into `oracle.Config.Filter` for real coverage.

Run it:

```
docker compose up -d ollama
docker exec askmuskie-ollama-1 ollama pull qwen3:4b
MUSKIE_LLM_URL=http://localhost:11434 MUSKIE_LLM_MODEL=qwen3:4b \
  DATABASE_URL=postgres://askmuskie:askmuskie@localhost:5439/askmuskie \
  PORT=8099 ./bin/askmuskie
```

On a CPU box a stock 4B (~2.5 GB) returns a one-liner in ~1–2 s; on a
multi-socket host, pin Ollama to one NUMA node (see `docker-compose.yml`).

## Adding wisdom

Edit `data/quotes.txt`. One quote per stanza, separated by blank lines; a line
starting with `--` is the attribution; `#` lines are comments:

```
Still water holds the deepest fish.
-- The Muskie

The river does not hurry, yet everything arrives.
```

## The model

`static/models/muskie.glb` is a rigged muskellunge (source: `Y:\Space Game`).
The client relies on its rig:

- bone **`jaw`** — pitch about **−X** opens the mouth (`rotation.x = -0.55` fully
  open); driven from the audio level for lip-sync.
- clips **`Swim`** (spine undulation) and **`EyesLookAround`** are layered; the
  canned **`Talk`** clip is intentionally *not* played, since the jaw is driven
  manually.
- head is **+X**, ~2 units snout-to-tail.

If the GLB ever fails to load, a procedural stand-in fish keeps the page working.

## Vendored libraries

Self-hosted under `static/js/vendor/` (CSP is `'self'`):

- **three.js** 0.169.0 (`three/`) — `GLTFLoader` / `BufferGeometryUtils` /
  `ConvexGeometry` / `ConvexHull` import specifiers are patched to absolute paths
  so no import map is needed.
- **klattsch** 0.5.0 (`klattsch/`) — parallel-formant synth + AudioWorklet.
- **CMU Pronouncing Dictionary** (`internal/speech/cmudict.dict`).

All MIT/BSD; license files are kept alongside.

## Tuning

Pond geometry and economy are constants at the top of `internal/pond/pond.go`
(radii, speeds, food accrual/cap, speak-duration estimate). The camera, movement,
and look are at the top of `static/js/muskie.js`; the avatar catalog and palettes
are in `static/js/shapes.js`.
