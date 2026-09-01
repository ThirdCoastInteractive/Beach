package main

// The explorer's markup and behaviour, kept apart from the handlers in
// explore.go so neither file is mostly the other one's payload.
//
// The <script> is a bodyless same-origin module reference, which is the shape
// beach-vet sanctions: behaviour is served like any other asset rather than
// pasted inline.

// presetSlot is where the preset <option> list goes. It is a marker rather than
// a format verb because the page is mostly CSS, and CSS is full of percent signs.
const presetSlot = "<!--PRESETS-->"

const explorePage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>beach-palette — theme explorer</title>
<style>
  :root { color-scheme: dark; --ui-bg:#131315; --ui-panel:#1c1c1f; --ui-line:#34343a;
          --ui-fg:#e8e8ea; --ui-dim:#9a9aa2; --ui-hi:#7ec9c6; }
  * { box-sizing: border-box; }
  body { margin:0; background:var(--ui-bg); color:var(--ui-fg);
         font:14px/1.5 ui-sans-serif, system-ui, sans-serif; }
  header { display:flex; align-items:center; gap:1rem; flex-wrap:wrap;
           padding:.7rem 1rem; border-bottom:1px solid var(--ui-line); background:var(--ui-panel); }
  h1 { margin:0; font-size:14px; font-weight:700; }
  h1 small { font-weight:400; color:var(--ui-dim); margin-left:.6rem; }
  .tabs { margin-left:auto; display:flex; gap:.3rem; }
  .tab { border:1px solid var(--ui-line); background:transparent; color:var(--ui-dim);
         padding:.35rem .9rem; font:inherit; font-size:12px; cursor:pointer; }
  .tab[aria-pressed="true"] { background:var(--ui-fg); color:var(--ui-bg); border-color:var(--ui-fg); }
  main { padding:1rem; }
  .hidden { display:none !important; }

  /* --- gallery: choose, don't configure --- */
  .gallery { display:grid; grid-template-columns:repeat(auto-fill, minmax(310px,1fr)); gap:12px; }
  .card { border:1px solid var(--ui-line); background:var(--ui-panel); }
  .card > button { display:block; width:100%; text-align:left; cursor:pointer;
                   border:0; background:transparent; color:inherit; font:inherit; padding:0; }
  .card.is-current { outline:2px solid var(--ui-hi); }
  .cap { padding:6px 9px; border-bottom:1px solid var(--ui-line); }
  .cap b { font-size:13px; }
  .cap span { display:block; color:var(--ui-dim); font-size:11px; margin-top:1px; }
  .duo { display:grid; grid-template-columns:1fr 1fr; }

  /* --- the wheel --- */
  .wheel-wrap { display:grid; grid-template-columns:340px minmax(0,1fr); gap:1.25rem; align-items:start; }
  @media (max-width: 900px) { .wheel-wrap { grid-template-columns:1fr; } }
  .wheel { width:100%; max-width:320px; touch-action:none; cursor:grab; }
  .wheel:active { cursor:grabbing; }
  .pin { cursor:grab; }
  .legend { display:flex; flex-direction:column; gap:.35rem; margin-top:.6rem; font-size:12px; }
  .legend i { display:inline-block; width:11px; height:11px; margin-right:.45rem;
              vertical-align:-1px; }
  .harmony { display:flex; gap:.3rem; margin-top:.7rem; flex-wrap:wrap; }
  .harmony button { border:1px solid var(--ui-line); background:transparent; color:var(--ui-dim);
                    padding:.25rem .6rem; font:inherit; font-size:11px; cursor:pointer; }
  .harmony button:hover { color:var(--ui-fg); border-color:var(--ui-fg); }

  details { border:1px solid var(--ui-line); margin-top:1rem; }
  summary { padding:.5rem .7rem; cursor:pointer; font-size:12px; color:var(--ui-dim); }
  .adv { padding:.2rem .7rem .7rem; display:grid; grid-template-columns:repeat(auto-fill,minmax(210px,1fr)); gap:.1rem .9rem; }
  .adv label { display:grid; grid-template-columns:1fr auto; font-size:11px; margin-bottom:.35rem; }
  .adv label span:last-child { font-family:ui-monospace,monospace; color:var(--ui-dim); }
  .adv input { grid-column:1/-1; width:100%; accent-color:var(--ui-hi); }
  select { background:var(--ui-bg); color:var(--ui-fg); border:1px solid var(--ui-line);
           padding:.35rem; font:inherit; font-size:12px; }
  .err { border:1px solid #a33; background:#2a1414; color:#f4b3b3; padding:.7rem .9rem; margin-bottom:.8rem; }
  pre { background:#000; border:1px solid var(--ui-line); padding:.8rem; overflow:auto;
        font:12px/1.5 ui-monospace,monospace; color:#cfcfcf; margin-top:1rem; }
  .chip { padding:2px 7px; font-size:10px; font-weight:700; display:inline-block; }
  .bar { display:flex; gap:5px; align-items:center; flex-wrap:wrap; margin-top:7px; }
  .series { display:flex; height:14px; margin-top:7px; }
  .series div { flex:1; }
</style>
</head>
<body>
<header>
  <h1>beach-palette <small>every colour here is solved against the contrast it owes</small></h1>
  <div class="tabs">
    <button class="tab" id="tab-gallery" aria-pressed="true">Gallery</button>
    <button class="tab" id="tab-wheel" aria-pressed="false">Hue wheel</button>
  </div>
</header>

<main>
  <div id="err" class="err hidden"></div>

  <section id="view-gallery">
    <div class="gallery" id="gallery"></div>
  </section>

  <section id="view-wheel" class="hidden">
    <div class="wheel-wrap">
      <div>
        <label style="font-size:12px;color:var(--ui-dim)">Base preset<br>
          <select id="preset" style="width:100%;margin-top:.3rem"><!--PRESETS--></select>
        </label>
        <svg class="wheel" id="wheel" viewBox="0 0 240 240" aria-label="OKLCH hue wheel"></svg>
        <div class="legend" id="legend"></div>
        <div class="harmony">
          <button data-harmony="complement">Complement</button>
          <button data-harmony="triad">Triad</button>
          <button data-harmony="split">Split</button>
          <button data-harmony="analogous">Analogous</button>
        </div>
      </div>
      <div>
        <div class="duo" id="wheel-preview"></div>
        <details>
          <summary>Advanced — the raw parameters</summary>
          <form class="adv" id="adv">
            <label>neutral hue <span data-out="nhue"></span><input type="range" name="nhue" min="0" max="359" step="1"></label>
            <label>neutral chroma <span data-out="nchroma"></span><input type="range" name="nchroma" min="0" max="0.03" step="0.001"></label>
            <label>chroma fraction <span data-out="chroma"></span><input type="range" name="chroma" min="0.3" max="1" step="0.01"></label>
            <label>ghost wash <span data-out="wash"></span><input type="range" name="wash" min="0.04" max="0.4" step="0.01"></label>
            <label>dark paper L <span data-out="paperL"></span><input type="range" name="paperL" min="0.05" max="0.4" step="0.005"></label>
            <label>dark panel L <span data-out="panelL"></span><input type="range" name="panelL" min="0.05" max="0.45" step="0.005"></label>
            <label>dark hover L <span data-out="hoverL"></span><input type="range" name="hoverL" min="0.05" max="0.5" step="0.005"></label>
            <label>light paper L <span data-out="lightPaperL"></span><input type="range" name="lightPaperL" min="0.85" max="1" step="0.005"></label>
            <label>light panel L <span data-out="lightPanelL"></span><input type="range" name="lightPanelL" min="0.85" max="1" step="0.005"></label>
            <label>good hue <span data-out="good"></span><input type="range" name="good" min="0" max="359" step="1"></label>
            <label>warn hue <span data-out="warn"></span><input type="range" name="warn" min="0" max="359" step="1"></label>
            <label>bad hue <span data-out="bad"></span><input type="range" name="bad" min="0" max="359" step="1"></label>
            <label>info hue <span data-out="info"></span><input type="range" name="info" min="0" max="359" step="1"></label>
          </form>
        </details>
        <pre id="code"></pre>
      </div>
    </div>
  </section>
</main>

<script src="/static/js/explore.js" type="module"></script>
</body>
</html>
`

const exploreJS = `
// The explorer's client half. It owns no colour logic whatsoever: every value on
// screen came from theme.Build over the wire, so the page cannot drift from what
// the generator will actually write.

const $ = (id) => document.getElementById(id);
const PINS = ['accent', 'accent2', 'accent3'];
const PIN_LABEL = { accent: 'Accent', accent2: 'Category B', accent3: 'Category C' };

let mode = 'gallery';
let hues = { accent: 195, accent2: 35, accent3: 300, neutral: 70 };
let advanced = {};   // only fields the user has actually touched
let current = null;  // the derived theme showing in the wheel view

// --- a slice of real UI ---------------------------------------------------------
// Deliberately not a swatch sheet. A colour is only judgeable in the shapes it
// will be painted on, so this is a card, a heading, body copy, a link, the four
// button states, the status chips, an input edge and a focus ring — every pair
// the kit owes a ratio to, in the arrangement it owes it in.
function slice(t, scheme, compact) {
  const c = (n) => t['--color-' + n];
  const pad = compact ? '10px' : '18px';
  return ` + "`" + `
  <div style="background:${c('paper')};color:${c('fg-default')};padding:${pad}">
    <div style="font:700 ${compact ? 13 : 17}px/1.3 inherit;color:${c('fg-strong')}">Tide tables</div>
    <div style="color:${c('fg-muted')};font-size:${compact ? 11 : 13}px;margin:2px 0 9px">
      Secondary copy on the muted stop, with
      <a style="color:${c('accent-text')}">a link</a> in it.
    </div>
    <div style="background:${c('panel')};border:1px solid ${c('line-soft')};padding:${compact ? '8px' : '12px'}">
      <div class="bar">
        <span class="chip" style="background:${c('accent')};color:${c('on-accent')};padding:5px 11px">Primary</span>
        <span class="chip" style="background:${c('accent-hover')};color:${c('on-accent')};padding:5px 11px">Hover</span>
        <span class="chip" style="border:1px solid ${c('accent')};color:${c('accent-text')};padding:4px 10px">Ghost</span>
        <span class="chip" style="background:${c('accent-soft')};color:${c('accent-text')};padding:5px 11px">Wash</span>
      </div>
      <div class="bar">
        <span class="chip" style="background:${c('good')};color:${c('paper')}">GOOD</span>
        <span class="chip" style="background:${c('warn')};color:${c('paper')}">WARN</span>
        <span class="chip" style="background:${c('bad')};color:${c('paper')}">BAD</span>
        <span class="chip" style="background:${c('info')};color:${c('paper')}">INFO</span>
        <span class="chip" style="background:${c('accent-2')};color:${c('paper')}">B</span>
        <span class="chip" style="background:${c('accent-3')};color:${c('paper')}">C</span>
      </div>
      <div class="bar">
        <span style="border:1px solid ${c('line-strong')};background:${c('paper')};
                     color:${c('fg-default')};padding:4px 9px;font-size:11px">input edge</span>
        <span style="border:1px solid ${c('accent')};outline:2px solid ${c('accent')};outline-offset:2px;
                     background:${c('paper')};color:${c('fg-default')};padding:4px 9px;font-size:11px">focused</span>
        <span style="color:${c('fg-disabled')};font-size:11px">disabled</span>
      </div>
    </div>
    <div class="series">${series(t)}</div>
  </div>` + "`" + `;
}

function series(t) {
  let out = '';
  for (let i = 0; i < 15; i++) {
    out += '<div style="background:' + t['--color-series-' + String.fromCharCode(97 + i)] + '"></div>';
  }
  return out;
}

// --- gallery --------------------------------------------------------------------
async function loadGallery() {
  const all = await (await fetch('/api/gallery')).json();
  $('gallery').innerHTML = all.map((d) => {
    if (d.error) {
      return '<div class="card"><div class="cap"><b>' + d.title + '</b><span>' + d.error + '</span></div></div>';
    }
    return '<div class="card" data-preset="' + d.key + '"><button>' +
      '<div class="cap"><b>' + d.title + '</b><span>' + d.note + '</span></div>' +
      '<div class="duo">' + slice(d.dark, 'dark', true) + slice(d.light, 'light', true) + '</div>' +
      '</button></div>';
  }).join('');
}

// --- the hue wheel ---------------------------------------------------------------
// The circle is honest here in a way an HSL or RYB wheel is not: OKLCH hue is
// perceptually uniform, so equal angles really are equal perceptual steps, and a
// triad drawn as three points 120 degrees apart really does look like a triad.
function drawWheel() {
  const cx = 120, cy = 120, rOuter = 108, rInner = 78;
  let out = '';
  // The ring, one degree at a time, at the lightness and chroma the theme uses.
  for (let deg = 0; deg < 360; deg++) {
    const a0 = (deg - 90.5) * Math.PI / 180, a1 = (deg - 89.5) * Math.PI / 180;
    const p = (r, a) => (cx + r * Math.cos(a)).toFixed(2) + ' ' + (cy + r * Math.sin(a)).toFixed(2);
    out += '<path d="M ' + p(rInner, a0) + ' L ' + p(rOuter, a0) +
           ' A ' + rOuter + ' ' + rOuter + ' 0 0 1 ' + p(rOuter, a1) +
           ' L ' + p(rInner, a1) + ' Z" fill="oklch(0.72 0.16 ' + deg + ')"/>';
  }
  // Harmony guides between the pins, so the relationships are visible rather
  // than arithmetic.
  const pt = (deg, r) => {
    const a = (deg - 90) * Math.PI / 180;
    return [cx + r * Math.cos(a), cy + r * Math.sin(a)];
  };
  const [ax, ay] = pt(hues.accent, rInner - 8);
  for (const k of ['accent2', 'accent3']) {
    const [bx, by] = pt(hues[k], rInner - 8);
    out += '<line x1="' + ax + '" y1="' + ay + '" x2="' + bx + '" y2="' + by +
           '" stroke="#ffffff40" stroke-width="1" stroke-dasharray="3 3"/>';
  }
  for (const k of PINS) {
    const [x, y] = pt(hues[k], (rInner + rOuter) / 2);
    const fill = current ? current.dark['--color-' + (k === 'accent' ? 'accent' : k.replace('accent', 'accent-'))] : '#fff';
    out += '<g class="pin" data-pin="' + k + '">' +
      '<circle cx="' + x + '" cy="' + y + '" r="13" fill="' + fill + '" stroke="#fff" stroke-width="2.5"/>' +
      '<title>' + PIN_LABEL[k] + '</title></g>';
  }
  out += '<circle cx="' + cx + '" cy="' + cy + '" r="' + (rInner - 2) + '" fill="var(--ui-bg)"/>';
  out += '<text x="' + cx + '" y="' + (cy - 4) + '" text-anchor="middle" fill="var(--ui-dim)" font-size="11">OKLCH hue</text>';
  out += '<text x="' + cx + '" y="' + (cy + 12) + '" text-anchor="middle" fill="var(--ui-dim)" font-size="11">drag the pins</text>';
  $('wheel').innerHTML = out;

  $('legend').innerHTML = PINS.map((k) => {
    const sw = current ? current.dark['--color-' + (k === 'accent' ? 'accent' : k.replace('accent', 'accent-'))] : 'transparent';
    return '<div><i style="background:' + sw + '"></i>' + PIN_LABEL[k] + ' — ' + Math.round(hues[k]) + '°</div>';
  }).join('');
}

function wheelDrag(e) {
  const g = e.target.closest('[data-pin]');
  if (!g) return;
  const key = g.dataset.pin;
  const svg = $('wheel');
  const move = (ev) => {
    const r = svg.getBoundingClientRect();
    const x = (ev.clientX ?? ev.touches[0].clientX) - r.left - r.width / 2;
    const y = (ev.clientY ?? ev.touches[0].clientY) - r.top - r.height / 2;
    hues[key] = (Math.atan2(y, x) * 180 / Math.PI + 450) % 360;
    drawWheel();
    refreshWheel();
  };
  const up = () => {
    window.removeEventListener('pointermove', move);
    window.removeEventListener('pointerup', up);
  };
  window.addEventListener('pointermove', move);
  window.addEventListener('pointerup', up);
  e.preventDefault();
}

function applyHarmony(kind) {
  const a = hues.accent;
  if (kind === 'complement') { hues.accent2 = (a + 180) % 360; hues.accent3 = (a + 150) % 360; }
  if (kind === 'triad')      { hues.accent2 = (a + 120) % 360; hues.accent3 = (a + 240) % 360; }
  if (kind === 'split')      { hues.accent2 = (a + 150) % 360; hues.accent3 = (a + 210) % 360; }
  if (kind === 'analogous')  { hues.accent2 = (a + 35) % 360;  hues.accent3 = (a + 325) % 360; }
  drawWheel();
  refreshWheel();
}

// --- deriving --------------------------------------------------------------------
function query() {
  const q = new URLSearchParams({ preset: $('preset').value });
  for (const k of PINS) q.set(k, Math.round(hues[k]));
  q.set('nhue', Math.round(hues.neutral));
  for (const [k, v] of Object.entries(advanced)) q.set(k, v);
  return q.toString();
}

async function refreshWheel() {
  const d = await (await fetch('/api/theme?' + query())).json();
  const err = $('err');
  if (d.error) {
    err.textContent = d.error;
    err.classList.remove('hidden');
    return;
  }
  err.classList.add('hidden');
  current = d;
  $('wheel-preview').innerHTML = slice(d.dark, 'dark', false) + slice(d.light, 'light', false);
  $('code').textContent = d.go;
  drawWheel();
}

// Seed the advanced sliders from the chosen preset the first time it is opened,
// so their positions describe the theme on screen instead of sitting at zero.
async function seedAdvanced() {
  const d = await (await fetch('/api/theme?preset=' + $('preset').value)).json();
  if (d.hues) hues = { ...hues, ...d.hues };
}

function readouts() {
  for (const el of $('adv').querySelectorAll('input')) {
    const out = $('adv').querySelector('[data-out="' + el.name + '"]');
    if (out && el.value !== '') out.textContent = el.value;
  }
}

// --- wiring ----------------------------------------------------------------------
function setMode(next) {
  mode = next;
  $('tab-gallery').setAttribute('aria-pressed', String(next === 'gallery'));
  $('tab-wheel').setAttribute('aria-pressed', String(next === 'wheel'));
  $('view-gallery').classList.toggle('hidden', next !== 'gallery');
  $('view-wheel').classList.toggle('hidden', next !== 'wheel');
  if (next === 'wheel') refreshWheel();
}

$('tab-gallery').addEventListener('click', () => setMode('gallery'));
$('tab-wheel').addEventListener('click', () => setMode('wheel'));

// Picking from the gallery drops you into the wheel with that preset loaded,
// which is the move you always want next: choose the neighbourhood, then adjust.
$('gallery').addEventListener('click', async (e) => {
  const card = e.target.closest('[data-preset]');
  if (!card) return;
  $('preset').value = card.dataset.preset;
  advanced = {};
  await seedAdvanced();
  setMode('wheel');
});

$('preset').addEventListener('change', async () => {
  advanced = {};
  await seedAdvanced();
  refreshWheel();
});

$('wheel').addEventListener('pointerdown', wheelDrag);

for (const b of document.querySelectorAll('[data-harmony]')) {
  b.addEventListener('click', () => applyHarmony(b.dataset.harmony));
}

$('adv').addEventListener('input', (e) => {
  advanced[e.target.name] = e.target.value;
  readouts();
  refreshWheel();
});

loadGallery();
seedAdvanced().then(drawWheel);
`
