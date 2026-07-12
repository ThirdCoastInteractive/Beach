# lotophages.com

A single animated image, no text: a **3D lotus built on the golden angle**,
rendered to **1-bit dithered** cyanotype. WebGL via Three.js.

The "famous ratios" are the structure, not decoration:
- **Petals** are placed by the **golden angle** (137.507°) — phyllotaxis, not even spacing.
- The **seed pod** at the centre is a true **golden-angle spiral** (the sunflower/lotus-seedhead pattern), an `InstancedMesh` of glowing seeds.
- The dither threshold drifts in time by the **golden ratio** for a stable, non-repeating shimmer.

The 1-bit pass is **grid-free** — interleaved-gradient noise, not an ordered
Bayer matrix — so it reads as organic dithering rather than a dot grid. Two
colours only (Prussian blue + pale cyan-white); all tone is faked by the dither.
The post shader adds quiet digital artifacts: **soft scanlines**, a **vignette**,
and a **rare horizontal slip** (a signal tear) — all subtle.

## Files
- `index.html` — a full-bleed `<canvas>` + Three.js from CDN
- `styles.css` — black/blue full-screen canvas, nothing else
- `app.js` — the lotus, the pod, the motes, and the dither shader
- `favicon.png`, `og.png` — frames grabbed from the render

## How it renders
Scene (lit lotus + instanced seed pod) → a `WebGLRenderTarget` (with a
`DepthTexture`) → a fullscreen quad whose fragment shader: computes luminance,
burns in **depth-edge outlines** (dark contours where petals overlap or meet the
void — this is what gives the bloom its definition), then thresholds against
interleaved-gradient noise into the two cyanotype colours. The bloom slowly
rotates and "breathes"; reduced-motion stills it.

## Dependencies
Only **three.js r149** (one `<script>` from unpkg). No build step. To vendor it,
drop `three.min.js` next to the page and update the `<script src>`.

## Run / deploy
Open `index.html`, or drop the folder on any static host.

## Social card
`og.png` is a captured frame (1200×630). Re-grab it any time by screenshotting
the page at that size.
