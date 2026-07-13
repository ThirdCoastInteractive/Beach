package main

// shellCSS is the demo-site chrome: the page frame, top nav, panels, chart
// boxes, and the interactive drill panel. It rides on top of the inlined
// app.css, so it only styles the demo wrapper and leans on the same design
// tokens (--color-paper/panel/line/fg/accent) the charts use. No hex colors,
// no webfonts — matches the house look on the dark theme.
const shellCSS = `
/* ---- beach-demos site chrome ---- */
.demo-body {
  margin: 0;
  min-height: 100vh;
  background: var(--color-paper);
  color: var(--color-fg-default);
  font-family: var(--font-sans);
  font-size: 15px;
  line-height: 1.55;
  -webkit-font-smoothing: antialiased;
}
.demo-topbar {
  position: sticky;
  top: 0;
  z-index: 10;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem 1.5rem;
  padding: 0.85rem 1.5rem;
  background: color-mix(in oklab, var(--color-paper) 88%, transparent);
  backdrop-filter: blur(8px);
  border-bottom: 1px solid var(--color-line-soft);
}
.demo-brand { display: flex; flex-direction: column; line-height: 1.2; }
.demo-brand-mark { font-weight: 650; letter-spacing: 0.01em; }
.demo-brand-sub { font-size: 0.75rem; color: var(--color-fg-muted); font-family: var(--font-mono); }
.demo-nav { display: flex; flex-wrap: wrap; gap: 0.35rem; }
.demo-nav-link {
  display: inline-block;
  padding: 0.3rem 0.7rem;
  border-radius: 0.5rem;
  color: var(--color-fg-muted);
  text-decoration: none;
  font-size: 0.85rem;
  border: 1px solid transparent;
  transition: color 0.12s, background 0.12s, border-color 0.12s;
}
.demo-nav-link:hover { color: var(--color-fg-default); background: var(--color-panel-hover); }
.demo-nav-link.is-active {
  color: var(--color-fg-default);
  background: var(--color-panel);
  border-color: var(--color-line-soft);
}
.demo-main { max-width: 1200px; margin: 0 auto; padding: 1.75rem 1.5rem 3rem; }
.demo-head { margin-bottom: 1.5rem; }
.demo-h1 { margin: 0 0 0.35rem; font-size: 1.65rem; font-weight: 680; letter-spacing: -0.01em; }
.demo-sub { margin: 0; max-width: 62ch; color: var(--color-fg-muted); }
.demo-note {
  margin: 0.9rem 0 0;
  padding: 0.7rem 0.9rem;
  border-left: 3px solid var(--color-accent);
  background: var(--color-panel-hover);
  border-radius: 0 0.5rem 0.5rem 0;
  font-size: 0.85rem;
  color: var(--color-fg-muted);
}
.demo-note code { font-family: var(--font-mono); color: var(--color-fg-default); }

.demo-panel {
  background: var(--color-panel);
  border: 1px solid var(--color-line-soft);
  border-radius: 0.65rem;
  padding: 1rem 1.1rem 1.15rem;
  margin-bottom: 1.25rem;
}
.demo-panel-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.5rem 0.9rem;
  margin-bottom: 0.85rem;
}
.demo-eyebrow {
  font-family: var(--font-mono);
  font-size: 0.72rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--color-fg-muted);
}
.demo-panel-note { font-size: 0.8rem; color: var(--color-fg-muted); }

.demo-chart {
  position: relative;
  width: 100%;
  border-radius: 0.5rem;
  overflow: hidden;
}

/* Auto-flow gallery grid + explicit column spans for dashboard rows. */
.demo-grid {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 1.25rem;
}
.demo-grid > .demo-panel { margin-bottom: 0; }
.demo-span-12 { grid-column: span 12; }
.demo-span-8  { grid-column: span 8; }
.demo-span-6  { grid-column: span 6; }
.demo-span-4  { grid-column: span 4; }
.demo-span-3  { grid-column: span 3; }
@media (max-width: 900px) {
  .demo-span-8, .demo-span-6, .demo-span-4, .demo-span-3 { grid-column: span 12; }
}

/* Small labeled tiles for the ramp-comparison row. */
.demo-tile-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 0.9rem;
}
.demo-tile-cap {
  margin: 0.4rem 0 0;
  text-align: center;
  font-family: var(--font-mono);
  font-size: 0.72rem;
  letter-spacing: 0.04em;
  color: var(--color-fg-muted);
}

/* Gallery card (index) is a whole clickable panel. */
a.demo-card { display: block; text-decoration: none; color: inherit; }
a.demo-card:hover { border-color: var(--color-line-strong); background: var(--color-panel-hover); }
a.demo-card .demo-card-title { font-weight: 620; margin: 0.6rem 0 0.15rem; }
a.demo-card .demo-card-desc { margin: 0; font-size: 0.82rem; color: var(--color-fg-muted); }

/* Interactive drill layout: map beside an info rail. */
.demo-drill { display: grid; grid-template-columns: 1fr 300px; gap: 1.25rem; align-items: start; }
@media (max-width: 900px) { .demo-drill { grid-template-columns: 1fr; } }
.demo-rail {
  background: var(--color-panel);
  border: 1px solid var(--color-line-soft);
  border-radius: 0.65rem;
  padding: 1rem 1.1rem;
  min-height: 320px;
}
.demo-rail h3 { margin: 0 0 0.15rem; font-size: 1.05rem; }
.demo-rail .demo-rail-code { font-family: var(--font-mono); font-size: 0.78rem; color: var(--color-fg-muted); }
.demo-rail-empty { color: var(--color-fg-muted); font-size: 0.9rem; }
.demo-stat-row { display: flex; justify-content: space-between; padding: 0.4rem 0; border-bottom: 1px solid var(--color-line-soft); }
.demo-stat-row:last-child { border-bottom: 0; }
.demo-stat-label { color: var(--color-fg-muted); font-size: 0.85rem; }
.demo-stat-value { font-family: var(--font-mono); font-weight: 600; }
.demo-spark { width: 100%; height: 46px; margin-top: 0.6rem; display: block; }
`
