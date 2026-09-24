// Chord diagram interaction: hovering an outer arc highlights its
// connected ribbons and dims the rest.

(function () {
  function init(svg) {
    if (svg.__chordWired) return;
    svg.__chordWired = true;

    var arcs = svg.querySelectorAll('.chord-arc');
    var ribbons = svg.querySelectorAll('.chord-ribbon');

    function highlight(groupIdx) {
      var g = String(groupIdx);
      ribbons.forEach(function (r) {
        if (r.getAttribute('data-src') === g || r.getAttribute('data-dst') === g) {
          r.style.opacity = '0.7';
        } else {
          r.style.opacity = '0.06';
        }
      });
      arcs.forEach(function (a) {
        if (a.getAttribute('data-group') === g) {
          a.style.filter = 'brightness(1.3)';
        } else {
          a.style.opacity = '0.3';
        }
      });
    }

    function reset() {
      ribbons.forEach(function (r) { r.style.opacity = ''; });
      arcs.forEach(function (a) { a.style.opacity = ''; a.style.filter = ''; });
    }

    arcs.forEach(function (arc) {
      arc.addEventListener('mouseenter', function () {
        highlight(arc.getAttribute('data-group'));
      });
      arc.addEventListener('mouseleave', reset);
    });

    ribbons.forEach(function (r) {
      r.addEventListener('mouseenter', function () {
        ribbons.forEach(function (other) {
          if (other === r) { other.style.opacity = '0.8'; }
          else { other.style.opacity = '0.06'; }
        });
      });
      r.addEventListener('mouseleave', reset);
    });
  }

  function scan(root) {
    root.querySelectorAll('.chord-svg').forEach(init);
  }

  function start() {
    scan(document);
    new MutationObserver(function (mutations) {
      mutations.forEach(function (m) {
        m.addedNodes.forEach(function (node) {
          if (node.nodeType !== 1) return;
          if (node.matches && node.matches('.chord-svg')) init(node);
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
