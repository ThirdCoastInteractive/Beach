# lotophages.com

A one-page landing site as **the plate of an esoteric order** — *the Order of the
Lotus-Eaters*. Greek-chorus secrecy meets a true **cyanotype**: vivid Prussian /
cobalt blue, luminous **white stippled** imagery, a single flat **gold sun**.
ἥλιος · λωτός · λόγος — sun · lotus · *logos*.

The structure is the "secret society" half: **straight edges and corners** — a
ruled frame with corner brackets, registers of marks top and bottom, a Latin
motto (*Secretum Ordinis Loti*), hard-edged geometry (square, diamond, triangle,
baseline). The lotus is the one organic thing inside all that order, rendered in
pointillist stipple the way a cyanotype illustration is. The email field is a
**petition to the order**, not a newsletter.

## Files
- `index.html` — the page; inline SVG emblem (stipple-pattern lotus, gold sun, geometry) + cyanotype paper filters
- `styles.css` — cobalt palette, the ruled frame, registers, type, the print "develops" on load
- `app.js` — the rotating rites, the petition
- `favicon.svg`, `og.svg` — the mark as icon and social card

No build step, no dependencies. Type is **Cinzel** (engraved inscriptional caps)
+ **Archivo** (the structured order marks), from Google Fonts. Motion (develop-on-
load, faint star twinkle, rite cross-fades) honours `prefers-reduced-motion`.

Note: the stipple is a real SVG turbulence pattern (`#stippleDots` → `#stip`),
filling the petal paths directly — no fragile clip-paths.

## Run / deploy
Open `index.html` directly, or drop the folder on any static host
(Netlify, Cloudflare Pages, `caddy file-server`, an S3 bucket — anything).

## Wire the petition
The form confirms client-side only — no address leaves the browser. Point it at a
list provider in `app.js` (look for the `// No backend yet` note), e.g. POST to a
Formspree/Buttondown endpoint, or set a real `action` on the `<form id="summon">`
in `index.html`.

## Social card
`og.svg` is the source. Most scrapers ignore SVG `og:image`s — export a
**1200×630 PNG** from it (open in a browser, screenshot, or
`rsvg-convert og.svg -o og.png`) and point `og:image` at the PNG before launch.
