// GeoMap interactivity for the server-rendered Equal Earth choropleth.
// The map itself (outline, graticule, shapes, cities, points) is drawn
// server-side; this module only manipulates the svg viewBox for zoom/pan
// and layers small overlay controls on top. No D3, no chart drawing, no
// fetch — drill-down loading is Datastar's job via each shape's
// data-on:click attribute.
//
// Interaction model:
//   - Wheel: zoom toward the cursor, clamped 1x..12x of the original view.
//   - Drag: pan; clamped so the map cannot be flung out of frame.
//   - Two-finger pinch: zoom toward the gesture midpoint (pointer events).
//   - Double-click / reset button: return to the original viewBox.
//   - +/- buttons: step zoom about the map center.
//   - Shape click (a click, not a drag): dispatch a bubbling `geomap:region`
//     CustomEvent from the figure with detail {code, name, hasData}. The
//     region name is read from the shape's tooltip title
//     (`.chart-tooltip-head span:last-child` of data-tip); hasData is true
//     when the fill is an oklch ramp color rather than the no-data var.
//   - A shape carrying data-on:click gets the `geomap-busy` class (and so
//     does the figure) while Datastar loads the drill target; it clears on
//     the next Datastar fetch-finished event, or after a 3s fallback.
//
// Motion is a continuous exponential chase toward the latest target,
// presented as a GPU-composited CSS transform while in flight; the viewBox
// (a full repaint) is committed once when the view settles. Reduced motion
// makes every move instant.

const SVG_NS = 'http://www.w3.org/2000/svg';
const MAX_ZOOM = 12;           // deepest zoom: viewBox width = w0 / 12
const CHASE_TAU_MS = 100;      // zoom chase smoothing constant (~63% per tau)
const CLICK_SLOP = 6;          // px of motion still counted as a click
const BUSY_FALLBACK_MS = 3000; // remove geomap-busy after this if no settle

function reducedMotion() {
    return window.matchMedia &&
        window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function parseVB(str) {
    const p = String(str || '').trim().split(/\s+/).map(Number);
    if (p.length !== 4 || p.some(Number.isNaN)) return null;
    return { x: p[0], y: p[1], w: p[2], h: p[3] };
}

function vbStr(vb) {
    // Match the server's "0 0 W H" precision so nothing jitters at reset.
    const r = n => (Math.round(n * 100) / 100);
    return `${r(vb.x)} ${r(vb.y)} ${r(vb.w)} ${r(vb.h)}`;
}

function initGeoMap(svg) {
    if (svg.__geomapInteract) return;
    svg.__geomapInteract = true;

    const base = parseVB(svg.getAttribute('data-vb0')) ||
        parseVB(svg.getAttribute('viewBox'));
    if (!base) return;
    const aspect = base.h / base.w;          // preserved across all zoom
    const minW = base.w / MAX_ZOOM;          // 12x
    const maxW = base.w;                     // 1x (fit)

    const figure = svg.closest('figure') || svg.parentElement;
    if (figure) figure.classList.add('geomap-interactive');

    // View state is split in two so animation never repaints the SVG:
    //   cur       — the logical view the user sees right now
    //   committed — the viewBox actually on the attribute
    // While a gesture or chase is in flight, `cur` is presented by a
    // GPU-composited CSS transform on the svg element (cheap: the browser
    // scales the existing raster); the viewBox — a full ~220-path repaint —
    // is committed exactly once when the view settles. Deep zoom-in shows a
    // moment of raster blur until the settle repaint, the standard map
    // trade.
    let cur = parseVB(svg.getAttribute('viewBox')) || { ...base };
    let committed = { ...cur };
    let tf = { tx: 0, ty: 0, k: 1 }; // the CSS transform currently applied

    // The untransformed layout box in client coords (getBoundingClientRect
    // reflects the active transform, so undo it — we know exactly what we
    // set).
    function layoutRect() {
        const r = svg.getBoundingClientRect();
        return {
            left: r.left - tf.tx,
            top: r.top - tf.ty,
            width: r.width / tf.k,
            height: r.height / tf.k,
        };
    }

    // Present view v via CSS transform relative to the committed viewBox.
    function present(v) {
        cur = v;
        const r = layoutRect();
        if (!r.width || !r.height) return;
        const s0 = Math.min(r.width / committed.w, r.height / committed.h);
        const ox0 = (r.width - committed.w * s0) / 2;
        const oy0 = (r.height - committed.h * s0) / 2;
        const s1 = Math.min(r.width / v.w, r.height / v.h);
        const ox1 = (r.width - v.w * s1) / 2;
        const oy1 = (r.height - v.h * s1) / 2;
        const k = s1 / s0;
        tf = {
            k,
            tx: (ox1 - v.x * s1) - k * (ox0 - committed.x * s0),
            ty: (oy1 - v.y * s1) - k * (oy0 - committed.y * s0),
        };
        svg.style.transform = `translate(${tf.tx}px, ${tf.ty}px) scale(${tf.k})`;
    }

    // Commit view v: one real viewBox write, transform cleared.
    function commit(v) {
        cur = v;
        committed = { ...v };
        tf = { tx: 0, ty: 0, k: 1 };
        svg.style.transform = '';
        // Marks counter-scale via CSS so they keep their on-screen size.
        svg.style.setProperty('--geomap-zoom', (base.w / v.w).toFixed(4));
        svg.setAttribute('viewBox', vbStr(v));
    }

    function clampWidth(w) {
        return Math.max(minW, Math.min(maxW, w));
    }

    // Keep the viewBox inside the original extent so content can't leave the
    // frame; aspect is pinned to the original so meet-fit never letterboxes
    // differently than the server render.
    function clamp(vb) {
        const w = clampWidth(vb.w);
        const h = w * aspect;
        const x = Math.max(base.x, Math.min(base.x + base.w - w, vb.x));
        const y = Math.max(base.y, Math.min(base.y + base.h - h, vb.y));
        return { x, y, w, h };
    }


    // Smoothed move (wheel / buttons / reset). Instant when reduced motion.
    //
    // `pending` is the committed zoom target; all wheel/button math happens
    // in target space (pending ?? cur), never against the animating viewBox.
    // Presentation is a single continuous chase loop: every frame the view
    // eases toward the latest target by a fixed exponential rate. Retargeting
    // just moves the goalpost — the velocity profile never restarts, which is
    // what makes a rapid wheel stream feel like one fluid zoom instead of a
    // pulse per notch. Width is chased in LOG space (constant perceptual
    // zoom rate across scales) and x/y ride the anchor-preserving line
    // toward the target (x is linear in w along that line), so the cursor
    // anchor stays pinned mid-flight too.
    let raf = 0;
    let pending = null;
    let chaseT = 0;
    function chase(now) {
        raf = 0;
        if (!pending) { chaseT = 0; return; }
        const dt = chaseT ? Math.min(64, now - chaseT) : 16;
        chaseT = now;
        const t = pending;
        const a = 1 - Math.exp(-dt / CHASE_TAU_MS);
        let next;
        if (Math.abs(t.w - cur.w) > 1e-9 * base.w) {
            const newW = Math.exp(Math.log(cur.w) + (Math.log(t.w) - Math.log(cur.w)) * a);
            const f = (newW - cur.w) / (t.w - cur.w);
            next = {
                x: cur.x + (t.x - cur.x) * f,
                y: cur.y + (t.y - cur.y) * f,
                w: newW,
                h: newW * aspect,
            };
        } else {
            // Pure translation (same width): plain exponential approach.
            next = { x: cur.x + (t.x - cur.x) * a, y: cur.y + (t.y - cur.y) * a, w: t.w, h: t.h };
        }
        const done = Math.abs(Math.log(next.w / t.w)) < 5e-4 &&
            Math.abs(next.x - t.x) < 5e-4 * next.w &&
            Math.abs(next.y - t.y) < 5e-4 * next.w;
        if (done) {
            commit(t);
            pending = null;
            chaseT = 0;
            figure && figure.classList.remove('geomap-zooming');
            return;
        }
        present(next);
        raf = requestAnimationFrame(chase);
    }
    // commit: snap to the in-flight target instead of abandoning it — a grab
    // right after a wheel burst must land the zoom the user already asked
    // for, then pan from there.
    function cancelTween(commitPending) {
        if (raf) { cancelAnimationFrame(raf); raf = 0; }
        if (commitPending && pending) commit(pending);
        else if (tf.k !== 1 || tf.tx || tf.ty) commit(cur);
        pending = null;
        chaseT = 0;
        figure && figure.classList.remove('geomap-zooming');
    }
    function animateTo(target) {
        target = clamp(target);
        if (reducedMotion()) {
            commit(target);
            pending = null;
            return;
        }
        pending = target;
        // Pointer-events off while the view is in flight: shapes sliding
        // under a stationary cursor otherwise fire enter/leave storms that
        // thrash the tooltip and stutter the zoom.
        figure && figure.classList.add('geomap-zooming');
        if (!raf) raf = requestAnimationFrame(chase);
    }

    // Map a client point to user coords through an arbitrary viewBox,
    // assuming preserveAspectRatio="xMidYMid meet" (what GeoMapSVG emits).
    // getScreenCTM reflects the animating viewBox mid-tween, so target-space
    // math must compute the meet transform itself.
    function clientToUserVB(vb, cx, cy) {
        const r = layoutRect();
        if (!r.width || !r.height) return { x: vb.x + vb.w / 2, y: vb.y + vb.h / 2 };
        const s = Math.min(r.width / vb.w, r.height / vb.h);
        const ox = r.left + (r.width - vb.w * s) / 2;
        const oy = r.top + (r.height - vb.h * s) / 2;
        return { x: vb.x + (cx - ox) / s, y: vb.y + (cy - oy) / s };
    }

    // Zoom the given viewBox so the user point under (cx,cy) stays fixed.
    function zoomTargetAround(from, newW, cx, cy) {
        newW = clampWidth(newW);
        const p = clientToUserVB(from, cx, cy);
        const k = newW / from.w;
        return {
            x: p.x - (p.x - from.x) * k,
            y: p.y - (p.y - from.y) * k,
            w: newW,
            h: newW * aspect,
        };
    }

    // --- Wheel zoom ---------------------------------------------------------
    svg.addEventListener('wheel', ev => {
        ev.preventDefault();
        // Normalize delta units (0 = pixels, 1 = lines, 2 = pages) and cap a
        // single event so trackpad flicks can't teleport the zoom; the
        // exponential keeps each wheel notch a constant zoom ratio (scroll
        // up = in).
        const unit = ev.deltaMode === 1 ? 16 : ev.deltaMode === 2 ? 120 : 1;
        const dy = Math.max(-240, Math.min(240, ev.deltaY * unit));
        const from = pending || cur;
        animateTo(zoomTargetAround(from, from.w * Math.exp(dy * 0.0015), ev.clientX, ev.clientY));
    }, { passive: false });

    // --- Pointer drag pan + pinch zoom -------------------------------------
    const pointers = new Map(); // pointerId -> last client {x,y}
    let panStartVB = null;      // viewBox captured at grab
    let panStartClient = null;  // client point captured at grab
    let panScale = 1;           // user units per client px, captured at grab
    let moved = 0;              // total px moved this gesture
    let pinchStartDist = 0;
    let pinchStartVB = null; // viewBox captured at pinch start

    function pinchMidpoint() {
        const pts = [...pointers.values()];
        return {
            x: (pts[0].x + pts[1].x) / 2,
            y: (pts[0].y + pts[1].y) / 2,
        };
    }
    function pinchDist() {
        const pts = [...pointers.values()];
        return Math.hypot(pts[0].x - pts[1].x, pts[0].y - pts[1].y);
    }

    svg.addEventListener('pointerdown', ev => {
        if (ev.button && ev.button !== 0) return;
        // A grab claims the viewBox: land any in-flight zoom target, then
        // stop the tween so it can't keep writing frames under the drag.
        cancelTween(true);
        pointers.set(ev.pointerId, { x: ev.clientX, y: ev.clientY });
        moved = 0;
        if (pointers.size === 2) {
            // Enter pinch: freeze any pan.
            panStartVB = null;
            pinchStartDist = pinchDist();
            pinchStartVB = { ...cur };
        } else if (pointers.size === 1) {
            const ctm = svg.getScreenCTM();
            panScale = ctm ? 1 / ctm.a : cur.w; // user units per client px
            panStartVB = { ...cur };
            panStartClient = { x: ev.clientX, y: ev.clientY };
        }
    });

    svg.addEventListener('pointermove', ev => {
        if (!pointers.has(ev.pointerId)) return;
        const prev = pointers.get(ev.pointerId);
        moved += Math.hypot(ev.clientX - prev.x, ev.clientY - prev.y);
        pointers.set(ev.pointerId, { x: ev.clientX, y: ev.clientY });

        if (pointers.size === 2 && pinchStartDist > 0) {
            ev.preventDefault();
            const dist = pinchDist();
            if (dist > 0) {
                // Absolute from the gesture-start capture (no per-event
                // compounding): scale by the distance ratio, anchored at the
                // live midpoint mapped through the captured viewBox.
                const mid = pinchMidpoint();
                present(clamp(zoomTargetAround(
                    pinchStartVB, pinchStartVB.w * (pinchStartDist / dist), mid.x, mid.y)));
            }
            return;
        }

        if (pointers.size === 1 && panStartVB) {
            // Only capture (and thus claim the gesture) once it's a real drag,
            // so a plain click still reaches the shape's Datastar handler.
            if (moved > CLICK_SLOP) {
                if (svg.setPointerCapture) {
                    try { svg.setPointerCapture(ev.pointerId); } catch (e) { /* ignore */ }
                }
                figure && figure.classList.add('geomap-dragging');
                ev.preventDefault();
                const dx = (ev.clientX - panStartClient.x) * panScale;
                const dy = (ev.clientY - panStartClient.y) * panScale;
                present(clamp({
                    x: panStartVB.x - dx,
                    y: panStartVB.y - dy,
                    w: panStartVB.w,
                    h: panStartVB.h,
                }));
            }
        }
    });

    function endPointer(ev) {
        if (!pointers.has(ev.pointerId)) return;
        pointers.delete(ev.pointerId);
        if (pointers.size < 2) { pinchStartDist = 0; }
        if (pointers.size === 0) {
            panStartVB = null;
            figure && figure.classList.remove('geomap-dragging');
        } else if (pointers.size === 1) {
            // Dropped from pinch to one finger: re-seat the pan anchor.
            const [[, last]] = pointers;
            const ctm = svg.getScreenCTM();
            panScale = ctm ? 1 / ctm.a : cur.w;
            panStartVB = { ...cur };
            panStartClient = { x: last.x, y: last.y };
            moved = CLICK_SLOP + 1; // continuing a gesture, never a click
        }
    }
    svg.addEventListener('pointerup', endPointer);
    svg.addEventListener('pointercancel', endPointer);

    // Suppress the click that ends a drag so it doesn't trigger a shape's
    // Datastar drill; genuine clicks (moved <= slop) pass straight through.
    svg.addEventListener('click', ev => {
        if (moved > CLICK_SLOP) {
            ev.stopPropagation();
            ev.preventDefault();
            return;
        }
        const shape = ev.target.closest && ev.target.closest('.geomap-shape');
        if (!shape || !figure) return;
        const code = shape.getAttribute('data-region') || '';
        const fill = shape.getAttribute('fill') || '';
        const hasData = fill.indexOf('oklch(') === 0;
        // Name from the pre-built tooltip title, if present.
        let name = '';
        const tip = shape.getAttribute('data-tip');
        if (tip) {
            const doc = document.createElement('div');
            doc.innerHTML = tip;
            const title = doc.querySelector('.chart-tooltip-head span:last-child');
            if (title) name = title.textContent || '';
        }
        figure.dispatchEvent(new CustomEvent('geomap:region', {
            bubbles: true,
            detail: { code, name, hasData },
        }));
        // Drill loading feedback (does not block Datastar's own handler).
        if (shape.hasAttribute('data-on:click') || shape.hasAttribute('data-on-click')) {
            markBusy(shape);
        }
    }, true);

    // --- Double-click reset -------------------------------------------------
    svg.addEventListener('dblclick', ev => {
        ev.preventDefault();
        animateTo({ ...base });
    });

    // --- Overlay controls ---------------------------------------------------
    function buttonZoom(ratio) {
        const from = pending || cur;
        const c = rectCenter();
        animateTo(zoomTargetAround(from, from.w * ratio, c.x, c.y));
    }
    if (figure) buildControls(figure, {
        zoomIn: () => buttonZoom(1 / 1.6),
        zoomOut: () => buttonZoom(1.6),
        reset: () => animateTo({ ...base }),
    });

    function rectCenter() {
        const r = layoutRect();
        return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
    }
}

// markBusy flags a shape + its figure as loading a drill target; clears on
// the next Datastar fetch-finished event or a 3s fallback.
const busyTimers = new WeakMap();
function markBusy(shape) {
    const figure = shape.closest('figure');
    shape.classList.add('geomap-busy');
    if (figure) figure.classList.add('geomap-busy');
    const prev = busyTimers.get(shape);
    if (prev) clearTimeout(prev);
    busyTimers.set(shape, setTimeout(() => clearBusy(shape), BUSY_FALLBACK_MS));
}
function clearBusy(shape) {
    const figure = shape.closest('figure');
    shape.classList.remove('geomap-busy');
    // Only drop the figure flag when no sibling shape is still busy.
    if (figure && !figure.querySelector('.geomap-shape.geomap-busy')) {
        figure.classList.remove('geomap-busy');
    }
    const t = busyTimers.get(shape);
    if (t) { clearTimeout(t); busyTimers.delete(shape); }
}
function clearAllBusy() {
    document.querySelectorAll('.geomap-shape.geomap-busy').forEach(clearBusy);
}
// Best-effort: Datastar dispatches `datastar-fetch` with detail.type of
// 'started'/'finished'/'error'. Clear busy state when a fetch settles.
document.addEventListener('datastar-fetch', ev => {
    const t = ev && ev.detail && ev.detail.type;
    if (t === 'finished' || t === 'error' || t === 'settled') clearAllBusy();
}, true);

// buildControls injects the +/-/reset overlay into the figure once.
function buildControls(figure, handlers) {
    if (figure.querySelector('.geomap-controls')) return;
    const wrap = document.createElement('div');
    wrap.className = 'geomap-controls';
    const mk = (label, aria, fn) => {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'geomap-ctrl-btn';
        b.textContent = label;
        b.setAttribute('aria-label', aria);
        b.addEventListener('click', ev => { ev.stopPropagation(); fn(); });
        // Keep pointerdown from starting a pan on the svg beneath.
        b.addEventListener('pointerdown', ev => ev.stopPropagation());
        return b;
    };
    wrap.appendChild(mk('+', 'Zoom in', handlers.zoomIn));
    wrap.appendChild(mk('−', 'Zoom out', handlers.zoomOut)); // minus sign
    wrap.appendChild(mk('↺', 'Reset view', handlers.reset)); // ↺
    figure.appendChild(wrap);
}

function scan(root) {
    if (!root.querySelectorAll) return;
    root.querySelectorAll('svg[data-geomap-interact]').forEach(initGeoMap);
}

function start() {
    scan(document);
    new MutationObserver(mutations => {
        for (const m of mutations) {
            if (m.target.nodeType === 1 && m.target.matches &&
                m.target.matches('svg[data-geomap-interact]')) {
                initGeoMap(m.target);
            }
            for (const node of m.addedNodes) {
                if (node.nodeType !== 1) continue;
                if (node.matches && node.matches('svg[data-geomap-interact]')) {
                    initGeoMap(node);
                } else {
                    scan(node);
                }
            }
        }
    }).observe(document.body, { childList: true, subtree: true });
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
} else {
    start();
}
