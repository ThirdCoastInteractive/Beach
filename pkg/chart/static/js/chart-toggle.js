// Click-to-toggle highlight for chart elements. Clicking any
// interactive chart element pins the highlight state; clicking again
// or clicking empty space releases it.

(function () {
  var pinned = null;
  var pinnedSvg = null;

  var SELECTORS = '.chart-hover-fade, .chart-hover-node, .chord-arc, .chord-ribbon, .bundle-node';

  document.addEventListener('click', function (e) {
    // e.target is not always an Element (document/window on synthetic or
    // boundary events) and then has no .closest; treat those as "no hit".
    var el = e.target.closest && e.target.closest(SELECTORS);
    if (!el) {
      if (pinned) unpin();
      return;
    }

    var svg = el.closest('.chart-svg');
    if (!svg) return;

    if (pinned === el) {
      unpin();
      return;
    }

    if (pinned) unpin();

    pinned = el;
    pinnedSvg = svg;

    // For chord/bundle, simulate a mouseenter on the element to trigger
    // the chart-specific hover module's highlight, then freeze it.
    var enterEvt = new MouseEvent('mouseenter', { bubbles: false });
    el.dispatchEvent(enterEvt);

    svg.classList.add('chart-pinned');
    el.classList.add('chart-pin-active');
    svg.dataset.pinFrozen = '1';
  });

  // Block mouseleave resets while pinned.
  document.addEventListener('mouseleave', function (e) {
    if (!pinnedSvg || !pinnedSvg.dataset.pinFrozen) return;
    // Capture-phase mouseleave fires for the window/document boundary too,
    // where the target is not an Element and has no .closest.
    var el = e.target.closest && e.target.closest(SELECTORS);
    if (el && pinnedSvg.contains(el)) {
      e.stopPropagation();
    }
  }, true);

  function unpin() {
    if (!pinned) return;
    if (pinnedSvg) {
      pinnedSvg.classList.remove('chart-pinned');
      delete pinnedSvg.dataset.pinFrozen;
    }
    pinned.classList.remove('chart-pin-active');

    // Fire mouseleave to let the chart-specific hover module reset.
    var leaveEvt = new MouseEvent('mouseleave', { bubbles: false });
    pinned.dispatchEvent(leaveEvt);

    pinned = null;
    pinnedSvg = null;
  }
})();
