// goldgirl board — members rearrange the shared pinboard. Click to select,
// drag to move, gold knob to rotate about center, corners to scale, [ ]
// to restack. A drag that moved more than a few pixels does not follow
// the link. Positions POST to /p/{id}/pin on pointerup. CSP-clean.

const board = document.getElementById("hush-board");
if (!board || board.dataset.canPin !== "1") {
  // not a live board
} else {
  const MINW = 0.16;
  const MAXW = 0.36;
  const MAXROT = 26;
  const tools = document.getElementById("board-tools");
  let drag = null;
  let selected = null;
  let zTop = 1;

  const clamp = (n, lo, hi) => Math.max(lo, Math.min(hi, n));

  const pinOf = (el) => {
    const r = board.getBoundingClientRect();
    return {
      x: (parseFloat(el.style.left) || 50) / 100,
      y: (parseFloat(el.style.top) || 50) / 100,
      w: (el.offsetWidth || 1) / r.width,
      rot: parseFloat((el.style.getPropertyValue("--rot") || "0").replace("deg", "")) || 0,
      z: parseInt(el.style.zIndex, 10) || 0,
    };
  };

  const apply = (el, pin) => {
    el.style.left = (pin.x * 100).toFixed(2) + "%";
    el.style.top = (pin.y * 100).toFixed(2) + "%";
    el.style.width = (clamp(pin.w, MINW, MAXW) * 100).toFixed(2) + "%";
    el.style.setProperty("--rot", clamp(pin.rot, -MAXROT, MAXROT).toFixed(1) + "deg");
    if (pin.z != null) el.style.zIndex = String(pin.z);
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

  const select = (el) => {
    if (selected && selected !== el) {
      selected.classList.remove("is-selected");
      delete selected.dataset.armed;
    }
    selected = el;
    if (el) el.classList.add("is-selected");
    if (tools) tools.hidden = !el;
  };

  const printCenter = (el) => {
    const r = board.getBoundingClientRect();
    const pin = pinOf(el);
    return { x: r.left + pin.x * r.width, y: r.top + pin.y * r.height };
  };

  const start = (e, mode) => {
    const el = e.target.closest(".hush-print");
    if (!el || !board.contains(el)) return;
    e.preventDefault();
    select(el);
    const pin = pinOf(el);
    const c = printCenter(el);
    if (mode !== "move") el.dataset.suppressClick = "1";
    drag = {
      el,
      mode,
      moved: false,
      x0: e.clientX,
      y0: e.clientY,
      pin,
      startAngle: Math.atan2(e.clientY - c.y, e.clientX - c.x),
    };
    el.classList.add("hush-print--dragging");
    el.setPointerCapture?.(e.pointerId);
  };

  board.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) return;
    if (e.target.closest(".hush-print-rot")) {
      start(e, "rot");
      return;
    }
    if (e.target.closest(".hush-print-handle")) {
      start(e, "scale");
      return;
    }
    if (e.target.closest(".hush-print")) start(e, "move");
    else select(null);
  });

  board.addEventListener("pointermove", (e) => {
    if (!drag) return;
    const dx = e.clientX - drag.x0;
    const dy = e.clientY - drag.y0;
    if (!drag.moved && dx * dx + dy * dy < 25) return;
    if (!drag.moved && drag.mode === "move") {
      drag.pin.z = ++zTop;
      drag.el.style.zIndex = String(drag.pin.z);
    }
    drag.moved = true;
    const r = board.getBoundingClientRect();
    const pin = { ...drag.pin };
    if (drag.mode === "move") {
      pin.x = drag.pin.x + dx / r.width;
      pin.y = drag.pin.y + dy / r.height;
    } else if (drag.mode === "rot") {
      const c = { x: r.left + drag.pin.x * r.width, y: r.top + drag.pin.y * r.height };
      const ang = Math.atan2(e.clientY - c.y, e.clientX - c.x);
      pin.rot = drag.pin.rot + ((ang - drag.startAngle) * 180) / Math.PI;
    } else {
      const c = { x: r.left + drag.pin.x * r.width, y: r.top + drag.pin.y * r.height };
      const d0 = Math.hypot(drag.x0 - c.x, drag.y0 - c.y) || 1;
      const d1 = Math.hypot(e.clientX - c.x, e.clientY - c.y);
      pin.w = drag.pin.w * (d1 / d0);
    }
    apply(drag.el, pin);
  });

  const end = () => {
    if (!drag) return;
    const { el, moved } = drag;
    el.classList.remove("hush-print--dragging");
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
    if (!el) return;
    if (el.dataset.suppressClick === "1") {
      e.preventDefault();
      delete el.dataset.suppressClick;
      return;
    }
    // first click selects so the toolbar can run; click the same print again to open
    if (el.dataset.armed !== "1") {
      e.preventDefault();
      el.dataset.armed = "1";
    } else {
      delete el.dataset.armed;
    }
  });

  board.addEventListener(
    "wheel",
    (e) => {
      const el = e.target.closest(".hush-print");
      if (!el || !board.contains(el)) return;
      e.preventDefault();
      select(el);
      const pin = pinOf(el);
      pin.w = pin.w * (e.deltaY > 0 ? 0.94 : 1.06);
      apply(el, pin);
      save(el);
    },
    { passive: false },
  );

  if (tools) {
    tools.addEventListener("pointerdown", (e) => e.stopPropagation());
    tools.addEventListener("click", (e) => {
      e.preventDefault();
      e.stopPropagation();
      if (!selected) return;
      const btn = e.target.closest("[data-act]");
      if (!btn) return;
      const pin = pinOf(selected);
      const act = btn.dataset.act;
      if (act === "rot-ccw") pin.rot -= 4;
      else if (act === "rot-cw") pin.rot += 4;
      else if (act === "smaller") pin.w *= 0.9;
      else if (act === "bigger") pin.w *= 1.12;
      else if (act === "back") {
        let min = pin.z;
        for (const el of board.querySelectorAll(".hush-print")) {
          const z = parseInt(el.style.zIndex, 10) || 0;
          if (z < min) min = z;
        }
        pin.z = min - 1;
      } else if (act === "front") {
        pin.z = ++zTop;
      }
      apply(selected, pin);
      save(selected);
    });
  }

  document.addEventListener("keydown", (e) => {
    if (!selected || !board.contains(selected)) return;
    const tag = (e.target && e.target.tagName) || "";
    if (tag === "INPUT" || tag === "TEXTAREA") return;
    const pin = pinOf(selected);
    if (e.key === "[") {
      let min = pin.z;
      for (const el of board.querySelectorAll(".hush-print")) {
        const z = parseInt(el.style.zIndex, 10) || 0;
        if (z < min) min = z;
      }
      pin.z = min - 1;
      apply(selected, pin);
      save(selected);
    } else if (e.key === "]") {
      pin.z = ++zTop;
      apply(selected, pin);
      save(selected);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      pin.w = pin.w * 1.08;
      apply(selected, pin);
      save(selected);
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      pin.w = pin.w * 0.92;
      apply(selected, pin);
      save(selected);
    } else if (e.key === "ArrowLeft") {
      e.preventDefault();
      pin.rot = pin.rot - (e.shiftKey ? 10 : 4);
      apply(selected, pin);
      save(selected);
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      pin.rot = pin.rot + (e.shiftKey ? 10 : 4);
      apply(selected, pin);
      save(selected);
    }
  });

  for (const el of board.querySelectorAll(".hush-print")) {
    const z = parseInt(el.style.zIndex, 10) || 0;
    if (z > zTop) zTop = z;
    if (!el.querySelector(".hush-print-rot")) {
      const pip = document.createElement("span");
      pip.className = "hush-print-rot";
      pip.title = "drag to rotate";
      el.appendChild(pip);
    }
    if (!el.querySelector(".hush-print-handle")) {
      for (const corner of ["nw", "ne", "sw", "se"]) {
        const h = document.createElement("span");
        h.className = "hush-print-handle hush-print-handle--" + corner;
        h.dataset.corner = corner;
        h.title = "drag to scale";
        el.appendChild(h);
      }
    }
  }
}
