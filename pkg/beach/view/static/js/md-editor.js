// Markdown editor island: toolbar wraps a textarea; preview/upload hit server Actions.

const ROOT = '.dw-md, #dw-md-editor';
const WAIT = 300;
const CHUNK = 5 * 1024 * 1024; // Cloudflare Stream: min PATCH size except the final chunk

function source(root) {
  return root.querySelector('.dw-md-source') || root.querySelector('textarea');
}

function hdrs(root, extra) {
  const h = Object.assign({}, extra);
  if (root.dataset.csrf) h['X-CSRF-Token'] = root.dataset.csrf;
  return h;
}

function ping(ta) {
  ta.dispatchEvent(new Event('input', { bubbles: true }));
}

function wrap(ta, pre, post) {
  const a = ta.selectionStart, b = ta.selectionEnd;
  const sel = ta.value.slice(a, b);
  ta.value = ta.value.slice(0, a) + pre + sel + post + ta.value.slice(b);
  ta.setSelectionRange(a + pre.length, a + pre.length + sel.length);
  ta.focus();
  ping(ta);
}

function lineBlock(ta) {
  const val = ta.value;
  const s = ta.selectionStart, e = ta.selectionEnd;
  const a = val.lastIndexOf('\n', s - 1) + 1;
  let b = val.indexOf('\n', e);
  if (b < 0) b = val.length;
  if (e > s && val[e - 1] === '\n') b = e - 1;
  return [a, b];
}

function prefixLines(ta, map) {
  const [a, b] = lineBlock(ta);
  const out = ta.value.slice(a, b).split('\n').map(map).join('\n');
  ta.value = ta.value.slice(0, a) + out + ta.value.slice(b);
  ta.setSelectionRange(a, a + out.length);
  ta.focus();
  ping(ta);
}

function insert(ta, text) {
  const a = ta.selectionStart, b = ta.selectionEnd;
  ta.value = ta.value.slice(0, a) + text + ta.value.slice(b);
  const n = a + text.length;
  ta.setSelectionRange(n, n);
  ta.focus();
  ping(ta);
}

function pickFile(root, accept) {
  let inp = root.querySelector('input[data-md-file]');
  if (!inp) {
    inp = document.createElement('input');
    inp.type = 'file';
    inp.hidden = true;
    inp.setAttribute('data-md-file', '');
    root.appendChild(inp);
  }
  inp.accept = accept;
  inp.value = '';
  return new Promise((resolve) => {
    inp.onchange = () => resolve(inp.files && inp.files[0]);
    inp.click();
  });
}

async function cmdImage(root, ta) {
  const url = root.dataset.imageUrl;
  if (!url) return;
  const file = await pickFile(root, 'image/*');
  if (!file) return;
  const body = new FormData();
  body.append('file', file);
  const res = await fetch(url, { method: 'POST', body, headers: hdrs(root) });
  if (!res.ok) return;
  const j = await res.json();
  if (!j.url) return;
  insert(ta, `![${j.alt || ''}](${j.url})`);
}

async function cmdVideo(root, ta) {
  const create = root.dataset.tusUrl;
  if (!create) return;
  const file = await pickFile(root, 'video/*');
  if (!file || !file.size) return;
  const res = await fetch(create, {
    method: 'POST',
    headers: hdrs(root, { 'Content-Type': 'application/json' }),
    body: JSON.stringify({ length: file.size, name: file.name }),
  });
  if (!res.ok) return;
  const { url, uid } = await res.json();
  if (!url || !uid) return;
  // Persist Location including its query string (the upload token); do not rebuild the URL.
  let offset = 0;
  while (offset < file.size) {
    const end = Math.min(offset + CHUNK, file.size);
    const patch = await fetch(url, {
      method: 'PATCH',
      headers: {
        'Tus-Resumable': '1.0.0',
        'Upload-Offset': String(offset),
        'Content-Type': 'application/offset+octet-stream',
      },
      body: file.slice(offset, end),
    });
    if (!patch.ok) return;
    const next = Number(patch.headers.get('Upload-Offset'));
    const prev = offset;
    offset = Number.isFinite(next) && next > prev ? next : end;
    if (offset <= prev) return;
  }
  insert(ta, `:::video {${uid}}\n`);
}

const CMDS = {
  bold: (ta) => wrap(ta, '**', '**'),
  italic: (ta) => wrap(ta, '*', '*'),
  h2: (ta) => prefixLines(ta, (l, i) => (i === 0 ? '## ' + l : l)),
  link: (ta) => {
    const href = prompt('URL');
    if (href == null) return;
    wrap(ta, '[', `](${href})`);
  },
  ul: (ta) => prefixLines(ta, (l) => '- ' + l),
  ol: (ta) => prefixLines(ta, (l, i) => `${i + 1}. ${l}`),
  code: (ta) => {
    const sel = ta.value.slice(ta.selectionStart, ta.selectionEnd);
    if (sel.includes('\n')) wrap(ta, '```\n', '\n```');
    else wrap(ta, '`', '`');
  },
  quote: (ta) => prefixLines(ta, (l) => '> ' + l),
};

async function paint(root, signal) {
  const url = root.dataset.previewUrl;
  const pane = root.querySelector('.dw-md-preview');
  const ta = source(root);
  if (!url || !pane || !ta) return;
  const res = await fetch(url, {
    method: 'POST',
    signal,
    headers: hdrs(root, {
      'Content-Type': 'application/json',
      Accept: 'text/html',
    }),
    body: JSON.stringify({ markdown: ta.value }),
  });
  if (!res.ok) return;
  pane.innerHTML = await res.text();
}

function init(root) {
  if (root.__mdEditor) return;
  root.__mdEditor = true;

  let timer = 0, ac;
  const kick = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      ac?.abort();
      ac = new AbortController();
      paint(root, ac.signal).catch(() => {});
    }, WAIT);
  };

  root.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-md-cmd]');
    if (!btn || !root.contains(btn)) return;
    e.preventDefault();
    const ta = source(root);
    if (!ta) return;
    const cmd = btn.dataset.mdCmd;
    const run = cmd === 'image' ? cmdImage(root, ta)
      : cmd === 'video' ? cmdVideo(root, ta)
        : CMDS[cmd] ? Promise.resolve(CMDS[cmd](ta))
          : null;
    if (run) run.catch(() => {});
  });

  root.addEventListener('input', (e) => {
    if (e.target.matches && e.target.matches('.dw-md-source, textarea')) kick();
  });

  const ta = source(root);
  if (ta && (ta.value || root.dataset.previewUrl)) kick();
}

function scan(node) {
  if (!node) return;
  if (node.nodeType === 1 && node.matches && node.matches(ROOT)) init(node);
  if (node.querySelectorAll) node.querySelectorAll(ROOT).forEach(init);
}

function start() {
  scan(document);
  new MutationObserver((recs) => {
    for (const rec of recs) {
      for (const n of rec.addedNodes) if (n.nodeType === 1) scan(n);
      if (rec.target && rec.target.nodeType === 1) scan(rec.target);
    }
  }).observe(document.body, { childList: true, subtree: true });
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
else start();
