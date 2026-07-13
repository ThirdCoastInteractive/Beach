// Animated orthographic globe. The chart is server-rendered SVG
// (globe.templ); this module takes over figures tagged data-globe with
// a canvas that re-projects the raw country rings every frame, matching
// the SSR look (colors resolved from CSS custom properties at draw
// time, so theme switches stay faithful).
//
// Behavior:
//   - Auto-rotation at rotateSpeed deg/sec; orbit adds a ±8° latitude
//     sine (~40 s period). Paused offscreen (IntersectionObserver) and
//     while the document is hidden. prefers-reduced-motion disables
//     auto-rotation entirely (drag still works).
//   - Pointer drag spins the globe with a short inertia tail (~1 s
//     decay); auto-rotation resumes after ~3 s idle.
//   - Click (not drag) inverse-projects to lon/lat, hit-tests the
//     country rings, and dispatches a bubbling CustomEvent
//     "globe:region" with detail {code, name}.
//   - Hover shows a small themed label with the country name, drawn on
//     the canvas near the cursor (a canvas has no per-shape DOM, so the
//     shared data-tip pattern does not apply).
//
// The only network request is the same-origin static rings JSON named
// in the payload (fetched once, shared across every globe on the page).

const DEG = Math.PI / 180;
const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

// One fetch per rings URL, shared module-wide. A page can embed the rings
// JSON inline (<script type="application/json" id="world-geo-inline">) so a
// static file:// export needs no server; the fetch(src) path stays as the
// fallback when no inline copy is present.
const geoCache = new Map();
function loadGeo(src) {
    if (!geoCache.has(src)) {
        const inline = document.getElementById('world-geo-inline');
        if (inline) {
            try {
                const j = JSON.parse(inline.textContent);
                geoCache.set(src, Promise.resolve(j.countries || []));
                return geoCache.get(src);
            } catch (e) {
                // Malformed inline JSON: fall through to the network fetch.
            }
        }
        geoCache.set(src, fetch(src)
            .then(r => (r.ok ? r.json() : Promise.reject(new Error(r.status))))
            .then(j => j.countries || []));
    }
    return geoCache.get(src);
}

function norm180(x) {
    return ((x % 360) + 540) % 360 - 180;
}

function cssColor(el, name, fallback) {
    const v = getComputedStyle(el).getPropertyValue(name).trim();
    return v || fallback;
}

// Even-odd ray cast in lon/lat space. Longitudes are re-centered on the
// test point so antimeridian-crossing rings behave; edges longer than
// 180° in the shifted frame are skipped (110m-level edge misses are
// acceptable).
function ringCrossings(lon, lat, ring) {
    let crossings = 0;
    for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
        const yi = ring[i][1];
        const yj = ring[j][1];
        if ((yi > lat) === (yj > lat)) continue;
        const xi = norm180(ring[i][0] - lon);
        const xj = norm180(ring[j][0] - lon);
        if (Math.abs(xi - xj) > 180) continue;
        if (xi + (lat - yi) / (yj - yi) * (xj - xi) > 0) crossings++;
    }
    return crossings;
}

function countryAt(countries, lon, lat) {
    for (const c of countries) {
        let crossings = 0;
        for (const ring of c.r) crossings += ringCrossings(lon, lat, ring);
        if (crossings % 2 === 1) return c;
    }
    return null;
}

function initGlobe(fig) {
    if (fig.__globeWired) return;
    const script = document.getElementById(fig.id + '-globe');
    if (!script) return;
    let cfg;
    try {
        cfg = JSON.parse(script.textContent);
    } catch (e) {
        return;
    }
    fig.__globeWired = true;

    // --- state ---------------------------------------------------------
    let countries = null; // rings, once fetched
    let lon0 = cfg.lon0 || 0;
    let baseLat = cfg.lat0 || 0;
    let lat0 = baseLat;
    let orbitT = 0;
    let velLon = 0; // inertia, deg/sec
    let velLat = 0;
    let dragging = false;
    let visible = true;
    let idleUntil = 0; // performance.now() before which auto-rotate is held
    let hover = null; // {x, y, name} in CSS pixels

    const canvas = document.createElement('canvas');
    canvas.className = 'globe-canvas';
    const ctx = canvas.getContext('2d');

    function frame() {
        const w = canvas.clientWidth;
        const h = canvas.clientHeight;
        const s = Math.min(w, h);
        return {
            w, h,
            r: 0.49 * (cfg.zoom || 1) * s,
            cx: (cfg.centerX ?? 0.5) * w,
            cy: (cfg.centerY ?? 0.5) * h,
        };
    }

    // --- projection ------------------------------------------------------
    // Forward: standard orthographic around the (lon0, lat0) subpoint,
    // y flipped for the canvas. Returns cosc (visible when > 0).
    function project(lon, lat, f, sin0, cos0, out) {
        const dl = (lon - lon0) * DEG;
        const phi = lat * DEG;
        const cosPhi = Math.cos(phi);
        const sinPhi = Math.sin(phi);
        const cosDl = Math.cos(dl);
        out.cosc = sin0 * sinPhi + cos0 * cosPhi * cosDl;
        out.x = f.cx + f.r * cosPhi * Math.sin(dl);
        out.y = f.cy - f.r * (cos0 * sinPhi - sin0 * cosPhi * cosDl);
    }

    // Inverse: canvas point to lon/lat, or null outside the disc.
    function unproject(px, py, f) {
        const x = (px - f.cx) / f.r;
        const y = -(py - f.cy) / f.r;
        const rho = Math.hypot(x, y);
        if (rho > 1) return null;
        const c = Math.asin(Math.min(1, rho));
        const sinc = Math.sin(c);
        const cosc = Math.cos(c);
        const phi0 = lat0 * DEG;
        const sin0 = Math.sin(phi0);
        const cos0 = Math.cos(phi0);
        const lat = Math.asin(cosc * sin0 + (rho ? (y * sinc * cos0) / rho : 0)) / DEG;
        const lon = lon0 + Math.atan2(x * sinc, rho * cosc * cos0 - y * sinc * sin0) / DEG;
        return [norm180(lon), lat];
    }

    // --- drawing ---------------------------------------------------------
    const pt = { x: 0, y: 0, cosc: 0 };

    // Trace a lon/lat polyline as visible runs (drop hidden points,
    // restart on re-entry) — the same clip the SSR path builder uses.
    function traceRuns(pts, f, sin0, cos0) {
        let inRun = false;
        for (let i = 0; i < pts.length; i++) {
            project(pts[i][0], pts[i][1], f, sin0, cos0, pt);
            if (pt.cosc <= 1e-9) {
                inRun = false;
                continue;
            }
            if (!inRun) {
                ctx.moveTo(pt.x, pt.y);
                inRun = true;
            } else {
                ctx.lineTo(pt.x, pt.y);
            }
        }
    }

    function drawGraticule(f, sin0, cos0, lineStrong) {
        ctx.beginPath();
        for (let lon = -180; lon < 180; lon += 15) {
            const pts = [];
            for (let lat = -90; lat <= 90; lat += 3) pts.push([lon, lat]);
            traceRuns(pts, f, sin0, cos0);
        }
        for (let lat = -75; lat <= 75; lat += 15) {
            const pts = [];
            for (let lon = -180; lon <= 180; lon += 3) pts.push([lon, lat]);
            traceRuns(pts, f, sin0, cos0);
        }
        ctx.strokeStyle = lineStrong;
        ctx.globalAlpha = 0.25;
        ctx.lineWidth = 0.8;
        ctx.stroke();
        ctx.globalAlpha = 1;
    }

    function regionFill(code, noData) {
        if (cfg.style !== 'themed' || !cfg.regions || !(code in cfg.regions)) return noData;
        const span = (cfg.max - cfg.min) || 1;
        const t = Math.max(0, Math.min(1, (cfg.regions[code] - cfg.min) / span));
        const ramp = cfg.ramp || [];
        if (!ramp.length) return noData;
        return ramp[Math.round(t * (ramp.length - 1))];
    }

    function render() {
        const f = frame();
        if (f.w <= 0 || f.h <= 0) return;
        const dpr = window.devicePixelRatio || 1;
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.clearRect(0, 0, f.w, f.h);

        const panelHover = cssColor(fig, '--color-panel-hover', '#8883');
        const lineStrong = cssColor(fig, '--color-line-strong', '#888');
        const lineSoft = cssColor(fig, '--color-line-soft', '#8886');
        const fgDefault = cssColor(fig, '--color-fg-default', '#ddd');
        const panel = cssColor(fig, '--color-panel', '#222');

        const phi0 = lat0 * DEG;
        const sin0 = Math.sin(phi0);
        const cos0 = Math.cos(phi0);
        const wire = cfg.style === 'wire';

        // Ocean disc.
        ctx.beginPath();
        ctx.arc(f.cx, f.cy, f.r, 0, 2 * Math.PI);
        ctx.fillStyle = panelHover;
        ctx.globalAlpha = wire ? 0.12 : 0.35;
        ctx.fill();
        ctx.globalAlpha = 1;

        if (cfg.graticule || wire) drawGraticule(f, sin0, cos0, lineStrong);

        // Countries.
        if (countries) {
            for (const c of countries) {
                ctx.beginPath();
                for (const ring of c.r) traceRuns(ring, f, sin0, cos0);
                if (wire) {
                    ctx.strokeStyle = fgDefault;
                    ctx.globalAlpha = 0.45;
                    ctx.lineWidth = 1;
                    ctx.stroke();
                } else {
                    ctx.fillStyle = regionFill(c.c, lineSoft);
                    ctx.fill('evenodd');
                    ctx.strokeStyle = fgDefault;
                    ctx.globalAlpha = 0.16;
                    ctx.lineWidth = 0.75;
                    ctx.stroke();
                }
                ctx.globalAlpha = 1;
            }
        }

        // Limb shading (solid style) + limb circle. Fixed translucent
        // stops work over any theme, matching the SSR gradient.
        if (cfg.style === 'solid') {
            const grad = ctx.createRadialGradient(f.cx, f.cy, f.r * 0.72, f.cx, f.cy, f.r);
            grad.addColorStop(0, 'rgba(8, 10, 18, 0)');
            grad.addColorStop(0.78, 'rgba(8, 10, 18, 0.28)');
            grad.addColorStop(1, 'rgba(8, 10, 18, 0.45)');
            ctx.beginPath();
            ctx.arc(f.cx, f.cy, f.r, 0, 2 * Math.PI);
            ctx.fillStyle = grad;
            ctx.fill();
        }
        ctx.beginPath();
        ctx.arc(f.cx, f.cy, f.r, 0, 2 * Math.PI);
        ctx.strokeStyle = lineStrong;
        ctx.globalAlpha = 0.55;
        ctx.lineWidth = 1.5;
        ctx.stroke();
        ctx.globalAlpha = 1;

        // Hover label near the cursor.
        if (hover) {
            ctx.font = '12px system-ui, sans-serif';
            const padX = 8;
            const tw = ctx.measureText(hover.name).width;
            const bw = tw + padX * 2;
            const bh = 22;
            let bx = hover.x + 14;
            let by = hover.y + 14;
            if (bx + bw > f.w - 4) bx = hover.x - bw - 14;
            if (by + bh > f.h - 4) by = hover.y - bh - 14;
            ctx.beginPath();
            ctx.roundRect(bx, by, bw, bh, 5);
            ctx.fillStyle = panel;
            ctx.globalAlpha = 0.92;
            ctx.fill();
            ctx.globalAlpha = 0.6;
            ctx.strokeStyle = lineStrong;
            ctx.lineWidth = 1;
            ctx.stroke();
            ctx.globalAlpha = 1;
            ctx.fillStyle = fgDefault;
            ctx.textBaseline = 'middle';
            ctx.fillText(hover.name, bx + padX, by + bh / 2 + 1);
        }
    }

    // --- animation loop ----------------------------------------------------
    function autoOn() {
        return (cfg.rotateSpeed > 0 || cfg.orbit) &&
            !reducedMotion.matches &&
            visible && !document.hidden && !dragging &&
            performance.now() >= idleUntil;
    }
    function inertiaOn() {
        return !dragging && (Math.abs(velLon) > 0.02 || Math.abs(velLat) > 0.02);
    }

    let rafId = 0;
    let lastT = 0;
    function tick(t) {
        rafId = 0;
        const dt = lastT ? Math.min(0.1, (t - lastT) / 1000) : 0;
        lastT = t;
        let moving = false;
        if (inertiaOn()) {
            lon0 = norm180(lon0 + velLon * dt);
            lat0 = Math.max(-89, Math.min(89, lat0 + velLat * dt));
            const decay = Math.exp(-4 * dt); // ~1 s tail
            velLon *= decay;
            velLat *= decay;
            baseLat = lat0;
            moving = true;
        } else if (autoOn()) {
            lon0 = norm180(lon0 + (cfg.rotateSpeed || 0) * dt);
            if (cfg.orbit) {
                orbitT += dt;
                lat0 = baseLat + 8 * Math.sin((orbitT * 2 * Math.PI) / 40);
            }
            moving = true;
        }
        render();
        if (moving) schedule();
    }
    function schedule() {
        if (rafId) return;
        rafId = requestAnimationFrame(tick);
    }
    function stop() {
        if (rafId) {
            cancelAnimationFrame(rafId);
            rafId = 0;
        }
        // Next tick after a pause must not integrate the paused time
        // (dt is also clamped in tick, belt and suspenders).
        lastT = 0;
    }

    // --- interaction --------------------------------------------------------
    let pdown = false;
    let moved = false;
    let downX = 0;
    let downY = 0;
    let lastX = 0;
    let lastY = 0;
    let lastMoveT = 0;
    let idleTimer = 0;

    function holdAuto() {
        idleUntil = performance.now() + 3000;
        clearTimeout(idleTimer);
        idleTimer = setTimeout(schedule, 3100);
    }

    canvas.addEventListener('pointerdown', ev => {
        pdown = true;
        moved = false;
        dragging = false;
        downX = lastX = ev.clientX;
        downY = lastY = ev.clientY;
        lastMoveT = performance.now();
        velLon = velLat = 0;
        canvas.setPointerCapture(ev.pointerId);
        holdAuto();
    });

    canvas.addEventListener('pointermove', ev => {
        if (pdown) {
            const dx = ev.clientX - lastX;
            const dy = ev.clientY - lastY;
            if (!moved && Math.hypot(ev.clientX - downX, ev.clientY - downY) > 5) {
                moved = true;
                dragging = true;
                fig.classList.add('globe-dragging');
                hover = null;
            }
            if (dragging) {
                const f = frame();
                const degPerPx = 57.2958 / Math.max(1, f.r);
                const dLon = -dx * degPerPx;
                const dLat = dy * degPerPx;
                lon0 = norm180(lon0 + dLon);
                lat0 = Math.max(-89, Math.min(89, lat0 + dLat));
                baseLat = lat0;
                const now = performance.now();
                const dt = Math.max(0.008, (now - lastMoveT) / 1000);
                velLon = 0.7 * velLon + 0.3 * (dLon / dt);
                velLat = 0.7 * velLat + 0.3 * (dLat / dt);
                lastMoveT = now;
                lastX = ev.clientX;
                lastY = ev.clientY;
                holdAuto();
                render();
            }
            return;
        }
        // Hover hit-test, throttled to animation frames.
        if (!countries) return;
        const rect = canvas.getBoundingClientRect();
        const px = ev.clientX - rect.left;
        const py = ev.clientY - rect.top;
        if (canvas.__hoverPending) {
            canvas.__hoverAt = [px, py];
            return;
        }
        canvas.__hoverPending = true;
        canvas.__hoverAt = [px, py];
        requestAnimationFrame(() => {
            canvas.__hoverPending = false;
            const [hx, hy] = canvas.__hoverAt;
            const ll = unproject(hx, hy, frame());
            const c = ll ? countryAt(countries, ll[0], ll[1]) : null;
            const next = c ? { x: hx, y: hy, name: c.n } : null;
            const changed = (hover && !next) || (!hover && next) ||
                (hover && next && (hover.name !== next.name || hover.x !== next.x || hover.y !== next.y));
            hover = next;
            canvas.style.cursor = c ? 'pointer' : '';
            if (changed && !rafId) render();
        });
    });

    canvas.addEventListener('pointerleave', () => {
        if (hover) {
            hover = null;
            canvas.style.cursor = '';
            if (!rafId) render();
        }
    });

    canvas.addEventListener('pointerup', ev => {
        if (!pdown) return;
        pdown = false;
        fig.classList.remove('globe-dragging');
        if (dragging) {
            dragging = false;
            // Old velocity from a stalled drag should not fling.
            if (performance.now() - lastMoveT > 120) velLon = velLat = 0;
            holdAuto();
            schedule(); // inertia tail
            return;
        }
        // A click: resolve the country under the pointer.
        if (!countries) return;
        const rect = canvas.getBoundingClientRect();
        const ll = unproject(ev.clientX - rect.left, ev.clientY - rect.top, frame());
        if (!ll) return;
        const c = countryAt(countries, ll[0], ll[1]);
        if (c) {
            fig.dispatchEvent(new CustomEvent('globe:region', {
                bubbles: true,
                detail: { code: c.c || '', name: c.n },
            }));
        }
    });

    canvas.addEventListener('pointercancel', () => {
        pdown = false;
        dragging = false;
        fig.classList.remove('globe-dragging');
        holdAuto();
    });

    // --- lifecycle ------------------------------------------------------------
    function resize() {
        const dpr = window.devicePixelRatio || 1;
        const w = Math.max(1, Math.round(canvas.clientWidth * dpr));
        const h = Math.max(1, Math.round(canvas.clientHeight * dpr));
        if (canvas.width !== w || canvas.height !== h) {
            canvas.width = w;
            canvas.height = h;
        }
        render();
    }

    new IntersectionObserver(entries => {
        for (const e of entries) {
            visible = e.isIntersecting;
        }
        if (visible) schedule();
        else stop();
    }).observe(fig);

    document.addEventListener('visibilitychange', () => {
        if (document.hidden) stop();
        else schedule();
    });

    reducedMotion.addEventListener?.('change', () => {
        schedule(); // re-evaluates autoOn; renders one frame either way
    });

    loadGeo(cfg.src).then(list => {
        countries = list;
        fig.appendChild(canvas);
        fig.classList.add('globe-live');
        new ResizeObserver(resize).observe(canvas);
        resize();
        schedule();
    }).catch(() => {
        // Fetch failed: leave the SSR frame in place.
        fig.__globeWired = false;
    });
}

function scan(root) {
    for (const fig of root.querySelectorAll('[data-globe]')) initGlobe(fig);
}

function start() {
    scan(document);
    new MutationObserver(mutations => {
        for (const m of mutations) {
            if (m.target.nodeType === 1 && m.target.matches?.('[data-globe]')) {
                initGlobe(m.target);
            }
            for (const node of m.addedNodes) {
                if (node.nodeType !== 1) continue;
                if (node.matches?.('[data-globe]')) initGlobe(node);
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
