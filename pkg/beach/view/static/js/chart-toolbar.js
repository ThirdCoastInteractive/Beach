// Unified chart toolbar. Injects a toolbar into each dashboard widget
// header that contains a chart. Toolbar buttons toggle grid, legend,
// and fullscreen. Theme gets a dropdown picker.
//
// Because these controls are built here rather than rendered by the kit, they
// carry their own semantics: nothing in templ can see them, and neither can
// beach-vet. Two rules apply.
//
//   - A toggle says whether it is on. Grid and Legend are pressed/unpressed
//     buttons, not fire-and-forget ones, so they carry aria-pressed and keep it
//     in step; without it their state exists only in the chart itself.
//   - A disclosure says whether it is open, and can be closed from the keyboard.
//     The theme trigger carries aria-expanded and aria-haspopup, Escape closes
//     the menu and puts focus back on the trigger, and an outside click updates
//     the trigger too.
//
// No accessible *name* is invented here: every control is named by its own
// visible text and the menu is labelled by its trigger. Those visible strings
// are still English literals, because the framework's client modules cannot
// reach the i18n catalog — recorded as a limitation in
// docs/rfc/06-accessibility.md.

(function () {
  var seq = 0;
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

    var uid = 'chart-toolbar-' + ++seq;

    // Grid toggle. Starts pressed: the grid is drawn by default.
    var gridBtn = toggle('Grid', true, function (on) {
      body.querySelectorAll('.cline-grid, line[stroke-opacity]').forEach(function (el) {
        el.style.display = on ? '' : 'none';
      });
    });
    bar.appendChild(gridBtn);

    // Legend toggle. Also starts pressed.
    var legendBtn = toggle('Legend', true, function (on) {
      body.querySelectorAll('.cline-legend').forEach(function (el) {
        el.style.display = on ? '' : 'none';
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
    themeBtn.id = uid + '-theme';
    themeBtn.setAttribute('aria-haspopup', 'menu');
    themeBtn.setAttribute('aria-expanded', 'false');
    themeBtn.setAttribute('aria-controls', uid + '-menu');
    themeWrap.appendChild(themeBtn);

    var menu = document.createElement('div');
    menu.className = 'chart-toolbar-menu';
    menu.id = uid + '-menu';
    menu.setAttribute('role', 'menu');
    // Labelled by the trigger, rather than by a string invented in here.
    menu.setAttribute('aria-labelledby', themeBtn.id);
    THEMES.forEach(function (t) {
      var item = document.createElement('button');
      item.className = 'chart-toolbar-menu-item';
      item.type = 'button';
      item.setAttribute('role', 'menuitem');
      item.textContent = t.label;
      item.dataset.theme = t.key;
      item.addEventListener('click', function (e) {
        e.stopPropagation();
        if (t.key === 'munsell') {
          widget.removeAttribute('data-chart-theme');
        } else {
          widget.setAttribute('data-chart-theme', t.key);
        }
        setMenuOpen(themeBtn, menu, false, true);
      });
      menu.appendChild(item);
    });
    themeWrap.appendChild(menu);

    themeBtn.addEventListener('click', function (e) {
      e.stopPropagation();
      setMenuOpen(themeBtn, menu, !menu.classList.contains('chart-toolbar-menu-open'), false);
    });

    // Escape closes from anywhere inside the disclosure and returns focus to the
    // trigger. Without it a keyboard user who opens the menu has no way out that
    // does not involve tabbing through every theme.
    themeWrap.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape') return;
      if (!menu.classList.contains('chart-toolbar-menu-open')) return;
      e.stopPropagation();
      setMenuOpen(themeBtn, menu, false, true);
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

  // toggle builds a two-state button that reports its state. onchange receives
  // the new state, so the caller sets visibility outright instead of flipping
  // whatever the DOM happened to be showing — which is what keeps the button's
  // aria-pressed and the chart from drifting apart.
  function toggle(label, on, onchange) {
    var b = btn(label, function () {
      on = !on;
      b.setAttribute('aria-pressed', String(on));
      onchange(on);
    });
    b.setAttribute('aria-pressed', String(on));
    return b;
  }

  // setMenuOpen is the single place the menu's visual state and its trigger's
  // announced state change together, so they cannot disagree.
  function setMenuOpen(trigger, menu, open, refocus) {
    menu.classList.toggle('chart-toolbar-menu-open', open);
    trigger.setAttribute('aria-expanded', String(open));
    if (!open && refocus) trigger.focus();
  }

  // Close any open menu on outside click, keeping each trigger's state honest.
  document.addEventListener('click', function () {
    document.querySelectorAll('.chart-toolbar-menu-open').forEach(function (m) {
      var trigger = m.previousElementSibling;
      m.classList.remove('chart-toolbar-menu-open');
      if (trigger && trigger.hasAttribute('aria-expanded')) {
        trigger.setAttribute('aria-expanded', 'false');
      }
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
