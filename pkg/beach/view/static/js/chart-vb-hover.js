// Generic viewBox-based crosshair hover for charts that use a viewBox
// coordinate system (Bollinger, Difference, Scatter, etc.). Shows a
// vertical crosshair line tracking the pointer within the plot area.

(function () {
  var NS = 'http://www.w3.org/2000/svg';

  function init(svg) {
    if (svg.__vbHoverWired) return;
    svg.__vbHoverWired = true;

    var overlay = svg.querySelector('.vb-hover-overlay');
    if (!overlay) return;

    var plotL = parseFloat(svg.getAttribute('data-plot-l') || '0');
    var plotR = parseFloat(svg.getAttribute('data-plot-r') || '1000');
    var plotT = parseFloat(svg.getAttribute('data-plot-t') || '0');
    var plotB = parseFloat(svg.getAttribute('data-plot-b') || '600');

    var cross = document.createElementNS(NS, 'line');
    cross.setAttribute('stroke', 'var(--color-fg-muted)');
    cross.setAttribute('stroke-width', '1');
    cross.setAttribute('stroke-dasharray', '4 4');
    cross.setAttribute('pointer-events', 'none');
    cross.style.display = 'none';
    svg.appendChild(cross);

    function svgX(clientX) {
      var pt = svg.createSVGPoint();
      pt.x = clientX;
      pt.y = 0;
      var ctm = svg.getScreenCTM();
      if (!ctm) return plotL;
      var svgPt = pt.matrixTransform(ctm.inverse());
      return Math.max(plotL, Math.min(plotR, svgPt.x));
    }

    overlay.addEventListener('pointermove', function (ev) {
      var x = svgX(ev.clientX);
      cross.setAttribute('x1', x);
      cross.setAttribute('x2', x);
      cross.setAttribute('y1', plotT);
      cross.setAttribute('y2', plotB);
      cross.style.display = '';
    });

    overlay.addEventListener('pointerleave', function () {
      cross.style.display = 'none';
    });
  }

  function scan(root) {
    root.querySelectorAll('.vb-hover').forEach(init);
  }

  function start() {
    scan(document);
    new MutationObserver(function (mutations) {
      mutations.forEach(function (m) {
        m.addedNodes.forEach(function (node) {
          if (node.nodeType !== 1) return;
          if (node.matches && node.matches('.vb-hover')) init(node);
          else if (node.querySelectorAll) scan(node);
        });
      });
    }).observe(document.body, { childList: true, subtree: true });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})();
