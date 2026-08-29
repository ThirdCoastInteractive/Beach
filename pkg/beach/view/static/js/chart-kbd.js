// Wires the keyboard layer to every chart the server marked operable.
//
// The server decides *which* charts get a tab stop — it knows whether a chart
// has per-shape tooltips worth stepping through — and tags them with
// data-chart-keys carrying the chart family. This module only decides *what to
// step through* for each family, which is the one thing the server has no
// business knowing: a CSS selector.
//
// The line chart is absent from the table on purpose. It has its own module with
// a crosshair, a synced cursor across every chart on the page, and a window
// selection; a generic shape-walker would be a downgrade, so chart-line-hover.js
// keeps it and this leaves it alone.

import { hoverKeys, observeAll } from './chart-keys.js';

// Which shapes are the samples, per chart family. Each is the same set the
// pointer can hover, so the two input methods reach exactly the same things —
// which is the whole of WCAG 2.1.1 here.
const WALKS = {
  // Tooltip charts: bars, cells, dots, arcs. The tip carries the announcement.
  tip: '[data-tip]',
  // Flow diagrams: the nodes, not the links. Stepping node by node follows how
  // someone reads the diagram; stepping link by link does not.
  //
  // .chart-hover-node and not [data-node-id]: a sankey node's *label* carries
  // the same id and no tip, so the broader selector walked every node twice and
  // said nothing on the second visit.
  sankey: '.chart-hover-node',
  chord: '.chord-arc',
  bundle: '.bundle-node',
  // Band charts have no per-shape hover at all — their crosshair reads a
  // separate payload — so chart-vb-hover.js drives those itself.
};

observeAll('[data-chart-keys]', (fig) => {
  const selector = WALKS[fig.dataset.chartKeys];
  if (!selector) return;
  hoverKeys(fig, selector);
}, '__chartKbd');
