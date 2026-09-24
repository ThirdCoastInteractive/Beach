// Crosshair for the band charts — Bollinger, Difference — which share a viewBox
// coordinate system and a plot rectangle described by data-plot-* attributes.
//
// The crosshair used to show a position and nothing else: fine for a pointer,
// whose owner can read the line underneath it, and worth nothing to a keyboard,
// which has no way to read anything. So the server now sends the samples
// (data-vb-hover names a JSON payload) with the announcement text already
// formatted, and this module steps through them on the arrow keys and says each
// one — WCAG 2.1.1.
//
// The text arrives finished rather than as parts, because assembling it would
// mean deciding number formatting and word order for a language this module does
// not know. The server has the labels, the units and the catalog.

import { announce, liveRegion, observeAll, rovingKeys } from './chart-keys.js';

const NS = 'http://www.w3.org/2000/svg';

function init(svg) {
  const overlay = svg.querySelector('.vb-hover-overlay');
  if (!overlay) return;

  const plotL = parseFloat(svg.getAttribute('data-plot-l') || '0');
  const plotR = parseFloat(svg.getAttribute('data-plot-r') || '1000');
  const plotT = parseFloat(svg.getAttribute('data-plot-t') || '0');
  const plotB = parseFloat(svg.getAttribute('data-plot-b') || '600');

  const cross = document.createElementNS(NS, 'line');
  cross.setAttribute('stroke', 'var(--color-fg-muted)');
  cross.setAttribute('stroke-width', '1');
  cross.setAttribute('stroke-dasharray', '4 4');
  cross.setAttribute('pointer-events', 'none');
  cross.style.display = 'none';
  svg.appendChild(cross);

  const showAt = (x) => {
    cross.setAttribute('x1', x);
    cross.setAttribute('x2', x);
    cross.setAttribute('y1', plotT);
    cross.setAttribute('y2', plotB);
    cross.style.display = '';
  };
  const hide = () => { cross.style.display = 'none'; };

  const svgX = (clientX) => {
    const pt = svg.createSVGPoint();
    pt.x = clientX;
    pt.y = 0;
    const ctm = svg.getScreenCTM();
    if (!ctm) return plotL;
    return Math.max(plotL, Math.min(plotR, pt.matrixTransform(ctm.inverse()).x));
  };

  overlay.addEventListener('pointermove', (ev) => showAt(svgX(ev.clientX)));
  overlay.addEventListener('pointerleave', hide);

  // --- keyboard -------------------------------------------------------------

  // The figure is the tab stop, not the SVG: the SVG is aria-hidden, and a
  // focusable descendant of aria-hidden is its own defect.
  const fig = svg.closest('[data-chart-keys="vb"]');
  if (!fig) return;

  const payload = document.getElementById(svg.dataset.vbHover || '');
  let samples = [];
  if (payload) {
    try {
      samples = (JSON.parse(payload.textContent) || {}).samples || [];
    } catch {
      samples = [];
    }
  }
  if (!samples.length) return;

  const region = liveRegion(fig);
  rovingKeys(fig, {
    count: () => samples.length,
    onIndex(i) {
      const s = samples[i];
      showAt(plotL + s.fx * (plotR - plotL));
      announce(region, s.text);
    },
    onEscape: hide,
  });
}

observeAll('.vb-hover', init, '__vbHoverWired');
