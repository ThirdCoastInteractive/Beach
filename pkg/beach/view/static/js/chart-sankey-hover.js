// Sankey interaction: hovering a node bar (or its label) highlights the
// whole chain that flows through it -- every upstream feeder and every
// downstream consumer -- and dims everything off the chain. The chain is
// the directed reachability cone in both directions, so hovering UPS-1
// lights up its ATS / utility feeders and the halls it serves, but not the
// sibling branches that merely share a downstream node.

(function () {
  function init(svg) {
    if (svg.__sankeyWired) return;
    svg.__sankeyWired = true;

    var links = Array.prototype.slice.call(svg.querySelectorAll('.sankey-link'));
    var triggers = Array.prototype.slice.call(svg.querySelectorAll('[data-node-id]')); // bars + labels

    // Adjacency by node id: out-links (downstream) and in-links (upstream).
    var out = {}, into = {};
    links.forEach(function (l, i) {
      var s = l.getAttribute('data-src'), t = l.getAttribute('data-dst');
      (out[s] = out[s] || []).push(i);
      (into[t] = into[t] || []).push(i);
    });

    // chain returns the set of node ids and link indices reachable from
    // startId, walking out-links forward and in-links backward.
    function chain(startId) {
      var nodes = {}, edges = {};
      nodes[startId] = true;
      function walk(adj, endAttr) {
        var stack = [startId];
        while (stack.length) {
          var id = stack.pop();
          (adj[id] || []).forEach(function (li) {
            if (edges[li]) return;
            edges[li] = true;
            var next = links[li].getAttribute(endAttr);
            if (!nodes[next]) { nodes[next] = true; stack.push(next); }
          });
        }
      }
      walk(out, 'data-dst');  // downstream consumers
      walk(into, 'data-src'); // upstream feeders
      return { nodes: nodes, edges: edges };
    }

    function highlight(startId) {
      var c = chain(startId);
      links.forEach(function (l, i) {
        l.style.opacity = c.edges[i] ? '0.78' : '0.05';
      });
      triggers.forEach(function (el) {
        var on = !!c.nodes[el.getAttribute('data-node-id')];
        el.style.opacity = on ? '1' : '0.18';
        if (el.classList.contains('chart-hover-node')) {
          el.style.filter = on ? 'brightness(1.25)' : '';
        }
      });
    }

    function reset() {
      links.forEach(function (l) { l.style.opacity = ''; });
      triggers.forEach(function (el) { el.style.opacity = ''; el.style.filter = ''; });
    }

    triggers.forEach(function (el) {
      el.addEventListener('mouseenter', function () {
        highlight(el.getAttribute('data-node-id'));
      });
      el.addEventListener('mouseleave', reset);
    });
  }

  function scan(root) {
    root.querySelectorAll('.sankey-svg').forEach(init);
  }

  function start() {
    scan(document);
    new MutationObserver(function (mutations) {
      mutations.forEach(function (m) {
        m.addedNodes.forEach(function (node) {
          if (node.nodeType !== 1) return;
          if (node.matches && node.matches('.sankey-svg')) init(node);
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
