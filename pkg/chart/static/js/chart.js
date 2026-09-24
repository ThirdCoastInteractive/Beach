// Chart interaction entry point. Loaded by driftwood's Page head as
// <script type="module" src="/static/js/chart.js" defer>. Every import
// is a self-contained side-effect module: it wires its own listeners
// and MutationObservers against server-rendered SVG, and exports
// nothing. Plain browser ES modules — no bundler involved.

import './chart-tip.js';
import './chart-line-hover.js';
import './chart-toggle.js';
import './chart-kbd.js';
import './chart-sankey-hover.js';
import './chart-chord-hover.js';
import './chart-bundle-hover.js';
import './chart-vb-hover.js';
import './chart-toolbar.js';
import './geomap-interact.js';
import './globe.js';
