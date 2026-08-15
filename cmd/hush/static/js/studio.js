// goldgirl studio — BYOL. A fixed square of footage; you add images, drag
// them around, rotate and resize, then publish one layout post. Uploads go
// to /studio/piece as they land so the preview is the real file. Publish
// posts the composition to /studio and the server 303s home.

const MAX = 8;
const board = document.getElementById("studio-board");
const files = document.getElementById("studio-files");
const meter = document.getElementById("studio-meter");
const publishBtn = document.getElementById("studio-publish");
const statusEl = document.getElementById("studio-status");
const cert = document.getElementById("studio-2257");

const pieces = [];
let zTop = 1;
let drag = null;

function setStatus(msg) {
  if (statusEl) statusEl.textContent = msg || "";
}

function updateMeter() {
  if (meter) meter.textContent = pieces.length + " / " + MAX;
}

function certified() {
  return !cert || cert.checked;
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
      rot: p.rot,
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
    rot: -10 + (index * 7) % 21,
  };
}

function addPiece(data, aspect) {
  if (pieces.length >= MAX) {
    setStatus("that's the whole square-footage — eight prints.");
    return;
  }
  const geo = placeDefault(pieces.length, aspect);
  const el = document.createElement("div");
  el.className = "studio-piece";
  el.innerHTML =
    '<img alt="" draggable="false"/>' +
    '<button type="button" class="studio-kill" aria-label="remove">×</button>' +
    '<span class="studio-rot" title="drag to rotate"></span>' +
    '<span class="studio-resize" title="drag to resize"></span>';
  el.querySelector("img").src = data.thumb || data.url;
  const p = {
    cf_id: data.cf_id,
    url: data.url,
    x: geo.x,
    y: geo.y,
    w: geo.w,
    h: geo.h,
    rot: geo.rot,
    z: ++zTop,
    el,
  };
  pieces.push(p);
  paint(p);
  board.appendChild(el);
  updateMeter();

  el.querySelector(".studio-kill").addEventListener("click", (e) => {
    e.stopPropagation();
    el.remove();
    const i = pieces.indexOf(p);
    if (i >= 0) pieces.splice(i, 1);
    updateMeter();
  });
}

async function uploadFile(file) {
  if (!certified()) {
    setStatus("confirm the § 2257 certification first.");
    return;
  }
  const fd = new FormData();
  fd.append("image", file, file.name || "piece.jpg");
  fd.append("cert2257", "on");
  const res = await fetch("/studio/piece", { method: "POST", body: fd });
  if (!res.ok) {
    throw new Error(res.status === 400 ? "certify first, or that wasn't an image." : "upload failed.");
  }
  return res.json();
}

function probeAspect(file) {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      const a = img.naturalWidth / (img.naturalHeight || 1);
      URL.revokeObjectURL(url);
      resolve(a);
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      resolve(1);
    };
    img.src = url;
  });
}

if (files) {
  files.addEventListener("change", async () => {
    const list = Array.from(files.files || []);
    files.value = "";
    for (const file of list) {
      if (pieces.length >= MAX) {
        setStatus("that's the whole square-footage — eight prints.");
        break;
      }
      setStatus("adding…");
      try {
        const [data, aspect] = await Promise.all([uploadFile(file), probeAspect(file)]);
        addPiece(data, aspect);
        setStatus("");
      } catch (err) {
        setStatus(err.message || "upload failed.");
      }
    }
  });
}

if (board) {
  board.addEventListener("pointerdown", (e) => {
    const pieceEl = e.target.closest(".studio-piece");
    if (!pieceEl) return;
    const p = pieces.find((x) => x.el === pieceEl);
    if (!p) return;
    if (e.target.closest(".studio-kill")) return;
    e.preventDefault();
    p.z = ++zTop;
    paint(p);
    const mode = e.target.closest(".studio-rot")
      ? "rot"
      : e.target.closest(".studio-resize")
        ? "resize"
        : "move";
    drag = { p, mode, x0: e.clientX, y0: e.clientY, start: { ...p } };
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
      p.rot = drag.start.rot + dx * 80;
    } else {
      p.w = Math.max(0.12, drag.start.w + dx);
      p.h = Math.max(0.12, drag.start.h + dy);
    }
    paint(p);
  });

  const end = () => {
    drag = null;
  };
  board.addEventListener("pointerup", end);
  board.addEventListener("pointercancel", end);
}

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
    setStatus("publishing…");
    const fd = new FormData();
    fd.append("layout", layoutJSON());
    fd.append("cert2257", "on");
    fetch("/studio", { method: "POST", body: fd })
      .then((res) => {
        if (res.redirected) {
          window.location = res.url;
          return;
        }
        if (!res.ok) throw new Error("could not publish.");
        window.location = "/";
      })
      .catch((err) => setStatus(err.message || "could not publish."));
  });
}

updateMeter();
setStatus(statusEl && statusEl.textContent ? statusEl.textContent : "add images. drag them. this square is yours.");
