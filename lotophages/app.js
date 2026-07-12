// LOTOPHAGES — small rites for a static page.
(() => {
  "use strict";
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;

  // ── rites of the order ──
  const lines = [
    "They ate the lotus, and did not return.",
    "Initiates of the sun ask nothing of the night.",
    "The order keeps the sun; the sun keeps the order.",
    "What the lotus opens, it does not close.",
    "Enter by the flower. Leave by no door.",
    "Forget the world — the order will remember it for you.",
    "We are the keepers of the drowned sun.",
    "Sleep is a discipline. The sun is its master.",
  ];

  const el = document.getElementById("aphorism");
  if (el) {
    let i = 0;
    setInterval(() => {
      i = (i + 1) % lines.length;
      if (reduce) { el.textContent = lines[i]; return; }
      el.classList.add("fade");
      const swap = () => { el.textContent = lines[i]; el.classList.remove("fade"); el.removeEventListener("transitionend", swap); };
      el.addEventListener("transitionend", swap);
      // fallback if transitionend doesn't fire
      setTimeout(() => { if (el.classList.contains("fade")) swap(); }, 1100);
    }, 7200);
  }

  // ── the sun leans toward the cursor ──
  const seal = document.querySelector(".seal");
  if (seal && !reduce && matchMedia("(hover: hover) and (pointer: fine)").matches) {
    let raf = 0;
    addEventListener("pointermove", (e) => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        const dx = (e.clientX / innerWidth - 0.5);
        const dy = (e.clientY / innerHeight - 0.5);
        seal.style.transform = `translate(${dx * 22}px, ${dy * 18}px)`;
      });
    }, { passive: true });
  }

  // ── the summons ──
  const form = document.getElementById("summon");
  const note = document.getElementById("summon-note");
  if (form && note) {
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      const email = /** @type {HTMLInputElement} */ (document.getElementById("email"));
      const ok = email && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value.trim());
      if (!ok) {
        note.textContent = "A name is required.";
        note.classList.add("show");
        email && email.focus();
        return;
      }
      // No backend yet — wire this to your list (Formspree / Buttondown / etc.).
      form.classList.add("done");
      note.textContent = "Your petition is received.";
      note.classList.add("show");
    });
  }
})();
