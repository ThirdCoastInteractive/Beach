// goldgirl studio — click to select, drag to move, gold knob to rotate,
// corners to scale (aspect locked), toolbar for 15°/90°, size, and stack.

const MAX = 8;
const MIN = 0.12;
const MAXS = 0.85;
const board = document.getElementById("studio-board");
const files = document.getElementById("studio-files");
const meter = document.getElementById("studio-meter");
const publishBtn = document.getElementById("studio-publish");
const statusEl = document.getElementById("studio-status");
const cert = document.getElementById("studio-2257");
const tools = document.getElementById("studio-tools");
const degEl = document.getElementById("studio-deg");

const pieces = [];
let zTop = 1;
let drag = null;
let selected = null;

window.addEventListener("unhandledrejection", (e) => {
  if (e.reason && e.reason.name === "AbortError") e.preventDefault();
});

function setStatus(msg) {
  if (statusEl) statusEl.textContent = msg || "";
}

function updateMeter() {
  if (meter) meter.textContent = pieces.length + " / " + MAX;
}

function certified() {
  return !cert || cert.checked;
}

function wrapRot(r) {
  return ((((r + 180) % 360) + 360) % 360) - 180;
}

function layoutJSON() {
  return JSON.stringify({
    pieces: pieces.map((p) => ({
      cf_id: p.cf_id,
      url: p.url,
      x: p.x,
      y: p.y,
      w: p.w,
      h: p.h,
      rot: wrapRot(p.rot),
      z: p.z,
    })),
  });
}

function paint(p) {
  p.el.style.left = (p.x * 100).toFixed(2) + "%";
  p.el.style.top = (p.y * 100).toFixed(2) + "%";
  p.el.style.width = (p.w * 100).toFixed(2) + "%";
  p.el.style.height = (p.h * 100).toFixed(2) + "%";
  p.el.style.transform = "rotate(" + p.rot.toFixed(1) + "deg)";
  p.el.style.zIndex = String(p.z);
  if (p === selected && degEl) degEl.textContent = Math.round(wrapRot(p.rot)) + "°";
}

function select(p) {
  const prev = selected;
  if (prev && prev.el) prev.el.classList.remove("is-selected");
  selected = p;
  if (prev && prev !== p) paint(prev);
  if (p && p.el) p.el.classList.add("is-selected");
  if (p) paint(p);
  if (tools) tools.hidden = !p;
}

function placeDefault(index, aspect) {
  const col = index % 3;
  const row = Math.floor(index / 3);
  let w = 0.38;
  let h = w / (aspect || 1);
  if (h > 0.48) {
    h = 0.48;
    w = h * (aspect || 1);
  }
  return {
    x: 0.08 + col * 0.28 + (index % 2) * 0.03,
    y: 0.08 + row * 0.30,
    w,
    h,
    rot: 0,
  };
}

function addPiece(previewURL, aspect) {
  if (pieces.length >= MAX) {
    setStatus("that's the whole square-footage — eight prints.");
    return null;
  }
  const geo = placeDefault(pieces.length, aspect);
  const el = document.createElement("div");
  el.className = "studio-piece";
  el.innerHTML =
    '<img alt="" draggable="false"/>' +
    '<span class="studio-rot" title="drag to rotate"></span>' +
    '<span class="studio-handle studio-handle--nw" data-corner="nw"></span>' +
    '<span class="studio-handle studio-handle--ne" data-corner="ne"></span>' +
    '<span class="studio-handle studio-handle--sw" data-corner="sw"></span>' +
    '<span class="studio-handle studio-handle--se" data-corner="se"></span>';
  el.querySelector("img").src = previewURL;
  const p = {
    cf_id: "",
    url: previewURL,
    x: geo.x,
    y: geo.y,
    w: geo.w,
    h: geo.h,
    rot: geo.rot,
    z: ++zTop,
    el,
    pending: true,
  };
  pieces.push(p);
  paint(p);
  board.appendChild(el);
  updateMeter();
  select(p);
  return p;
}

function removePiece(p) {
  if (!p) return;
  p.el.remove();
  const i = pieces.indexOf(p);
  if (i >= 0) pieces.splice(i, 1);
  if (selected === p) select(null);
  updateMeter();
}

function fitSize(w, h) {
  const aspect = w / (h || 1);
  if (w > MAXS) { w = MAXS; h = w / aspect; }
  if (h > MAXS) { h = MAXS; w = h * aspect; }
  if (w < MIN) { w = MIN; h = w / aspect; }
  if (h < MIN) { h = MIN; w = h * aspect; }
  return { w, h };
}

function scaleAboutCenter(p, factor) {
  const cx = p.x + p.w / 2;
  const cy = p.y + p.h / 2;
  const next = fitSize(p.w * factor, p.h * factor);
  p.w = next.w;
  p.h = next.h;
  p.x = cx - p.w / 2;
  p.y = cy - p.h / 2;
  paint(p);
}

function rotateBy(p, deg) {
  p.rot = wrapRot(p.rot + deg);
  paint(p);
}

function sendBack(p) {
  let min = p.z;
  for (const q of pieces) if (q.z < min) min = q.z;
  p.z = min - 1;
  paint(p);
}

function bringFront(p) {
  p.z = ++zTop;
  paint(p);
}

async function uploadFile(file) {
  const fd = new FormData();
  fd.append("image", file, file.name || "piece.jpg");
  fd.append("cert2257", "on");
  const res = await fetch("/studio/piece", { method: "POST", body: fd, redirect: "manual" });
  if (res.status === 401 || res.status === 403 || res.type === "opaqueredirect") {
    throw new Error("sign in to add images.");
  }
  let data = {};
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("application/json")) data = await res.json();
  if (!res.ok) {
    throw new Error(data.error || (res.status === 400 ? "certify first, or that wasn't an image." : "upload failed."));
  }
  if (!data.cf_id) throw new Error("upload failed.");
  return data;
}

function probeAspect(file) {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => resolve({ aspect: img.naturalWidth / (img.naturalHeight || 1), url });
    img.onerror = () => {
      URL.revokeObjectURL(url);
      resolve({ aspect: 1, url: "" });
    };
    img.src = url;
  });
}

function pieceCenter(p) {
  const r = board.getBoundingClientRect();
  return {
    x: r.left + (p.x + p.w / 2) * r.width,
    y: r.top + (p.y + p.h / 2) * r.height,
  };
}

if (files) {
  files.addEventListener("change", async () => {
    const list = Array.from(files.files || []);
    files.value = "";
    if (!certified()) {
      setStatus("confirm the § 2257 certification first.");
      return;
    }
    for (const file of list) {
      if (pieces.length >= MAX) {
        setStatus("that's the whole square-footage — eight prints.");
        break;
      }
      setStatus("adding…");
      try {
        const preview = await probeAspect(file);
        const p = addPiece(preview.url || "", preview.aspect);
        if (!p) continue;
        const data = await uploadFile(file);
        p.cf_id = data.cf_id;
        p.url = data.url || data.thumb || p.url;
        p.pending = false;
        const img = p.el.querySelector("img");
        if (img && (data.url || data.thumb)) img.src = data.thumb || data.url;
        setStatus("click a print, then spin, scale, or stack it.");
      } catch (err) {
        setStatus(err.message || "upload failed.");
      }
    }
  });
}

if (board) {
  board.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) return;
    const pieceEl = e.target.closest(".studio-piece");
    if (!pieceEl) {
      select(null);
      return;
    }
    const p = pieces.find((x) => x.el === pieceEl);
    if (!p) return;
    e.preventDefault();
    select(p);
    const handle = e.target.closest(".studio-handle");
    const mode = e.target.closest(".studio-rot")
      ? "rot"
      : handle
        ? "scale"
        : "move";
    const c = pieceCenter(p);
    drag = {
      p,
      mode,
      x0: e.clientX,
      y0: e.clientY,
      start: { x: p.x, y: p.y, w: p.w, h: p.h, rot: p.rot },
      startAngle: Math.atan2(e.clientY - c.y, e.clientX - c.x),
    };
    pieceEl.setPointerCapture?.(e.pointerId);
  });

  board.addEventListener("pointermove", (e) => {
    if (!drag) return;
    const r = board.getBoundingClientRect();
    const dx = (e.clientX - drag.x0) / r.width;
    const dy = (e.clientY - drag.y0) / r.height;
    const p = drag.p;
    if (drag.mode === "move") {
      p.x = drag.start.x + dx;
      p.y = drag.start.y + dy;
    } else if (drag.mode === "rot") {
      const c = pieceCenter({ ...p, x: drag.start.x, y: drag.start.y, w: drag.start.w, h: drag.start.h });
      const ang = Math.atan2(e.clientY - c.y, e.clientX - c.x);
      p.rot = drag.start.rot + ((ang - drag.startAngle) * 180) / Math.PI;
    } else {
      const c = pieceCenter({ ...p, x: drag.start.x, y: drag.start.y, w: drag.start.w, h: drag.start.h });
      const d0 = Math.hypot(drag.x0 - c.x, drag.y0 - c.y) || 1;
      const d1 = Math.hypot(e.clientX - c.x, e.clientY - c.y);
      const next = fitSize(drag.start.w * (d1 / d0), drag.start.h * (d1 / d0));
      p.w = next.w;
      p.h = next.h;
      p.x = drag.start.x + drag.start.w / 2 - p.w / 2;
      p.y = drag.start.y + drag.start.h / 2 - p.h / 2;
    }
    paint(p);
  });

  const endDrag = () => {
    if (drag && drag.mode === "rot") {
      drag.p.rot = wrapRot(drag.p.rot);
      paint(drag.p);
    }
    drag = null;
  };
  board.addEventListener("pointerup", endDrag);
  board.addEventListener("pointercancel", endDrag);

  board.addEventListener(
    "wheel",
    (e) => {
      if (!selected) return;
      e.preventDefault();
      scaleAboutCenter(selected, e.deltaY > 0 ? 0.94 : 1.06);
    },
    { passive: false },
  );
}

if (tools) {
  tools.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-act]");
    if (!btn || !selected) return;
    const act = btn.dataset.act;
    if (act === "rot-ccw") rotateBy(selected, -15);
    else if (act === "rot-cw") rotateBy(selected, 15);
    else if (act === "rot-90") rotateBy(selected, 90);
    else if (act === "smaller") scaleAboutCenter(selected, 0.9);
    else if (act === "bigger") scaleAboutCenter(selected, 1.12);
    else if (act === "back") sendBack(selected);
    else if (act === "front") bringFront(selected);
    else if (act === "kill") removePiece(selected);
  });
}

document.addEventListener("keydown", (e) => {
  if (!selected) return;
  const tag = (e.target && e.target.tagName) || "";
  if (tag === "INPUT" || tag === "TEXTAREA") return;
  if (e.key === "Delete" || e.key === "Backspace") {
    e.preventDefault();
    removePiece(selected);
  } else if (e.key === "ArrowLeft") {
    e.preventDefault();
    rotateBy(selected, e.shiftKey ? -90 : -15);
  } else if (e.key === "ArrowRight") {
    e.preventDefault();
    rotateBy(selected, e.shiftKey ? 90 : 15);
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    scaleAboutCenter(selected, 1.08);
  } else if (e.key === "ArrowDown") {
    e.preventDefault();
    scaleAboutCenter(selected, 0.92);
  } else if (e.key === "[") {
    sendBack(selected);
  } else if (e.key === "]") {
    bringFront(selected);
  }
});

if (publishBtn) {
  publishBtn.addEventListener("click", () => {
    if (!certified()) {
      setStatus("confirm the § 2257 certification first.");
      return;
    }
    if (pieces.length === 0) {
      setStatus("add at least one image.");
      return;
    }
    if (pieces.some((p) => p.pending || !p.cf_id)) {
      setStatus("wait for the images to finish uploading.");
      return;
    }
    setStatus("publishing…");
    const form = document.createElement("form");
    form.method = "POST";
    form.action = "/studio";
    const layout = document.createElement("input");
    layout.type = "hidden";
    layout.name = "layout";
    layout.value = layoutJSON();
    const box = document.createElement("input");
    box.type = "hidden";
    box.name = "cert2257";
    box.value = "on";
    form.append(layout, box);
    document.body.appendChild(form);
    form.submit();
  });
}

updateMeter();
setStatus(statusEl && statusEl.textContent ? statusEl.textContent : "check the box, then add images. this square is yours.");
