// Unified chart toolbar. Injects a toolbar into each dashboard widget
// header that contains a chart. Toolbar buttons toggle grid, legend,
// and fullscreen. Theme gets a dropdown picker.

(function () {
  // 'munsell' is the default: it is the :root series palette, so picking
  // it clears data-chart-theme instead of setting it. The rest switch the
  // widget to an alternate [data-chart-theme="…"] palette.
  var THEMES = [
    { key: 'munsell', label: 'Munsell' },
    { key: 'warm', label: 'Warm' },
    { key: 'cool', label: 'Cool' },
    { key: 'neon', label: 'Neon' },
    { key: 'terminal', label: 'Terminal' },
    { key: 'amber', label: 'Amber' },
    { key: 'mono', label: 'Mono' },
    { key: 'earth', label: 'Earth' },
  ];

  function initWidget(widget) {
    if (widget.__chartToolbar) return;
    var body = widget.querySelector('.dash-widget-body');
    if (!body) return;
    var svg = body.querySelector('.chart-svg');
    if (!svg) return;
    widget.__chartToolbar = true;

    var header = widget.querySelector('.dash-widget-header');
    if (!header) return;

    var bar = document.createElement('div');
    bar.className = 'chart-toolbar-inline';

    // Grid toggle
    var gridBtn = btn('Grid', function () {
      body.querySelectorAll('.cline-grid, line[stroke-opacity]').forEach(function (el) {
        el.style.display = el.style.display === 'none' ? '' : 'none';
      });
    });
    bar.appendChild(gridBtn);

    // Legend toggle
    var legendBtn = btn('Legend', function () {
      body.querySelectorAll('.cline-legend').forEach(function (el) {
        el.style.display = el.style.display === 'none' ? '' : 'none';
      });
    });
    bar.appendChild(legendBtn);

    // Theme dropdown
    var themeWrap = document.createElement('div');
    themeWrap.className = 'chart-toolbar-dropdown';

    var themeBtn = document.createElement('button');
    themeBtn.className = 'chart-toolbar-btn';
    themeBtn.textContent = 'Theme';
    themeBtn.type = 'button';
    themeWrap.appendChild(themeBtn);

    var menu = document.createElement('div');
    menu.className = 'chart-toolbar-menu';
    THEMES.forEach(function (t) {
      var item = document.createElement('button');
      item.className = 'chart-toolbar-menu-item';
      item.type = 'button';
      item.textContent = t.label;
      item.dataset.theme = t.key;
      item.addEventListener('click', function (e) {
        e.stopPropagation();
        if (t.key === 'munsell') {
          widget.removeAttribute('data-chart-theme');
        } else {
          widget.setAttribute('data-chart-theme', t.key);
        }
        menu.classList.remove('chart-toolbar-menu-open');
      });
      menu.appendChild(item);
    });
    themeWrap.appendChild(menu);

    themeBtn.addEventListener('click', function (e) {
      e.stopPropagation();
      menu.classList.toggle('chart-toolbar-menu-open');
    });

    bar.appendChild(themeWrap);

    // Fullscreen
    var fsBtn = btn('Expand', function () {
      if (document.fullscreenElement === widget) {
        document.exitFullscreen();
      } else {
        widget.requestFullscreen().catch(function () {});
      }
    });
    bar.appendChild(fsBtn);

    header.appendChild(bar);
  }

  function btn(label, onclick) {
    var b = document.createElement('button');
    b.className = 'chart-toolbar-btn';
    b.type = 'button';
    b.textContent = label;
    b.addEventListener('click', onclick);
    return b;
  }

  // Close any open menu on outside click.
  document.addEventListener('click', function () {
    document.querySelectorAll('.chart-toolbar-menu-open').forEach(function (m) {
      m.classList.remove('chart-toolbar-menu-open');
    });
  });

  function scan(root) {
    root.querySelectorAll('.dash-widget').forEach(initWidget);
  }

  function start() {
    scan(document);
    new MutationObserver(function (mutations) {
      mutations.forEach(function (m) {
        m.addedNodes.forEach(function (node) {
          if (node.nodeType !== 1) return;
          if (node.classList && node.classList.contains('dash-widget')) initWidget(node);
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
