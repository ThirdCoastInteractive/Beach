// muskie.js — the shared-tank client island.
//
// The Muskie lives in a glass tank (server-authoritative — internal/pond) and
// visitors are avatars (polyhedron body + a head with a face) who walk the floor
// around it. Controls are third-person: the camera orbits behind you (drag to
// look, scroll to zoom), WASD moves camera-relative, and clicking the floor walks
// you there (the camera eases in behind). The world arrives as SSE frames:
//   hot   — the Muskie + stats + quote (interpolated, render-behind = smooth)
//   cold  — tank size, count, the sampled crowd (with appearance), food, queue
//   moves — the crowd's floor positions (eased toward)
//   self  — your own food, position, appearance, queue slot
// Only the visitor the Muskie speaks to hears the klattsch voice.

import * as THREE from '/static/js/vendor/three/three.module.js';
import { GLTFLoader } from '/static/js/vendor/three/GLTFLoader.js';
import { compileString } from '/static/js/vendor/klattsch/engine/index.js';
import { makeAvatar, applyAvatar } from '/static/js/avatar.js';
import { SHAPES, BODY_PALETTE, SKIN_PALETTE } from '/static/js/shapes.js';

const MODEL_URL = '/static/models/muskie.glb';
const WORKLET_URL = '/static/js/vendor/klattsch/formant-worklet.js';

const TARGET_LEN = 2.4;
const JAW_OPEN = -0.55;
const BASE_W = 4.5, BASE_D = 2.2, WATER_TOP = 1.5, WATER_BOT = -1.5;
const FLOOR_Y = -3.4;
const RING_GAP = 2.2, ROOM_GAP = 4.0; // mirror pond geometry
const HOT_DELAY = 0.12; // seconds of render-behind interpolation for the Muskie
const MOVE_SPEED = 6.0; // avatar walk speed (world units/sec)

const mySid = document.body.dataset.sid || '';

// --- DOM -------------------------------------------------------------------

const canvas = document.getElementById('muskie-canvas');
const stage = document.getElementById('muskie-stage');
const loading = document.getElementById('muskie-loading');
const askBtn = document.getElementById('muskie-ask');
const skipBtn = document.getElementById('muskie-skip');
const customizeBtn = document.getElementById('muskie-customize');
const quoteText = document.getElementById('muskie-quote-text');
const quoteAttr = document.getElementById('muskie-quote-attr');
const foodHud = document.getElementById('am-food');
const queueHud = document.getElementById('am-queue');
const moodHud = document.getElementById('am-mood');
const countHud = document.getElementById('am-count');

// --- three.js scene --------------------------------------------------------

const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, powerPreference: 'high-performance' });
renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));

const scene = new THREE.Scene();
scene.background = new THREE.Color(0x2a121a);
scene.fog = new THREE.FogExp2(0x2a121a, 0.012);

const camera = new THREE.PerspectiveCamera(50, 1, 0.1, 200);

scene.add(new THREE.HemisphereLight(0xbfe8ff, 0x201018, 1.0));
const sun = new THREE.DirectionalLight(0xfff4e0, 1.4);
sun.position.set(3, 9, 5);
scene.add(sun);
const fill = new THREE.DirectionalLight(0x6fb0c8, 0.6);
fill.position.set(-5, 3, -4);
scene.add(fill);

const floor = new THREE.Mesh(
  new THREE.CircleGeometry(60, 64),
  new THREE.MeshStandardMaterial({ color: 0x3a2630, roughness: 1 }),
);
floor.rotation.x = -Math.PI / 2;
floor.position.y = FLOOR_Y;
scene.add(floor);

const tankGroup = new THREE.Group();
scene.add(tankGroup);
buildTank(tankGroup);

function buildTank(g) {
  const standH = WATER_BOT - FLOOR_Y;
  const stand = new THREE.Mesh(
    new THREE.BoxGeometry(2 * BASE_W + 0.6, standH, 2 * BASE_D + 0.6),
    new THREE.MeshStandardMaterial({ color: 0x241a12, roughness: 0.9 }),
  );
  stand.position.y = FLOOR_Y + standH / 2;
  g.add(stand);
  const wH = WATER_TOP - WATER_BOT;
  const water = new THREE.Mesh(
    new THREE.BoxGeometry(2 * BASE_W, wH, 2 * BASE_D),
    new THREE.MeshStandardMaterial({ color: 0x1f7f8e, transparent: true, opacity: 0.32, roughness: 0.25, metalness: 0.1, depthWrite: false }),
  );
  water.position.y = (WATER_TOP + WATER_BOT) / 2;
  g.add(water);
  const gW = 2 * BASE_W + 0.2, gH = (WATER_TOP + 0.25) - WATER_BOT, gD = 2 * BASE_D + 0.2;
  const glassGeo = new THREE.BoxGeometry(gW, gH, gD);
  const edges = new THREE.LineSegments(
    new THREE.EdgesGeometry(glassGeo),
    new THREE.LineBasicMaterial({ color: 0x9fd6ea, transparent: true, opacity: 0.6 }),
  );
  edges.position.y = WATER_BOT + gH / 2;
  g.add(edges);
}

let modelRoot = new THREE.Group();
scene.add(modelRoot);
let model = null, mixer = null, jaw = null;

const zzz = makeLabel('z  z  z', '#cfe8ff');
zzz.visible = false;
scene.add(zzz);

function sizeToStage() {
  const w = stage.clientWidth || 480;
  const h = stage.clientHeight || 360;
  renderer.setSize(w, h, false);
  camera.aspect = w / h;
  camera.updateProjectionMatrix();
}
new ResizeObserver(sizeToStage).observe(stage);
sizeToStage();

// --- Muskie model ----------------------------------------------------------

new GLTFLoader().load(MODEL_URL, (gltf) => installModel(gltf.scene, gltf.animations), undefined, () => {});

function installModel(obj, animations) {
  const box = new THREE.Box3().setFromObject(obj);
  const size = box.getSize(new THREE.Vector3());
  const center = box.getCenter(new THREE.Vector3());
  const scale = TARGET_LEN / (Math.max(size.x, size.y, size.z) || 1);
  obj.scale.setScalar(scale);
  obj.position.sub(center.multiplyScalar(scale));
  modelRoot.add(obj);
  model = obj;
  jaw = obj.getObjectByName('jaw');
  if (animations && animations.length) {
    mixer = new THREE.AnimationMixer(obj);
    for (const name of ['Swim', 'EyesLookAround']) {
      const clip = THREE.AnimationClip.findByName(animations, name);
      if (clip) mixer.clipAction(clip).play();
    }
  }
  if (loading) loading.hidden = true;
}

function setMouth(amount) { if (jaw) jaw.rotation.x = amount * JAW_OPEN; }

// --- labels ----------------------------------------------------------------

function makeLabel(text, color) {
  const c = document.createElement('canvas');
  const ctx = c.getContext('2d');
  ctx.font = '28px Georgia, serif';
  const pad = 14;
  c.width = Math.ceil(ctx.measureText(text).width) + pad * 2;
  c.height = 44;
  ctx.font = '28px Georgia, serif';
  ctx.fillStyle = 'rgba(8,18,18,0.65)';
  ctx.fillRect(0, 0, c.width, c.height);
  ctx.fillStyle = color || '#f4efe2';
  ctx.textBaseline = 'middle';
  ctx.fillText(text, pad, c.height / 2 + 1);
  const tex = new THREE.CanvasTexture(c);
  tex.colorSpace = THREE.SRGBColorSpace;
  const spr = new THREE.Sprite(new THREE.SpriteMaterial({ map: tex, transparent: true, depthWrite: false }));
  spr.scale.set((c.width / c.height) * 0.5, 0.5, 1);
  return spr;
}

// --- crowd (other visitors) + own avatar -----------------------------------

const crowd = new Map(); // short -> { group, tx, tz }
const crowdGroup = new THREE.Group();
scene.add(crowdGroup);

let meGroup = null; // own avatar (with label)
const mePos = new THREE.Vector3(0, FLOOR_Y, 12); // predicted; seeded by first self frame
let mePlaced = false;

function reconcileCrowd(others) {
  const seen = new Set();
  for (const u of others) {
    if (u.id === mySid) continue;
    seen.add(u.i);
    let c = crowd.get(u.i);
    if (!c) {
      const g = makeAvatar(u.shape, u.bc, u.sk, u.i);
      g.position.set(u.x, FLOOR_Y, u.z);
      crowdGroup.add(g);
      c = { group: g, tx: u.x, tz: u.z };
      crowd.set(u.i, c);
    } else {
      applyAvatar(c.group, u.shape, u.bc, u.sk, u.i);
      c.tx = u.x;
      c.tz = u.z;
    }
  }
  for (const [short, c] of crowd) {
    if (!seen.has(short)) { crowdGroup.remove(c.group); crowd.delete(short); }
  }
}

function applyMoves(arr) {
  for (const [short, x, z] of arr) {
    const c = crowd.get(short);
    if (c) { c.tx = x; c.tz = z; }
  }
}

function ensureMe(self) {
  if (!meGroup) {
    meGroup = makeAvatar(self.shape, self.bc, self.sk, self.i);
    const label = makeLabel((self.n || 'You') + ' (you)', '#ffe6a6');
    label.position.y = 1.7;
    meGroup.add(label);
    meGroup.userData.label = label;
    scene.add(meGroup);
  } else {
    applyAvatar(meGroup, self.shape, self.bc, self.sk, self.i);
  }
  if (!mePlaced) {
    mePos.set(self.x, FLOOR_Y, self.z);
    mePlaced = true;
    // Start the camera beyond you on your radial from the tank, so you look past
    // yourself toward the Muskie.
    if (self.x || self.z) camTheta = Math.atan2(self.z, self.x);
  }
}

// --- food pellets ----------------------------------------------------------

const pellets = new Map();
const foodGroup = new THREE.Group();
scene.add(foodGroup);

function reconcileFood(food) {
  const seen = new Set();
  for (const f of food) {
    seen.add(f.id);
    let m = pellets.get(f.id);
    if (!m) {
      m = new THREE.Mesh(new THREE.SphereGeometry(0.1, 12, 12),
        new THREE.MeshStandardMaterial({ color: 0x9a6b3a, emissive: 0x3a2410, roughness: 0.7 }));
      pellets.set(f.id, m);
      foodGroup.add(m);
    }
    m.position.set(f.x, f.y, f.z);
  }
  for (const [id, m] of pellets) {
    if (!seen.has(id)) { foodGroup.remove(m); pellets.delete(id); }
  }
}

// --- audio / voice ---------------------------------------------------------

let audioCtx = null, synthNode = null, analyser = null, workletReady = null;
const ampData = new Float32Array(256);

async function ensureAudio() {
  if (!audioCtx) {
    audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    workletReady = audioCtx.audioWorklet.addModule(WORKLET_URL).then(() => {
      synthNode = new AudioWorkletNode(audioCtx, 'formant-processor', { outputChannelCount: [1] });
      const gain = audioCtx.createGain();
      gain.gain.value = 0.5;
      analyser = audioCtx.createAnalyser();
      analyser.fftSize = 512;
      synthNode.connect(gain).connect(analyser).connect(audioCtx.destination);
    });
  }
  if (audioCtx.state === 'suspended') await audioCtx.resume();
  return workletReady;
}

// VOICES maps an oracle-chosen voice name to a klattsch directive prefix that's
// prepended to the phoneme string (b=baseF0 Hz, r=rate ms/phoneme, s=formant
// scale, g=effort, h=aspiration, v=vibrato depth, w=vibrato rate, m=tremolo
// depth, n=tremolo rate, t=spectral tilt). Names MUST match oracle.Voices; an
// unknown/empty voice uses the Muskie's default (engine defaults).
const VOICES = {
  deep: 'b88 s1.1 r122 g0.65',
  booming: 'b70 s1.2 r135 g0.9 t0.2',
  ancient: 'b92 r150 h0.35 m0.25 n4 g0.4',
  sly: 'b140 r96 s0.97 g0.4',
  manic: 'b182 r82 v9 w7 g0.7',
  doom: 'b60 s1.25 r160 g0.85',
  whisper: 'b104 r122 h0.6 g0.2',
  shrill: 'b220 s0.85 r92 t0.3',
};

let speakingUntil = 0;
async function speak(phonemes, voice) {
  await ensureAudio();
  if (!synthNode || !phonemes) return;
  const prefix = voice && VOICES[voice] ? VOICES[voice] + ' ' : '';
  const { schedule, totalMs } = compileString(prefix + phonemes);
  synthNode.port.postMessage({ type: 'schedule', schedule });
  speakingUntil = performance.now() + totalMs;
}

// --- commands --------------------------------------------------------------

function post(url, body) {
  return fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
}

// askMessage reads (and clears) the optional message a logged-in visitor types
// for the Muskie to react to. Empty for anon/no-input asks.
const msgInput = document.getElementById('muskie-msg');
function askMessage() {
  if (!msgInput) return '';
  const v = msgInput.value.trim();
  msgInput.value = '';
  return v;
}

if (askBtn) askBtn.addEventListener('click', () => { ensureAudio(); post('/pond/ask', { skip: false, msg: askMessage() }); });
if (skipBtn) skipBtn.addEventListener('click', () => { ensureAudio(); post('/pond/ask', { skip: true, msg: askMessage() }); });
if (customizeBtn) customizeBtn.addEventListener('click', () => { const d = document.getElementById('customize-modal'); if (d) d.showModal(); });

// --- chat ------------------------------------------------------------------

const chatLog = document.getElementById('chat-log');
const chatForm = document.getElementById('chat-form');
const chatInput = document.getElementById('chat-input');

function appendChatLine(name, body) {
  if (!chatLog) return;
  const line = document.createElement('div');
  line.className = 'am-chat-line';
  const who = document.createElement('span');
  who.className = 'am-chat-who';
  who.textContent = name + ': ';
  line.append(who, document.createTextNode(body)); // textNode = XSS-safe
  chatLog.append(line);
  while (chatLog.children.length > 80) chatLog.removeChild(chatLog.firstChild);
  chatLog.scrollTop = chatLog.scrollHeight;
}

function setChatLog(msgs) {
  if (!chatLog) return;
  chatLog.innerHTML = '';
  for (const m of msgs) appendChatLine(m.n, m.b);
}

if (chatForm) {
  chatForm.addEventListener('submit', (e) => {
    e.preventDefault();
    const t = chatInput.value.trim();
    if (!t) return;
    post('/chat', { text: t });
    chatInput.value = '';
  });
}

// --- follow-cam + input ----------------------------------------------------

let camTheta = Math.PI / 2 + 0.4;
let camPhi = 1.18; // a flatter, more behind-the-shoulder pitch than top-down
let camRadius = 10;
let autoTrail = null; // desired camTheta to ease behind you while click-walking
const camTarget = new THREE.Vector3(0, FLOOR_Y + 1.1, 0);

// trailTheta is the camTheta that places the camera directly behind a heading.
function trailTheta(heading) { return Math.atan2(-Math.cos(heading), -Math.sin(heading)); }

let dragging = false, moved = 0, lastX = 0, lastY = 0;
const keys = new Set();
let walkTarget = null; // click-to-move destination (THREE.Vector3 on floor) or null

if (canvas) {
  canvas.addEventListener('pointerdown', (e) => {
    dragging = true; moved = 0; lastX = e.clientX; lastY = e.clientY;
    try { canvas.setPointerCapture(e.pointerId); } catch { /* ignore */ }
  });
  canvas.addEventListener('pointermove', (e) => {
    if (!dragging) return;
    const dx = e.clientX - lastX, dy = e.clientY - lastY;
    lastX = e.clientX; lastY = e.clientY;
    moved += Math.abs(dx) + Math.abs(dy);
    camTheta -= dx * 0.006;
    camPhi = clamp(camPhi - dy * 0.006, 0.35, 1.45);
  });
  canvas.addEventListener('pointerup', (e) => { dragging = false; if (moved < 6) onTap(e); });
  canvas.addEventListener('wheel', (e) => { e.preventDefault(); camRadius = clamp(camRadius * (1 + e.deltaY * 0.001), 4, 30); }, { passive: false });
}

window.addEventListener('keydown', (e) => {
  if (e.target && /input|textarea/i.test(e.target.tagName)) return;
  const k = e.key.toLowerCase();
  if ('wasd'.includes(k) || k.startsWith('arrow')) { keys.add(k); walkTarget = null; }
});
window.addEventListener('keyup', (e) => keys.delete(e.key.toLowerCase()));

const ray = new THREE.Raycaster();
const waterPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), -WATER_TOP);
const floorPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), -FLOOR_Y);

function onTap(e) {
  const rect = canvas.getBoundingClientRect();
  const ndc = new THREE.Vector2(((e.clientX - rect.left) / rect.width) * 2 - 1, -((e.clientY - rect.top) / rect.height) * 2 + 1);
  ray.setFromCamera(ndc, camera);
  // Over the tank's water surface → feed; otherwise walk to the floor point.
  const wp = new THREE.Vector3();
  if (ray.ray.intersectPlane(waterPlane, wp) && Math.abs(wp.x) < renderedW - 0.3 && Math.abs(wp.z) < renderedD - 0.3) {
    post('/pond/food', { x: wp.x, z: wp.z });
    return;
  }
  const fp = new THREE.Vector3();
  if (ray.ray.intersectPlane(floorPlane, fp)) {
    const c = clampFloor(fp.x, fp.z);
    walkTarget = new THREE.Vector3(c.x, FLOOR_Y, c.z);
  }
}

function clamp(v, lo, hi) { return v < lo ? lo : v > hi ? hi : v; }

function clampFloor(x, z) {
  const roomEdge = Math.max(renderedW, renderedD) + RING_GAP + ROOM_GAP;
  const r = Math.hypot(x, z);
  if (r > roomEdge) { const s = roomEdge / r; x *= s; z *= s; }
  const ex = renderedW + 0.6, ez = renderedD + 0.6;
  if (Math.abs(x) < ex && Math.abs(z) < ez) {
    if (ex - Math.abs(x) < ez - Math.abs(z)) x = Math.sign(x || 1) * ex;
    else z = Math.sign(z || 1) * ez;
  }
  return { x, z };
}

// --- snapshot intake -------------------------------------------------------

let cold = null, self = null;
let spokenUtter = -1;
let renderedW = BASE_W, renderedD = BASE_D;
const hotBuf = []; // {t, x, y, z, yaw} render-behind buffer for the Muskie
let lastHot = null;

const es = new EventSource('/pond/stream');
es.onmessage = (e) => {
  let f;
  try { f = JSON.parse(e.data); } catch { return; }
  switch (f.k) {
    case 'hot': onHot(f); break;
    case 'cold': onCold(f); break;
    case 'moves': applyMoves(f.m || []); break;
    case 'self': onSelf(f); break;
    case 'chat': appendChatLine(f.n, f.b); break;
    case 'chatlog': setChatLog(f.msgs || []); break;
  }
};

function onHot(f) {
  lastHot = f;
  hotBuf.push({ t: performance.now() / 1000, x: f.m.x, y: f.m.y, z: f.m.z, yaw: f.m.yaw });
  while (hotBuf.length > 12) hotBuf.shift();
  setBar('stat-hunger', f.stats.hunger);
  setBar('stat-energy', f.stats.energy);
  setBar('stat-happy', f.stats.happy);
  updateMood(f);
  const servingMe = f.m.srv === mySid;
  if (f.m.st === 'speak' && f.quote) {
    if (servingMe) {
      showQuote(f.quote.t, f.quote.a);
      if (f.m.ut !== spokenUtter) { spokenUtter = f.m.ut; speak(f.quote.p, f.quote.v); }
    } else {
      showQuote(f.quote.t, 'to ' + (f.quote.srv || 'someone'));
    }
  }
  updateButtons(servingMe);
}

function onCold(f) {
  cold = f;
  reconcileCrowd(f.others || []);
  reconcileFood(f.food || []);
  if (countHud) countHud.textContent = f.count + ' here';
}

function onSelf(f) {
  self = f;
  ensureMe(f);
  if (foodHud) foodHud.textContent = 'food: ' + f.food;
  updateQueue();
  updateButtons(lastHot && lastHot.m.srv === mySid);
}

function setBar(id, val) { const el = document.getElementById(id + '-fill'); if (el) el.style.width = clamp(val, 0, 100) + '%'; }

function updateQueue() {
  if (!queueHud) return;
  if (lastHot && lastHot.m.srv === mySid) { queueHud.textContent = 'the Muskie is with you'; queueHud.hidden = false; return; }
  if (self && self.qpos > 0) { queueHud.textContent = 'in line: #' + self.qpos; queueHud.hidden = false; }
  else queueHud.hidden = true;
}

function updateMood(f) {
  if (!moodHud) return;
  let mood = 'content';
  if (f.m.st === 'sleep') mood = 'napping  z z z';
  else if (f.stats.hunger > 80) mood = 'hungry';
  else if (f.stats.happy > 80) mood = 'delighted';
  else if (f.stats.energy < 35) mood = 'sleepy';
  else if (f.stats.happy < 30) mood = 'glum';
  moodHud.textContent = mood;
}

function updateButtons(servingMe) {
  const queued = servingMe || (self && self.qpos > 0);
  if (askBtn) askBtn.disabled = !!queued;
  if (skipBtn) skipBtn.disabled = !!queued || !self || self.food < 1;
}

function showQuote(text, caption) {
  if (quoteText) { quoteText.textContent = text; quoteText.classList.remove('am-quote-empty'); }
  if (quoteAttr) {
    if (caption) { quoteAttr.textContent = caption; quoteAttr.hidden = false; }
    else quoteAttr.hidden = true;
  }
}

// --- render loop -----------------------------------------------------------

let mouth = 0;
let lastMs = performance.now();
let lastSent = 0;
const fwd = new THREE.Vector3(), rightv = new THREE.Vector3();

function tick(nowMs) {
  const dt = Math.min(0.05, (nowMs - lastMs) / 1000);
  lastMs = nowMs;
  const t = nowMs / 1000;

  // Tank size eases toward the server's.
  if (cold && cold.tank) {
    const k = Math.min(1, dt * 1.5);
    renderedW += (cold.tank.w - renderedW) * k;
    renderedD += (cold.tank.d - renderedD) * k;
  }
  tankGroup.scale.set(renderedW / BASE_W, 1, renderedD / BASE_D);

  if (mixer) mixer.update(dt);
  stepMuskie();
  stepMovement(dt);
  stepCamera(dt);
  stepCrowd(dt);
  stepJaw(nowMs, t);

  for (const m of pellets.values()) m.position.y = WATER_TOP - 0.2 + Math.sin(t * 2 + m.id) * 0.03;

  renderer.render(scene, camera);
  requestAnimationFrame(tick);
}
requestAnimationFrame(tick);

// stepMuskie interpolates the Muskie ~HOT_DELAY behind real time for smoothness.
function stepMuskie() {
  if (!model || hotBuf.length === 0) return;
  const rt = performance.now() / 1000 - HOT_DELAY;
  let a = hotBuf[0], b = hotBuf[hotBuf.length - 1];
  for (let i = 0; i < hotBuf.length - 1; i++) {
    if (hotBuf[i].t <= rt && hotBuf[i + 1].t >= rt) { a = hotBuf[i]; b = hotBuf[i + 1]; break; }
  }
  let f = 0;
  if (b.t > a.t) f = clamp((rt - a.t) / (b.t - a.t), 0, 1);
  modelRoot.position.set(a.x + (b.x - a.x) * f, a.y + (b.y - a.y) * f, a.z + (b.z - a.z) * f);
  modelRoot.rotation.y = lerpAngle(a.yaw, b.yaw, f);
}

function stepMovement(dt) {
  if (!meGroup) return;
  // WASD (camera-relative) takes precedence; else walk toward a click target.
  fwd.set(camTarget.x - camera.position.x, 0, camTarget.z - camera.position.z);
  if (fwd.lengthSq() < 1e-6) fwd.set(0, 0, -1);
  fwd.normalize();
  rightv.set(fwd.z, 0, -fwd.x);
  let dx = 0, dz = 0;
  if (keys.has('w') || keys.has('arrowup')) { dx += fwd.x; dz += fwd.z; }
  if (keys.has('s') || keys.has('arrowdown')) { dx -= fwd.x; dz -= fwd.z; }
  if (keys.has('d') || keys.has('arrowright')) { dx += rightv.x; dz += rightv.z; }
  if (keys.has('a') || keys.has('arrowleft')) { dx -= rightv.x; dz -= rightv.z; }
  let nx = mePos.x, nz = mePos.z, moving = false;
  if (dx || dz) {
    const len = Math.hypot(dx, dz);
    nx += (dx / len) * MOVE_SPEED * dt;
    nz += (dz / len) * MOVE_SPEED * dt;
    moving = true;
  } else if (walkTarget) {
    const tx = walkTarget.x - mePos.x, tz = walkTarget.z - mePos.z;
    const d = Math.hypot(tx, tz);
    if (d < 0.15) { walkTarget = null; }
    else { const step = Math.min(d, MOVE_SPEED * dt); nx += (tx / d) * step; nz += (tz / d) * step; moving = true; }
  }
  if (moving) {
    const hx = nx - mePos.x, hz = nz - mePos.z; // intended step (pre-clamp) = heading
    const c = clampFloor(nx, nz);
    mePos.set(c.x, FLOOR_Y, c.z);
    if (hx * hx + hz * hz > 1e-7) {
      // Turn to face the travel direction (+Z is the avatar's front), smoothed.
      const heading = Math.atan2(hx, hz);
      meGroup.rotation.y = lerpAngle(meGroup.rotation.y, heading, Math.min(1, dt * 12));
      // Ease the camera behind you while click-walking so you never walk
      // off-screen; WASD leaves the camera where you aimed it (drag to look).
      autoTrail = walkTarget ? trailTheta(heading) : null;
    }
    const now = performance.now();
    if (now - lastSent > 140) { lastSent = now; post('/pond/move', { x: mePos.x, z: mePos.z }); }
  } else {
    autoTrail = null;
  }
  meGroup.position.copy(mePos);
}

function stepCamera(dt) {
  const av = meGroup ? meGroup.position : { x: 0, z: 0 };
  // Third-person: the camera targets YOU (head height) and orbits behind, so
  // "forward" is always into the screen. Drag orbits, wheel zooms, and
  // click-walking eases the camera in behind you.
  const k = Math.min(1, dt * 8);
  camTarget.x += (av.x - camTarget.x) * k;
  camTarget.y += (FLOOR_Y + 1.1 - camTarget.y) * k;
  camTarget.z += (av.z - camTarget.z) * k;
  if (autoTrail != null && !dragging) camTheta = lerpAngle(camTheta, autoTrail, Math.min(1, dt * 3));
  const sinPhi = Math.sin(camPhi);
  camera.position.set(
    camTarget.x + camRadius * sinPhi * Math.cos(camTheta),
    camTarget.y + camRadius * Math.cos(camPhi),
    camTarget.z + camRadius * sinPhi * Math.sin(camTheta),
  );
  camera.lookAt(camTarget);
}

function stepCrowd(dt) {
  const k = Math.min(1, dt * 8);
  for (const c of crowd.values()) {
    const p = c.group.position;
    p.x += (c.tx - p.x) * k;
    p.z += (c.tz - p.z) * k;
    p.y = FLOOR_Y;
  }
}

function stepJaw(nowMs, t) {
  const napping = lastHot && lastHot.m.st === 'sleep';
  zzz.visible = napping;
  if (napping) zzz.position.set(modelRoot.position.x, modelRoot.position.y + 1.1, modelRoot.position.z);
  let target = 0;
  const speaking = lastHot && lastHot.m.st === 'speak';
  if (speaking && lastHot.m.srv === mySid && analyser && nowMs < speakingUntil) {
    analyser.getFloatTimeDomainData(ampData);
    let sum = 0;
    for (let i = 0; i < ampData.length; i++) sum += ampData[i] * ampData[i];
    target = Math.min(1, Math.sqrt(sum / ampData.length) * 9);
  } else if (speaking) {
    target = 0.5 + 0.4 * Math.sin(t * 22);
  }
  mouth += (target - mouth) * (target > mouth ? 0.6 : 0.25);
  setMouth(mouth);
}

function lerpAngle(a, b, t) {
  let d = (b - a) % (Math.PI * 2);
  if (d > Math.PI) d -= Math.PI * 2;
  if (d < -Math.PI) d += Math.PI * 2;
  return a + d * t;
}

// --- customize modal -------------------------------------------------------

buildCustomizeUI();

function buildCustomizeUI() {
  const body = document.getElementById('customize-body');
  if (!body) return;
  let sel = { shape: '', bc: 0, sk: 0 };
  const seedRef = () => (self ? self.i : 1);

  const shapeRow = swatchRow(SHAPES.keys.map((k) => ({ key: k, label: SHAPES.map[k].label, short: SHAPES.map[k].short })),
    (k) => { sel.shape = k; refresh(); }, true);
  const bodyRow = swatchRow(BODY_PALETTE.map((hex, i) => ({ key: i, color: hex })),
    (i) => { sel.bc = i; refresh(); });
  const skinRow = swatchRow(SKIN_PALETTE.map((hex, i) => ({ key: i, color: hex })),
    (i) => { sel.sk = i; refresh(); });

  body.innerHTML = '';
  body.append(labeled('Shape', shapeRow.el), labeled('Body color', bodyRow.el), labeled('Skin', skinRow.el));
  const save = document.createElement('button');
  save.className = 'am-ask';
  save.textContent = 'Save look';
  save.addEventListener('click', async () => {
    await post('/me/avatar', { shape: sel.shape, bc: sel.bc, sk: sel.sk });
    document.getElementById('customize-modal').close();
  });
  body.append(save);

  function refresh() {
    shapeRow.mark(sel.shape);
    bodyRow.mark(sel.bc);
    skinRow.mark(sel.sk);
  }
  // Seed from current self when the modal opens.
  const modal = document.getElementById('customize-modal');
  if (modal) modal.addEventListener('close', () => {});
  if (customizeBtn) customizeBtn.addEventListener('click', () => {
    if (self) { sel = { shape: self.shape || SHAPES.keys[0], bc: self.bc, sk: self.sk }; refresh(); }
  });
}

function labeled(label, el) {
  const wrap = document.createElement('div');
  wrap.className = 'am-cust-row';
  const h = document.createElement('div');
  h.className = 'am-cust-label';
  h.textContent = label;
  wrap.append(h, el);
  return wrap;
}

function swatchRow(items, onPick, isShape) {
  const el = document.createElement('div');
  el.className = 'am-swatches';
  const btns = new Map();
  for (const it of items) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'am-swatch' + (isShape ? ' am-swatch-shape' : '');
    if (it.color) b.style.background = it.color;
    if (it.label) b.title = it.label;
    if (isShape) b.textContent = it.short || it.label;
    b.addEventListener('click', () => onPick(it.key));
    btns.set(it.key, b);
    el.append(b);
  }
  return {
    el,
    mark(key) { for (const [k, b] of btns) b.classList.toggle('is-active', k === key); },
  };
}
