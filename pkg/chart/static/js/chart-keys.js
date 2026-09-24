// Keyboard access for charts, and the small shared helpers the chart modules
// each used to reinvent.
//
// Every chart SVG in this kit is aria-hidden: a bag of paths and numbers says
// nothing read node by node, and a nameless graphic announces "image" and stops.
// That is the right call for the picture — but it means a chart whose only
// interaction is hover is a chart that does not exist for anyone who cannot use
// a mouse. WCAG 2.1.1 asks that all functionality be operable from a keyboard,
// and "all" includes the tooltip that holds the actual numbers.
//
// The design has one trick behind it. chart-toggle.js already drives the
// highlight modules by *synthesising* mouseenter/mouseleave, so a keyboard layer
// that does the same inherits every existing highlight behaviour without those
// modules being edited at all. And the announcement text is already in the DOM:
// data-tip carries server-rendered, already-localized HTML, which is how this
// reaches the i18n catalog that client modules otherwise cannot.
//
// Exports are the four things five modules had each grown their own copy of.

// observeAll runs init over everything matching selector, now and as the DOM
// changes. Datastar patches fragments in place, so a chart can arrive — or be
// replaced — long after load; flag is the property name that keeps init
// idempotent when it does.
export function observeAll(selector, init, flag) {
  const wire = (el) => {
    if (el[flag]) return;
    el[flag] = true;
    init(el);
  };
  const scan = (root) => {
    if (root.matches && root.matches(selector)) wire(root);
    if (root.querySelectorAll) for (const el of root.querySelectorAll(selector)) wire(el);
  };
  const start = () => {
    scan(document);
    new MutationObserver((records) => {
      for (const rec of records) {
        // Both added subtrees and mutated targets: an SSE morph changes an
        // existing element's children rather than replacing the element, which
        // a childList-only observer misses.
        for (const node of rec.addedNodes) if (node.nodeType === 1) scan(node);
        if (rec.target && rec.target.nodeType === 1) scan(rec.target);
      }
    }).observe(document.body, { childList: true, subtree: true });
  };
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
}

// liveRegion finds the announcement region inside a chart figure.
//
// It matches on [aria-live] and not on .sr-only, which is the bug this replaces:
// a figure with a Describe() caption has *two* screen-reader-only children, the
// caption comes first, and selecting by class quietly overwrote the chart's text
// alternative while announcing nothing.
export function liveRegion(fig) {
  return fig.querySelector('[aria-live]');
}

// announce puts text into a live region so it is actually read.
//
// Assigning the same string twice is a no-op for most screen readers, so
// stepping onto two samples with the same value would say nothing at all. A
// zero-width space, toggled per call, makes each assignment a genuine change
// without changing what is spoken.
let announceToggle = false;
export function announce(region, text) {
  if (!region || !text) return;
  announceToggle = !announceToggle;
  region.textContent = announceToggle ? text + '​' : text;
}

// textOf turns a server-rendered data-tip — which is HTML — into the flat string
// a live region wants. Using the DOM to do it means no HTML is ever parsed by
// hand and nothing is injected: textContent cannot execute anything.
const scratch = document.createElement('div');
export function textOf(html) {
  if (!html) return '';
  scratch.innerHTML = html;
  return scratch.textContent.replace(/\s+/g, ' ').trim();
}

// rovingKeys gives a focusable chart figure the keyboard model the line chart
// already had, generalised: arrows step, Home and End jump, Escape lets go.
//
// The -1 sentinel means "no keyboard position yet", so the first arrow press
// lands on an end rather than on an arbitrary middle. sync() lets a pointer
// interaction move the cursor, so arrowing after a click continues from where
// the mouse left off instead of restarting.
export function rovingKeys(fig, { count, onIndex, onEscape }) {
  let idx = -1;
  const clamp = (i) => Math.max(0, Math.min(count() - 1, i));

  fig.addEventListener('keydown', (ev) => {
    const n = count();
    if (!n) return;
    switch (ev.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        ev.preventDefault();
        idx = idx < 0 ? 0 : clamp(idx + 1);
        onIndex(idx);
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        ev.preventDefault();
        idx = idx < 0 ? n - 1 : clamp(idx - 1);
        onIndex(idx);
        break;
      case 'Home':
        ev.preventDefault();
        idx = 0;
        onIndex(idx);
        break;
      case 'End':
        ev.preventDefault();
        idx = n - 1;
        onIndex(idx);
        break;
      case 'Escape':
        // Only swallow Escape when this widget has something to let go of;
        // otherwise it belongs to whatever dialog encloses the chart.
        if (idx < 0) return;
        ev.preventDefault();
        idx = -1;
        if (onEscape) onEscape();
        break;
      default:
        return;
    }
  });

  // Leaving the chart drops the cursor, so returning to it starts clean rather
  // than resuming a position the user can no longer see highlighted.
  fig.addEventListener('focusout', (ev) => {
    if (fig.contains(ev.relatedTarget)) return;
    if (idx < 0) return;
    idx = -1;
    if (onEscape) onEscape();
  });

  return { sync: (i) => { idx = i; }, index: () => idx };
}

// hoverKeys is the whole keyboard layer for the charts whose interaction is
// "hover a shape, see a tooltip": it walks the shapes in document order and
// drives each one through the module that already knows how to highlight it.
//
// Nothing here knows what a sankey link or a chord arc is. It dispatches the
// same mouseenter/mouseleave the pointer would, which is exactly what
// chart-toggle.js does for pinning, so the four highlight modules need no
// changes and cannot drift out of step with this.
export function hoverKeys(fig, selector) {
  const region = liveRegion(fig);
  let current = null;

  const items = () => Array.from(fig.querySelectorAll(selector));
  const leave = () => {
    if (!current) return;
    current.dispatchEvent(new MouseEvent('mouseleave', { bubbles: false }));
    current.classList.remove('chart-kb-active');
    current = null;
  };

  const keys = rovingKeys(fig, {
    count: () => items().length,
    onIndex(i) {
      const list = items();
      const el = list[i];
      if (!el) return;
      leave();
      current = el;
      el.classList.add('chart-kb-active');
      el.dispatchEvent(new MouseEvent('mouseenter', { bubbles: false }));
      announce(region, textOf(el.dataset.tip) || el.getAttribute('aria-label') || '');
    },
    onEscape: leave,
  });

  // A pointer hover moves the keyboard cursor with it, so the two input methods
  // share one position instead of fighting over it.
  fig.addEventListener('pointerenter', (ev) => {
    const el = ev.target.closest && ev.target.closest(selector);
    if (!el) return;
    const i = items().indexOf(el);
    if (i >= 0) keys.sync(i);
  }, true);

  return keys;
}
