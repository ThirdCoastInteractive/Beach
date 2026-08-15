// goldgirl studio — BYOL. Preview is local (blob URLs) so the board
// responds immediately; Cloudflare upload rides in the background and
// stamps cf_id onto each piece before publish. Publish is a real form
// POST (one navigation) so it doesn't fight the page's view transition.

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

// View Transitions abort when a navigation supersedes them — noise, not a bug.
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
    '<button type="button" class="studio-kill" aria-label="remove">×</button>' +
    '<span class="studio-rot" title="drag to rotate"></span>' +
    '<span class="studio-resize" title="drag to resize"></span>';
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

  el.querySelector(".studio-kill").addEventListener("click", (e) => {
    e.stopPropagation();
    el.remove();
    const i = pieces.indexOf(p);
    if (i >= 0) pieces.splice(i, 1);
    updateMeter();
  });
  return p;
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
  if (ct.includes("application/json")) {
    data = await res.json();
  }
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
    img.onload = () => {
      resolve({ aspect: img.naturalWidth / (img.naturalHeight || 1), url });
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      resolve({ aspect: 1, url: "" });
    };
    img.src = url;
  });
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
        if (data.url || data.thumb) {
          const img = p.el.querySelector("img");
          if (img) img.src = data.thumb || data.url;
        }
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
