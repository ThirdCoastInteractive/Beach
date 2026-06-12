// Edge bundling interaction: hovering a node highlights its connected
// links and dims everything else.

(function () {
  function init(svg) {
    if (svg.__bundleWired) return;
    svg.__bundleWired = true;

    var nodes = svg.querySelectorAll('.bundle-node');
    var links = svg.querySelectorAll('.bundle-link');

    function highlight(nodeId) {
      links.forEach(function (l) {
        if (l.getAttribute('data-src') === nodeId || l.getAttribute('data-dst') === nodeId) {
          l.style.opacity = '0.7';
          l.style.strokeWidth = '1';
        } else {
          l.style.opacity = '0.03';
        }
      });
      nodes.forEach(function (n) {
        if (n.getAttribute('data-node-id') === nodeId) {
          n.style.r = '2.5';
          n.style.filter = 'brightness(1.4)';
        } else {
          n.style.opacity = '0.2';
        }
      });
    }

    function reset() {
      links.forEach(function (l) { l.style.opacity = ''; l.style.strokeWidth = ''; });
      nodes.forEach(function (n) { n.style.opacity = ''; n.style.r = ''; n.style.filter = ''; });
    }

    nodes.forEach(function (node) {
      node.addEventListener('mouseenter', function () {
        highlight(node.getAttribute('data-node-id'));
      });
      node.addEventListener('mouseleave', reset);
    });
  }

  function scan(root) {
    root.querySelectorAll('.bundle-svg').forEach(init);
  }

  function start() {
    scan(document);
    new MutationObserver(function (mutations) {
      mutations.forEach(function (m) {
        m.addedNodes.forEach(function (node) {
          if (node.nodeType !== 1) return;
          if (node.matches && node.matches('.bundle-svg')) init(node);
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
