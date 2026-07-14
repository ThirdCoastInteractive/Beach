// Chart tooltip for server-rendered SVG charts. Elements with a
// data-tip attribute show a floating tooltip on hover. The tooltip
// HTML is pre-built server-side and stored in the attribute.
// No D3 — just pointer events and a shared floating div.

let tipEl = null;

function tip() {
    if (tipEl) return tipEl;
    tipEl = document.createElement('div');
    tipEl.className = 'chart-tooltip';
    tipEl.setAttribute('role', 'tooltip');
    tipEl.style.display = 'none';
    document.body.appendChild(tipEl);
    return tipEl;
}

function show(html, e) {
    const t = tip();
    t.innerHTML = html;
    t.style.display = 'block';
    move(e);
}

function move(e) {
    const t = tip();
    const pad = 14;
    const rect = t.getBoundingClientRect();
    let x = e.clientX + pad;
    let y = e.clientY + pad;
    if (x + rect.width > window.innerWidth - 4) x = e.clientX - rect.width - pad;
    if (y + rect.height > window.innerHeight - 4) y = e.clientY - rect.height - pad;
    t.style.left = `${Math.max(4, x)}px`;
    t.style.top = `${Math.max(4, y)}px`;
}

function hide() {
    if (tipEl) tipEl.style.display = 'none';
}

// Capture-phase listeners on document see every pointerenter/pointerleave,
// including ones whose target is the document itself (the pointer entering or
// leaving the window). Those targets are not Elements and have no .closest,
// so guard before walking up — same idiom as geomap-interact.js.
document.addEventListener('pointerenter', (e) => {
    const el = e.target.closest && e.target.closest('[data-tip]');
    if (el) show(el.getAttribute('data-tip'), e);
}, true);

document.addEventListener('pointermove', (e) => {
    if (tipEl && tipEl.style.display !== 'none') move(e);
}, true);

document.addEventListener('pointerleave', (e) => {
    const el = e.target.closest && e.target.closest('[data-tip]');
    if (el) hide();
}, true);
