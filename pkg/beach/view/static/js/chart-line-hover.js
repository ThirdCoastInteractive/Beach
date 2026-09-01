// Line chart interaction with synchronised markers. The chart is
// server-rendered SVG; this module only positions a crosshair, handles,
// a selection band, and a tooltip over it. No D3, no chart drawing.
//
// Interaction model (shared across every line chart on the page):
//   - Hover: the marker snaps to the NEAREST real sample (no
//     interpolation); the sampled point is highlighted and the tooltip
//     shows its exact value. Every chart shows the marker at the same
//     time, snapped to its own nearest sample; charts whose domain does
//     not cover the time stay hidden.
//   - Click: pins the marker at that sample. Pinned markers persist when
//     the pointer leaves. Click again (anywhere) to unpin.
//   - Drag: selects a time window; a shaded band spans it and the
//     tooltip shows the change (delta and percent) for each series over
//     the window. The window persists until a click clears it.

import { announce, liveRegion } from './chart-keys.js';

const NS = 'http://www.w3.org/2000/svg';

// Registry of charts; shared interaction state drives all of them.
const charts = [];
let mode = 'idle'; // idle | hover | pinned | select
let hoverT = null;
let pinT = null;
let selA = null;
let selB = null;

function redraw() {
    for (const c of charts) c.apply();
}

let tipEl = null;
function tooltip() {
    if (tipEl) return tipEl;
    tipEl = document.createElement('div');
    tipEl.className = 'chart-tooltip';
    tipEl.style.display = 'none';
    document.body.appendChild(tipEl);
    return tipEl;
}
function hideTip() {
    if (tipEl) tipEl.style.display = 'none';
}
function showTipHTML(html, x, y) {
    const t = tooltip();
    t.innerHTML = html;
    t.style.display = 'block';
    const pad = 14;
    const r = t.getBoundingClientRect();
    let px = x + pad;
    let py = y + pad;
    if (px + r.width > window.innerWidth - 4) px = x - r.width - pad;
    if (py + r.height > window.innerHeight - 4) py = y - r.height - pad;
    t.style.left = `${Math.max(4, px)}px`;
    t.style.top = `${Math.max(4, py)}px`;
}

function clearAll() {
    mode = 'idle';
    hoverT = pinT = selA = selB = null;
    redraw();
    hideTip();
}

function escapeHTML(s) {
    return String(s).replace(/[&<>"]/g, c =>
        ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
}
function fmt(n) {
    return (Math.round(n * 10) / 10).toLocaleString('en-US');
}
function signed(n) {
    const s = fmt(Math.abs(n));
    return (n >= 0 ? '+' : '−') + s;
}

function mkEl(tag, cls) {
    const e = document.createElementNS(NS, tag);
    e.setAttribute('class', cls);
    e.style.display = 'none';
    return e;
}

function initLineHover(fig) {
    const svg = fig.querySelector('svg.cline');
    const plot = fig.querySelector('.cline-plot');
    const overlay = fig.querySelector('.cline-overlay');
    const script = document.getElementById(fig.id + '-hover');
    if (!svg || !plot || !overlay || !script) return;
    if (overlay.__lineWired) return;
    overlay.__lineWired = true;

    let data;
    try {
        data = JSON.parse(script.textContent);
    } catch (e) {
        return;
    }
    if (!data.series || !data.series.length) return;

    const hidden = new Set(); // series indices toggled off via the legend

    svg.querySelectorAll('.cline-crosshair, .cline-handle, .cline-band, .cline-startline')
        .forEach(n => n.remove());

    const band = mkEl('rect', 'cline-band');
    const startLine = mkEl('line', 'cline-startline');
    const cross = mkEl('line', 'cline-crosshair');
    svg.insertBefore(band, overlay);
    svg.insertBefore(startLine, overlay);
    svg.insertBefore(cross, overlay);
    const handles = data.series.map(s => {
        const c = mkEl('circle', 'cline-handle');
        c.setAttribute('r', '6');
        c.setAttribute('fill', s.color);
        svg.insertBefore(c, overlay);
        return c;
    });

    const grid = data.series[0].pts;
    const tmin = data.tmin;
    const tmax = data.tmax;

    const inDomain = t => t != null && t >= tmin - 1e-6 && t <= tmax + 1e-6;
    const clamp01 = v => Math.max(0, Math.min(1, v));

    function nearestBy(key, val) {
        let bi = 0;
        let bd = Infinity;
        for (let i = 0; i < grid.length; i++) {
            const d = Math.abs(grid[i][key] - val);
            if (d < bd) { bd = d; bi = i; }
        }
        return bi;
    }
    // Convert a viewport (screen) point to SVG user-unit coordinates.
    // This correctly handles any CSS transforms, flex sizing, etc.
    function screenToSVG(clientX, clientY) {
        const pt = svg.createSVGPoint();
        pt.x = clientX;
        pt.y = clientY;
        const ctm = svg.getScreenCTM();
        if (!ctm) return { x: 0, y: 0 };
        return pt.matrixTransform(ctm.inverse());
    }

    function plotRect() {
        const r = plot.getBoundingClientRect();
        const tl = screenToSVG(r.left, r.top);
        const br = screenToSVG(r.right, r.bottom);
        return { x: tl.x, y: tl.y, w: br.x - tl.x, h: br.y - tl.y, css: r };
    }

    function hideMarks() {
        band.style.display = 'none';
        startLine.style.display = 'none';
        cross.style.display = 'none';
        handles.forEach(h => (h.style.display = 'none'));
    }
    function drawCross(x, pr) {
        cross.setAttribute('x1', x);
        cross.setAttribute('x2', x);
        cross.setAttribute('y1', pr.y);
        cross.setAttribute('y2', pr.y + pr.h);
        cross.style.display = '';
    }
    function drawHandles(idx, x, pr) {
        data.series.forEach((s, k) => {
            if (hidden.has(k)) { handles[k].style.display = 'none'; return; }
            const p = s.pts[idx];
            handles[k].setAttribute('cx', x);
            handles[k].setAttribute('cy', pr.y + (1 - p.vy) * pr.h);
            handles[k].style.display = '';
        });
    }

    function drawMark(idx) {
        const pr = plotRect();
        if (pr.w <= 0) { hideMarks(); return; }
        band.style.display = 'none';
        startLine.style.display = 'none';
        const x = pr.x + grid[idx].fx * pr.w;
        drawCross(x, pr);
        drawHandles(idx, x, pr);
    }

    function drawBand(ia, ib) {
        const pr = plotRect();
        if (pr.w <= 0) { hideMarks(); return; }
        const xa = pr.x + grid[ia].fx * pr.w;
        const xb = pr.x + grid[ib].fx * pr.w;
        band.setAttribute('x', Math.min(xa, xb));
        band.setAttribute('y', pr.y);
        band.setAttribute('width', Math.abs(xb - xa));
        band.setAttribute('height', pr.h);
        band.style.display = '';
        startLine.setAttribute('x1', xa);
        startLine.setAttribute('x2', xa);
        startLine.setAttribute('y1', pr.y);
        startLine.setAttribute('y2', pr.y + pr.h);
        startLine.style.display = '';
        drawCross(xb, pr);
        drawHandles(ib, xb, pr);
    }

    function apply() {
        if (!plot.isConnected) return;
        if (mode === 'select' && selA != null && selB != null) {
            const lo = Math.min(selA, selB);
            const hi = Math.max(selA, selB);
            if (hi < tmin - 1e-6 || lo > tmax + 1e-6) { hideMarks(); return; }
            drawBand(nearestBy('t', Math.max(tmin, lo)), nearestBy('t', Math.min(tmax, hi)));
            return;
        }
        const t = mode === 'pinned' ? pinT : mode === 'hover' ? hoverT : null;
        if (!inDomain(t)) { hideMarks(); return; }
        drawMark(nearestBy('t', t));
    }

    charts.push({ apply });

    function valueTip(idx, x, y) {
        const unit = data.unit ? ' ' + data.unit : '';
        const head = escapeHTML(data.xlabels[idx] || '');
        const body = data.series
            .map((s, k) =>
                hidden.has(k) ? '' :
                `<div class="chart-tooltip-row">` +
                `<span class="chart-tooltip-label">${escapeHTML(s.label)}</span>` +
                `<span class="chart-tooltip-value">${fmt(s.pts[idx].v)}${escapeHTML(unit)}</span></div>`)
            .join('');
        showTipHTML(`<div class="chart-tooltip-head">${head}</div>` +
            `<div class="chart-tooltip-rows">${body}</div>`, x, y);
    }

    function windowTip(ia, ib, x, y) {
        const lo = Math.min(ia, ib);
        const hi = Math.max(ia, ib);
        const unit = data.unit ? ' ' + data.unit : '';
        const head = escapeHTML(data.xlabels[lo] || '') + ' → ' + escapeHTML(data.xlabels[hi] || '');
        const dt = grid[hi].t - grid[lo].t;
        const body = data.series
            .map((s, k) => {
                if (hidden.has(k)) return '';
                let mn = Infinity, mx = -Infinity, sum = 0, cnt = 0;
                for (let i = lo; i <= hi; i++) {
                    const v = s.pts[i].v;
                    if (v < mn) mn = v;
                    if (v > mx) mx = v;
                    sum += v;
                    cnt++;
                }
                const a = s.pts[lo].v;
                const b = s.pts[hi].v;
                const d = b - a;
                const pctTxt = a !== 0 ? ` (${signed((d / Math.abs(a)) * 100)}%)` : '';
                const cls = d > 0 ? 'cline-up' : d < 0 ? 'cline-down' : '';
                let stats = `min ${fmt(mn)} · max ${fmt(mx)} · avg ${fmt(cnt ? sum / cnt : 0)}`;
                if (data.rateUnit && dt !== 0) {
                    stats += ` · ${signed(d / dt)}${escapeHTML(unit)}/${escapeHTML(data.rateUnit)}`;
                }
                return `<div class="chart-tooltip-row">` +
                    `<span class="chart-tooltip-label">${escapeHTML(s.label)}</span>` +
                    `<span class="chart-tooltip-value ${cls}">${signed(d)}${escapeHTML(unit)}${pctTxt}</span></div>` +
                    `<div class="chart-tooltip-row chart-tooltip-sub">` +
                    `<span class="chart-tooltip-stat">${escapeHTML(stats)}</span></div>`;
            })
            .join('');
        showTipHTML(`<div class="chart-tooltip-head">${head}</div>` +
            `<div class="chart-tooltip-rows">${body}</div>`, x, y);
    }

    function idxAt(ev) {
        const r = plot.getBoundingClientRect();
        return nearestBy('fx', clamp01((ev.clientX - r.left) / r.width));
    }

    // Continuous time under the cursor, interpolated across this chart's
    // x range. Broadcasting this (rather than a sample-snapped time) lets
    // every other chart snap to ITS OWN nearest sample, so a coarse
    // source chart no longer makes fine charts jump at its granularity.
    function timeAt(ev) {
        const r = plot.getBoundingClientRect();
        const fx = clamp01((ev.clientX - r.left) / r.width);
        let i = 0;
        while (i < grid.length - 1 && grid[i + 1].fx < fx) i++;
        const j = Math.min(i + 1, grid.length - 1);
        const span = grid[j].fx - grid[i].fx;
        const lt = span > 0 ? (fx - grid[i].fx) / span : 0;
        return grid[i].t + (grid[j].t - grid[i].t) * lt;
    }

    let pdown = false;
    let dragging = false;
    let downX = 0;
    let downIdx = 0;

    overlay.addEventListener('pointerdown', ev => {
        pdown = true;
        dragging = false;
        downX = ev.clientX;
        downIdx = idxAt(ev);
    });

    overlay.addEventListener('pointermove', ev => {
        if (pdown) {
            if (!dragging && Math.abs(ev.clientX - downX) > 5) dragging = true;
            if (dragging) {
                const ci = idxAt(ev);
                mode = 'select';
                selA = grid[downIdx].t;
                selB = grid[ci].t;
                redraw();
                windowTip(downIdx, ci, ev.clientX, ev.clientY);
            }
            return;
        }
        if (mode === 'pinned' || mode === 'select') return; // frozen until cleared
        mode = 'hover';
        hoverT = timeAt(ev); // continuous; each chart snaps to its own grid
        redraw();
        valueTip(idxAt(ev), ev.clientX, ev.clientY); // source tooltip uses its nearest sample
    });

    overlay.addEventListener('pointerleave', () => {
        if (mode === 'hover') clearAll();
    });

    // pointerup on window so a release outside the chart still lands.
    window.addEventListener('pointerup', ev => {
        if (!pdown) return;
        pdown = false;
        if (dragging) { dragging = false; return; } // keep the window
        // a click: clear a pin/selection, or pin the sample under it
        if (mode === 'pinned' || mode === 'select') { clearAll(); return; }
        mode = 'pinned';
        pinT = grid[downIdx].t;
        redraw();
        valueTip(downIdx, ev.clientX, ev.clientY);
    });

    // Legend toggle: hide/show a series and its handle. Visual only; the
    // axis does not rescale (that is a server re-render concern).
    fig.querySelectorAll('.cline-legend-item').forEach(btn => {
        btn.addEventListener('click', () => {
            const k = Number(btn.dataset.series);
            if (hidden.has(k)) {
                hidden.delete(k);
                btn.classList.remove('cline-off');
            } else {
                hidden.add(k);
                btn.classList.add('cline-off');
            }
            svg.querySelectorAll(`[data-series="${k}"]`).forEach(el => {
                el.style.display = hidden.has(k) ? 'none' : '';
            });
            redraw();
        });
    });

    // Keyboard stepping for accessibility: focus the chart and arrow
    // through samples. Pins the synced marker and announces the value.
    const live = liveRegion(fig);
    let kbIdx = -1;
    function announceIdx(idx) {
        if (!live) return;
        const unit = data.unit ? ' ' + data.unit : '';
        const parts = data.series
            .map((s, k) => (hidden.has(k) ? null : `${s.label} ${fmt(s.pts[idx].v)}${unit}`))
            .filter(Boolean);
        announce(live, `${data.xlabels[idx] || ''}: ${parts.join(', ')}`);
    }
    // kbIdx tracks here rather than only in the key handler, so arrowing after a
    // mouse click continues from the sample under the pointer instead of
    // restarting at one end.
    function pinTo(idx) {
        kbIdx = idx;
        mode = 'pinned';
        pinT = grid[idx].t;
        redraw();
        const r = plot.getBoundingClientRect();
        valueTip(idx, r.left + grid[idx].fx * r.width, r.top);
        announceIdx(idx);
    }
    fig.addEventListener('keydown', ev => {
        const n = grid.length;
        if (ev.key === 'ArrowRight' || ev.key === 'ArrowLeft') {
            const step = ev.key === 'ArrowRight' ? 1 : -1;
            kbIdx = kbIdx < 0 ? (step > 0 ? 0 : n - 1) : Math.max(0, Math.min(n - 1, kbIdx + step));
            ev.preventDefault();
            pinTo(kbIdx);
        } else if (ev.key === 'Home') {
            kbIdx = 0;
            ev.preventDefault();
            pinTo(0);
        } else if (ev.key === 'End') {
            kbIdx = n - 1;
            ev.preventDefault();
            pinTo(n - 1);
        } else if (ev.key === 'Escape') {
            // Only swallow Escape when there is something to let go of;
            // otherwise it belongs to whatever dialog encloses the chart.
            if (kbIdx < 0 && mode === 'idle') return;
            ev.preventDefault();
            kbIdx = -1;
            clearAll();
        }
    });
}

function scan(root) {
    for (const fig of root.querySelectorAll('[data-chart-keys="line"]')) initLineHover(fig);
}

function start() {
    scan(document);
    new MutationObserver(mutations => {
        for (const m of mutations) {
            if (m.target.nodeType === 1 && m.target.matches?.('[data-chart-keys="line"]')) {
                initLineHover(m.target);
            }
            for (const node of m.addedNodes) {
                if (node.nodeType !== 1) continue;
                if (node.matches?.('[data-chart-keys="line"]')) initLineHover(node);
                else scan(node);
            }
        }
    }).observe(document.body, { childList: true, subtree: true });
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
} else {
    start();
}
