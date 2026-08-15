// goldgirl board — members rearrange the shared pinboard. Drag a print to
// move it, drag the pip to rotate, wheel while hovering to scale. A drag
// that moved more than a few pixels does not follow the link. Positions
// POST to /p/{id}/pin on pointerup. CSP-clean: no inline handlers.

const board = document.getElementById("hush-board");
if (!board || board.dataset.canPin !== "1") {
  // not a live board
} else {
  let drag = null;

  const pinOf = (el) => {
    const r = board.getBoundingClientRect();
    return {
      x: (parseFloat(el.style.left) || 50) / 100,
      y: (parseFloat(el.style.top) || 50) / 100,
      w: (el.offsetWidth || 1) / r.width,
      rot: parseFloat((el.style.getPropertyValue("--rot") || "0").replace("deg", "")) || 0,
    };
  };

  const apply = (el, pin) => {
    el.style.left = (pin.x * 100).toFixed(2) + "%";
    el.style.top = (pin.y * 100).toFixed(2) + "%";
    el.style.width = (pin.w * 100).toFixed(2) + "%";
    el.style.setProperty("--rot", pin.rot.toFixed(1) + "deg");
  };

  const save = (el) => {
    const id = el.dataset.id;
    if (!id) return;
    const pin = pinOf(el);
    fetch("/p/" + id + "/pin", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(pin),
    }).catch(() => {});
  };

  const start = (e, mode) => {
    const el = e.target.closest(".hush-print");
    if (!el || !board.contains(el)) return;
    e.preventDefault();
    const pin = pinOf(el);
    drag = {
      el,
      mode,
      moved: false,
      x0: e.clientX,
      y0: e.clientY,
      pin,
      z: el.style.zIndex,
    };
    el.classList.add("hush-print--dragging");
    el.style.zIndex = "9999";
    el.setPointerCapture?.(e.pointerId);
  };

  board.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) return;
    if (e.target.closest(".hush-print-rot")) {
      start(e, "rot");
      return;
    }
    if (e.target.closest(".hush-print")) start(e, "move");
  });

  board.addEventListener("pointermove", (e) => {
    if (!drag) return;
    const dx = e.clientX - drag.x0;
    const dy = e.clientY - drag.y0;
    if (!drag.moved && dx * dx + dy * dy < 25) return;
    drag.moved = true;
    const r = board.getBoundingClientRect();
    const pin = { ...drag.pin };
    if (drag.mode === "move") {
      pin.x = drag.pin.x + dx / r.width;
      pin.y = drag.pin.y + dy / r.height;
    } else {
      pin.rot = drag.pin.rot + dx * 0.4;
    }
    apply(drag.el, pin);
  });

  const end = (e) => {
    if (!drag) return;
    const { el, moved, z } = drag;
    el.classList.remove("hush-print--dragging");
    el.style.zIndex = z;
    if (moved) {
      save(el);
      el.dataset.suppressClick = "1";
    }
    drag = null;
  };

  board.addEventListener("pointerup", end);
  board.addEventListener("pointercancel", end);

  board.addEventListener("click", (e) => {
    const el = e.target.closest(".hush-print");
    if (el && el.dataset.suppressClick === "1") {
      e.preventDefault();
      delete el.dataset.suppressClick;
    }
  });

  board.addEventListener(
    "wheel",
    (e) => {
      const el = e.target.closest(".hush-print");
      if (!el || !board.contains(el)) return;
      e.preventDefault();
      const pin = pinOf(el);
      pin.w = pin.w * (e.deltaY > 0 ? 0.94 : 1.06);
      apply(el, pin);
      save(el);
    },
    { passive: false },
  );

  // rotation pip — one per print, so we don't need it in the SSR
  for (const el of board.querySelectorAll(".hush-print")) {
    if (el.querySelector(".hush-print-rot")) continue;
    const pip = document.createElement("span");
    pip.className = "hush-print-rot";
    pip.title = "drag to rotate";
    el.appendChild(pip);
  }
}
