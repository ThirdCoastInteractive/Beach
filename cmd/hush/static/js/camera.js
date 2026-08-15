// hush camera — the client-side island, now a real shader instrument. Two
// sources feed one WebGPU pipeline: the camera (getUserMedia → per-frame copy
// into a texture) or a picked image file (createImageBitmap → the same
// texture). Nine preset looks are parameter sets + effect flags in ONE WGSL
// fragment shader; six sliders fine-tune the grade live on top of whichever
// look is selected. On capture the *filtered* canvas becomes a JPEG blob and
// posts to the same /upload endpoint the wall's plain upload uses. Everything
// else in hush is SSR; this is the only real JS.
//
// "Capture the raw image, then modify": the shader modifies the live preview,
// so the filtered canvas already IS the modified still — toBlob needs no
// post-step. File mode renders on the same rAF loop as the camera, so animated
// grain and scanlines stay alive even on stills.
//
// No WebGPU? The capture button is disabled, the instrument hides, and the
// page points back to the wall's plain (unfiltered) upload, so a device
// without WebGPU can still post. No webcam but WebGPU present? File mode
// works on its own.

const canvas = document.getElementById("cam-canvas");
const statusEl = document.getElementById("cam-status");
const shootBtn = document.getElementById("cam-shoot");
const openBtn = document.getElementById("cam-open");
const fileIn = document.getElementById("cam-file");
const sourcesEl = document.getElementById("cam-sources");
const rackEl = document.getElementById("cam-rack");
const slidersEl = document.getElementById("cam-sliders");

// the six grade sliders; ids are cam-<name>, uniforms in this order
const SLIDERS = ["brightness", "contrast", "saturation", "temperature", "grain", "vignette"];
const DEFAULTS = { brightness: 0, contrast: 1, saturation: 1, temperature: 0, grain: 0, vignette: 0 };
// effect flags/intensities the presets toggle; sliders never touch these
const FX0 = { mono: 0, crush: 0, blow: 0, lift: 0, bloom: 0, vhs: 0, poster: 0, duotone: 0, negative: 0 };

// the preset rack: each look is a slider set plus effect flags. picking one
// writes the sliders (so you can see — and then bend — what it did) and swaps
// the flags in whole.
const PRESETS = {
  none: { sliders: {}, fx: {} },
  // b/w, crushed blacks, a film of grain
  noir: { sliders: { brightness: -0.02, contrast: 1.35, saturation: 0, grain: 0.15, vignette: 0.3 }, fx: { mono: 1, crush: 0.1 } },
  // blown highlights, the color rinsed out
  bleach: { sliders: { brightness: 0.06, contrast: 1.25, saturation: 0.45, temperature: 0.02, grain: 0.06, vignette: 0.08 }, fx: { blow: 0.6 } },
  // lifted blacks and a warm halation glow
  dream: { sliders: { brightness: 0.05, contrast: 0.9, saturation: 1.15, temperature: 0.04, grain: 0.05, vignette: 0.12 }, fx: { lift: 0.14, bloom: 0.6 } },
  // chromatic split, scanlines, tape noise, a per-line smear
  vhs: { sliders: { contrast: 1.05, saturation: 0.9, temperature: -0.02, grain: 0.18, vignette: 0.15 }, fx: { vhs: 1 } },
  // five levels through an ordered-dither screen
  poster: { sliders: { contrast: 1.1, saturation: 1.05, vignette: 0.1 }, fx: { poster: 5 } },
  // deep red duotone
  wound: { sliders: { contrast: 1.15, grain: 0.08, vignette: 0.3 }, fx: { duotone: 1 } },
  // inverted, a slight cyan cast
  negative: { sliders: { saturation: 0.95, grain: 0.05, vignette: 0.1 }, fx: { negative: 1 } },
  // brutal contrast, heavy animated grain, a closing vignette
  crush: { sliders: { brightness: -0.04, contrast: 2.2, saturation: 0.85, grain: 0.55, vignette: 0.7 }, fx: { crush: 0.18 } },
};

// one fragment shader, one pass: sample (with vhs displacement and aberration
// when on), bloom taps, grade, effects, grain, vignette. every knob is a
// uniform so the pipeline never rebuilds — presets and sliders only write
// this struct.
const WGSL = `
struct U {
  grade : vec4<f32>, // brightness, contrast, saturation, temperature
  look  : vec4<f32>, // grain, vignette, time (s), mono
  fxa   : vec4<f32>, // black crush, highlight blowout, black lift, bloom
  fxb   : vec4<f32>, // vhs, poster levels (0 = off), duotone, negative
};
@group(0) @binding(0) var samp: sampler;
@group(0) @binding(1) var tex: texture_2d<f32>;
@group(0) @binding(2) var<uniform> u: U;

struct VSOut { @builtin(position) pos: vec4<f32>, @location(0) uv: vec2<f32> };

@vertex fn vs(@builtin(vertex_index) i: u32) -> VSOut {
  var corners = array<vec2<f32>, 3>(vec2(-1.0, -1.0), vec2(3.0, -1.0), vec2(-1.0, 3.0));
  let xy = corners[i];
  var out: VSOut;
  out.pos = vec4(xy, 0.0, 1.0);
  out.uv = vec2((xy.x + 1.0) * 0.5, 1.0 - (xy.y + 1.0) * 0.5);
  return out;
}

fn hash(p: vec2<f32>) -> f32 {
  return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453);
}
fn luma(c: vec3<f32>) -> f32 { return dot(c, vec3(0.299, 0.587, 0.114)); }

@fragment fn fs(in: VSOut) -> @location(0) vec4<f32> {
  let t = u.look.z;
  var uv = in.uv;
  var c: vec3<f32>;

  // sample — vhs displaces each scan row a little (re-rolled a few times a
  // second) and lands the color channels slightly apart
  if (u.fxb.x > 0.0) {
    let row = floor(uv.y * 240.0);
    uv.x += (hash(vec2(row, floor(t * 7.0))) - 0.5) * 0.006 * u.fxb.x;
    let split = 0.004 * u.fxb.x;
    c = vec3(
      textureSample(tex, samp, uv + vec2(split, 0.0)).r,
      textureSample(tex, samp, uv).g,
      textureSample(tex, samp, uv - vec2(split, 0.0)).b
    );
  } else {
    c = textureSample(tex, samp, uv).rgb;
  }

  // halation: eight wide taps, keep only what's bright, breathe it back warm
  if (u.fxa.w > 0.0) {
    var glow = vec3(0.0);
    for (var i = 0u; i < 8u; i++) {
      let a = f32(i) * 0.7853982;
      glow += textureSample(tex, samp, in.uv + vec2(cos(a), sin(a)) * 0.014).rgb;
    }
    glow /= 8.0;
    c += max(glow - vec3(0.5), vec3(0.0)) * u.fxa.w * vec3(1.1, 1.0, 0.92);
  }

  // the grade — the six sliders, in the order printed on the page
  c += u.grade.x;                                   // brightness
  c = (c - 0.5) * u.grade.y + 0.5;                  // contrast
  c = mix(vec3(luma(c)), c, u.grade.z);             // saturation
  c += vec3(u.grade.w, 0.0, -u.grade.w);            // temperature

  // effects — all arithmetic, all identity at zero, so they stack freely
  c = clamp((c - vec3(u.fxa.x)) / (1.0 - u.fxa.x), vec3(0.0), vec3(1.0)); // crushed blacks
  c += vec3(smoothstep(0.55, 1.0, luma(c)) * u.fxa.y);                    // blown highlights
  c = c * (1.0 - u.fxa.z) + vec3(u.fxa.z);                                // lifted blacks
  c = mix(c, vec3(luma(c)), u.look.w);                                    // mono
  let duo = mix(vec3(0.04, 0.0, 0.01), vec3(0.95, 0.16, 0.10), pow(luma(c), 1.1));
  c = mix(c, duo, u.fxb.z);                                               // red duotone
  let neg = clamp(vec3(1.0) - c + vec3(-0.06, 0.02, 0.06), vec3(0.0), vec3(1.0));
  c = mix(c, neg, u.fxb.w);                                               // negative, cyan cast

  // posterize through a 4x4 bayer screen, so the bands break into ordered dots
  if (u.fxb.y >= 2.0) {
    var bayer = array<f32, 16>(
      0.0, 8.0, 2.0, 10.0, 12.0, 4.0, 14.0, 6.0,
      3.0, 11.0, 1.0, 9.0, 15.0, 7.0, 13.0, 5.0
    );
    let bi = (u32(in.pos.y) % 4u) * 4u + (u32(in.pos.x) % 4u);
    let d = (bayer[bi] / 16.0 - 0.5) / u.fxb.y;
    c = clamp(floor((c + vec3(d)) * u.fxb.y) / (u.fxb.y - 1.0), vec3(0.0), vec3(1.0));
  }

  // vhs scanlines and tape noise ride on top of the graded image
  if (u.fxb.x > 0.0) {
    let sl = 0.82 + 0.18 * sin(uv.y * 480.0 * 3.14159);
    c *= mix(1.0, sl, 0.55 * u.fxb.x);
    c += vec3((hash(vec2(uv.y * 913.0, t * 17.0)) - 0.5) * 0.05 * u.fxb.x);
    c += vec3((hash(in.pos.xy * 0.7 + vec2(t * 19.0, t * 23.0)) - 0.5) * 0.06 * u.fxb.x);
  }

  // grain re-rolls every frame — alive even on stills — then the vignette closes
  c += vec3((hash(in.pos.xy + vec2(fract(t) * 1741.0, fract(t * 0.61) * 911.0)) - 0.5) * u.look.x * 0.5);
  let v = smoothstep(0.3, 0.95, distance(in.uv, vec2(0.5)));
  c *= 1.0 - v * u.look.y;

  return vec4(clamp(c, vec3(0.0), vec3(1.0)), 1.0);
}`;

const state = { ...DEFAULTS };
let fx = { ...FX0 };

let device, ctx, pipeline, sampler, uniforms;
let srcTex = null, bindGroup = null;
let mode = null; // "camera" | "file" — null until a source is chosen
let video = null, stream = null;
let running = false, pendingCapture = false;

function setStatus(msg) {
  if (statusEl) statusEl.textContent = msg;
}

// ensureTexture (re)creates the one source texture both modes copy into; the
// bind group follows it because the texture view is baked in.
function ensureTexture(w, h) {
  if (srcTex && srcTex.width === w && srcTex.height === h) return;
  if (srcTex) srcTex.destroy();
  srcTex = device.createTexture({
    size: [w, h],
    format: "rgba8unorm",
    usage: GPUTextureUsage.TEXTURE_BINDING | GPUTextureUsage.COPY_DST | GPUTextureUsage.RENDER_ATTACHMENT,
  });
  bindGroup = device.createBindGroup({
    layout: pipeline.getBindGroupLayout(0),
    entries: [
      { binding: 0, resource: sampler },
      { binding: 1, resource: srcTex.createView() },
      { binding: 2, resource: { buffer: uniforms } },
    ],
  });
}

function sizeCanvas(w, h) {
  if (canvas.width !== w) canvas.width = w;
  if (canvas.height !== h) canvas.height = h;
}

// writeUniforms packs the whole instrument — sliders, effect flags, the
// clock — into the one 64-byte struct, every frame.
function writeUniforms(t) {
  device.queue.writeBuffer(uniforms, 0, new Float32Array([
    state.brightness, state.contrast, state.saturation, state.temperature,
    state.grain, state.vignette, t, fx.mono,
    fx.crush, fx.blow, fx.lift, fx.bloom,
    fx.vhs, fx.poster, fx.duotone, fx.negative,
  ]));
}

function render() {
  const encoder = device.createCommandEncoder();
  const pass = encoder.beginRenderPass({
    colorAttachments: [{
      view: ctx.getCurrentTexture().createView(),
      clearValue: { r: 0, g: 0, b: 0, a: 1 },
      loadOp: "clear",
      storeOp: "store",
    }],
  });
  pass.setPipeline(pipeline);
  pass.setBindGroup(0, bindGroup);
  pass.draw(3);
  pass.end();
  device.queue.submit([encoder.finish()]);
}

// the one rAF loop both modes share: camera mode copies the current video
// frame in first; file mode's still is already in the texture, but the loop
// keeps running so time-driven effects (grain, scanlines) stay animated.
function frame(now) {
  if (mode === "camera") {
    if (!video || video.readyState < 2) { requestAnimationFrame(frame); return; }
    const w = video.videoWidth, h = video.videoHeight;
    sizeCanvas(w, h);
    ensureTexture(w, h);
    device.queue.copyExternalImageToTexture({ source: video }, { texture: srcTex }, [w, h]);
  }
  // keep the clock small so f32 hash inputs stay precise across long sessions
  writeUniforms((now / 1000) % 3600);
  render();
  if (pendingCapture) {
    pendingCapture = false;
    // toBlob must read the canvas in the same task as the render, before the
    // frame is presented away; the loop keeps going so a refused capture
    // refused upload leaves a live preview behind
    capture();
  }
  requestAnimationFrame(frame);
}

function startLoop() {
  if (running) return;
  running = true;
  requestAnimationFrame(frame);
}

function applyPreset(name) {
  const p = PRESETS[name] || PRESETS.none;
  Object.assign(state, DEFAULTS, p.sliders);
  fx = { ...FX0, ...p.fx };
  for (const k of SLIDERS) {
    const el = document.getElementById("cam-" + k);
    if (el) el.value = String(state[k]);
  }
  if (rackEl) {
    for (const btn of rackEl.querySelectorAll("[data-preset]")) {
      btn.setAttribute("aria-pressed", btn.dataset.preset === name ? "true" : "false");
    }
  }
}

async function openCamera() {
  let s;
  try {
    s = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "user" }, audio: false });
  } catch (err) {
    setStatus("couldn't open the camera: " + err.message + " — run a photo through it instead.");
    return;
  }
  stream = s;
  video = document.createElement("video");
  video.srcObject = stream;
  video.muted = true;
  video.playsInline = true;
  await video.play();
  mode = "camera";
  setStatus("pick a look, bend the sliders, capture.");
  startLoop();
}

async function pickFile(file) {
  let bm;
  try {
    bm = await createImageBitmap(file);
  } catch (err) {
    setStatus("couldn't read that image: " + err.message);
    return;
  }
  // cap the long edge so the captured jpeg stays inside the upload's 8 MiB
  const long = Math.max(bm.width, bm.height);
  if (long > 1600) {
    const s = 1600 / long;
    const scaled = await createImageBitmap(bm, {
      resizeWidth: Math.round(bm.width * s),
      resizeHeight: Math.round(bm.height * s),
      resizeQuality: "high",
    });
    bm.close();
    bm = scaled;
  }
  // a picked photo replaces the camera; let the lens go dark
  if (stream) { stream.getTracks().forEach((t) => t.stop()); stream = null; video = null; }
  mode = "file";
  sizeCanvas(bm.width, bm.height);
  ensureTexture(bm.width, bm.height);
  device.queue.copyExternalImageToTexture({ source: bm }, { texture: srcTex }, [bm.width, bm.height]);
  bm.close();
  setStatus("pick a look, bend the sliders, capture.");
  startLoop();
}

function capture() {
  setStatus("posting…");
  canvas.toBlob((blob) => {
    if (!blob) { setStatus("capture failed — try again."); return; }
    const fd = new FormData();
    fd.append("image", blob, "capture.jpg");
    fetch("/upload", { method: "POST", body: fd })
      .then(() => {
        if (stream) stream.getTracks().forEach((t) => t.stop());
        window.location = "/";
      })
      .catch((err) => setStatus("upload failed: " + err.message));
  }, "image/jpeg", 0.9);
}

async function main() {
  const fail = (msg) => {
    setStatus(msg);
    if (shootBtn) shootBtn.disabled = true;
    // no pipeline means no instrument — hide the controls that would lie
    for (const el of [sourcesEl, rackEl, slidersEl]) if (el) el.hidden = true;
  };
  if (!navigator.gpu) {
    fail("webgpu isn't available in this browser — use “upload image” on the wall instead.");
    return;
  }
  const adapter = await navigator.gpu.requestAdapter();
  if (!adapter) {
    fail("webgpu has no adapter here — use “upload image” on the wall instead.");
    return;
  }
  device = await adapter.requestDevice();
  ctx = canvas.getContext("webgpu");
  const format = navigator.gpu.getPreferredCanvasFormat();
  ctx.configure({ device, format, alphaMode: "opaque" });

  sampler = device.createSampler({ magFilter: "linear", minFilter: "linear" });
  uniforms = device.createBuffer({ size: 64, usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST });

  const module = device.createShaderModule({ code: WGSL });
  pipeline = device.createRenderPipeline({
    layout: "auto",
    vertex: { module, entryPoint: "vs" },
    fragment: { module, entryPoint: "fs", targets: [{ format }] },
    primitive: { topology: "triangle-list" },
  });

  if (openBtn) openBtn.addEventListener("click", openCamera);
  if (fileIn) fileIn.addEventListener("change", () => {
    if (fileIn.files && fileIn.files[0]) pickFile(fileIn.files[0]);
  });
  if (rackEl) rackEl.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-preset]");
    if (btn) applyPreset(btn.dataset.preset);
  });
  for (const k of SLIDERS) {
    const el = document.getElementById("cam-" + k);
    if (el) el.addEventListener("input", () => { state[k] = parseFloat(el.value); });
  }
  if (shootBtn) shootBtn.addEventListener("click", () => {
    if (!mode) { setStatus("open the camera or pick a photo first."); return; }
    pendingCapture = true;
  });

  applyPreset("none");
  setStatus("open the camera, or run a photo through it.");
}

main();
