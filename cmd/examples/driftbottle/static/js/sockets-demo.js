// The /sockets demo page: a text echo over the ordered queue and a 60 Hz
// binary tick stream over WriteLatest.
//
// The tick consumer uses WebSocketStream where available (Chromium): pausing
// reads exerts REAL TCP backpressure — a classic `WebSocket` cannot, because
// the browser's network process keeps draining the socket into an unbounded
// task queue no matter how stalled the page is. With reads paused the path
// buffers fill in about a second, the server's writer blocks, and the
// WriteLatest mailbox coalesces: on release the counter JUMPS and "skipped"
// counts the frames the server never had to send. Hold the stall past the
// keepalive window (~30s) and the server rightly declares the consumer dead —
// the reconnect lands on fresh state, which is latest-state-wins by other
// means.
import { connect } from '/static/js/beach-ws.js';

// --- 60 Hz tick -------------------------------------------------------------

const fps = document.getElementById('tick-fps');
const counterEl = document.getElementById('tick-counter');
const receivedEl = document.getElementById('tick-received');
const skippedEl = document.getElementById('tick-skipped');
const statusEl = document.getElementById('tick-status');
const stallBtn = document.getElementById('tick-stall');
const canvas = document.getElementById('tick-graph');
const g = canvas.getContext('2d');

let stalled = false;
let received = 0;
let skipped = 0;
let lastCounter = -1;
let window1s = []; // receive timestamps in the last second
let rates = [];    // per-graph-tick fps history

stallBtn.addEventListener('click', () => {
    stalled = !stalled;
    stallBtn.textContent = stalled ? 'Release consumer' : 'Stall consumer';
});

function status(state) {
    statusEl.textContent = state;
    // A reconnect starts a fresh server-side counter; don't count the rewind
    // as a skip.
    if (state !== 'open') lastCounter = -1;
}

function onTick(data) {
    // Binary frame: uint32 counter, uint64 server unix-nanos, padding.
    const view = data instanceof ArrayBuffer
        ? new DataView(data)
        : new DataView(data.buffer, data.byteOffset, data.byteLength);
    const counter = view.getUint32(0);
    received++;
    if (lastCounter >= 0 && counter > lastCounter + 1) {
        skipped += counter - lastCounter - 1;
    }
    lastCounter = counter;
    window1s.push(performance.now());
    counterEl.textContent = String(counter);
    receivedEl.textContent = String(received);
    skippedEl.textContent = String(skipped);
}

const tickURL = (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws/tick';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

if ('WebSocketStream' in window) {
    // Reconnecting read loop with genuine backpressure: while stalled we
    // simply do not read, and the network stack stops acking for us.
    (async () => {
        let backoff = 250;
        for (;;) {
            try {
                status('connecting');
                const wss = new WebSocketStream(tickURL);
                const { readable } = await wss.opened;
                status('open');
                backoff = 250;
                const reader = readable.getReader();
                for (;;) {
                    while (stalled) await sleep(100); // the stall: reads stop, TCP backs up
                    const { value, done } = await reader.read();
                    if (done) break;
                    if (typeof value !== 'string') onTick(value);
                }
            } catch { /* fall through to reconnect */ }
            status('closed');
            await sleep(backoff);
            backoff = Math.min(backoff * 2, 10000);
        }
    })();
} else {
    // Fallback: a classic WebSocket delivers every frame regardless of page
    // stalls (no receive backpressure exists), so the stall button only slows
    // the display. The coalescing itself is exercised by native clients and
    // the framework test suite.
    stallBtn.disabled = true;
    stallBtn.textContent = 'Stall needs WebSocketStream';
    connect('/ws/tick', {
        onStatus: status,
        onFrame(data) { if (typeof data !== 'string') onTick(data); },
    });
}

// Graph loop: one bar per 250 ms, height = receive rate (fps), full scale 70.
setInterval(() => {
    const now = performance.now();
    window1s = window1s.filter((t) => now - t < 1000);
    const rate = window1s.length;
    fps.textContent = String(rate);
    rates.push(rate);
    if (rates.length > 160) rates.shift();

    const w = canvas.width, h = canvas.height;
    g.clearRect(0, 0, w, h);
    g.fillStyle = getComputedStyle(canvas).color || '#888';
    const barW = w / 160;
    rates.forEach((r, i) => {
        const bh = Math.min(r / 70, 1) * (h - 4);
        g.fillRect(i * barW, h - bh, barW - 1, bh);
    });
}, 250);

// --- echo -------------------------------------------------------------------

const form = document.getElementById('echo-form');
const input = document.getElementById('echo-input');
const log = document.getElementById('echo-log');

const echo = connect('/ws/echo', {
    onFrame(data) {
        if (typeof data !== 'string') return;
        log.textContent += '< ' + data + '\n';
        log.scrollTop = log.scrollHeight;
    },
});

form.addEventListener('submit', (e) => {
    e.preventDefault();
    const msg = input.value.trim();
    if (!msg) return;
    echo.send(msg);
    log.textContent += '> ' + msg + '\n';
    input.value = '';
});
