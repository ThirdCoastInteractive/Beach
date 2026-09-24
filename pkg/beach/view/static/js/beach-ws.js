// beach-ws: a small wrapper over the native WebSocket for App.Socket routes.
// Binary frames arrive as ArrayBuffer, reconnects back off exponentially, and
// sends queue while (re)connecting. No dependencies; the socket carries
// payloads that are not hypermedia — UI sync stays on SSE.
//
//   import { connect } from '/static/js/beach-ws.js';
//   const ws = connect('/ws/tick', {
//       onFrame(data) { ... },          // ArrayBuffer (binary) or string (text)
//       onStatus(state) { ... },        // 'connecting' | 'open' | 'closed'
//   });
//   ws.send(payload);                   // queued while connecting
//   ws.close();                         // stops reconnecting

export function connect(path, opts = {}) {
    const url = path.startsWith('ws')
        ? path
        : (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + path;
    const minBackoff = opts.minBackoff ?? 250;   // ms
    const maxBackoff = opts.maxBackoff ?? 10000; // ms

    let sock = null;
    let backoff = minBackoff;
    let queue = [];
    let closed = false;
    let timer = null;

    function status(state) {
        if (opts.onStatus) opts.onStatus(state);
    }

    function open() {
        if (closed) return;
        status('connecting');
        sock = new WebSocket(url);
        sock.binaryType = 'arraybuffer';
        sock.onopen = () => {
            backoff = minBackoff;
            status('open');
            for (const p of queue) sock.send(p);
            queue = [];
        };
        sock.onmessage = (e) => {
            if (opts.onFrame) opts.onFrame(e.data);
        };
        sock.onclose = () => {
            sock = null;
            status('closed');
            if (closed) return;
            timer = setTimeout(open, backoff);
            backoff = Math.min(backoff * 2, maxBackoff);
        };
        // onerror always precedes onclose; the close handler owns recovery.
    }

    open();

    return {
        send(payload) {
            if (sock && sock.readyState === WebSocket.OPEN) sock.send(payload);
            else if (!closed) queue.push(payload);
        },
        close() {
            closed = true;
            clearTimeout(timer);
            if (sock) sock.close(1000);
        },
    };
}
