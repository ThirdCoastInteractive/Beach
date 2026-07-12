// shapes.js — the avatar body-shape catalog and the color/skin palettes.
//
// Bodies are polyhedra: the 5 Platonic via three.js built-ins, plus Archimedean
// and Johnson solids built as the convex hull of a coordinate-formula vertex set
// (ConvexGeometry). Every geometry is normalized to a common size so avatars
// look consistent in the crowd.
//
// BODY_PALETTE / SKIN_PALETTE lengths MUST match internal/pond's bodyPaletteLen
// / skinPaletteLen so server-derived indices line up.

import * as THREE from '/static/js/vendor/three/three.module.js';
import { ConvexGeometry } from '/static/js/vendor/three/ConvexGeometry.js';

const S2 = Math.SQRT2;

// allPerms returns every coordinate permutation of (a,b,c) with all sign combos
// (deduped) — the vertex set for octahedral-symmetry solids.
function allPerms(a, b, c) {
  const orders = [[a, b, c], [a, c, b], [b, a, c], [b, c, a], [c, a, b], [c, b, a]];
  const seen = new Set();
  const pts = [];
  for (const [x, y, z] of orders) {
    for (const sx of [1, -1]) {
      for (const sy of [1, -1]) {
        for (const sz of [1, -1]) {
          const p = [x * sx, y * sy, z * sz];
          const key = p.map((n) => n.toFixed(4)).join(',');
          if (!seen.has(key)) { seen.add(key); pts.push(new THREE.Vector3(p[0], p[1], p[2])); }
        }
      }
    }
  }
  return pts;
}

function hull(pts) { return new ConvexGeometry(pts); }

// bipyramid / pyramid build simple Johnson-style convex solids.
function ring(n) {
  const pts = [];
  for (let i = 0; i < n; i++) {
    const a = (i / n) * Math.PI * 2;
    pts.push(new THREE.Vector3(Math.cos(a), 0, Math.sin(a)));
  }
  return pts;
}
function pyramid(n, h) { return hull([...ring(n), new THREE.Vector3(0, h, 0)]); }
function bipyramid(n, h) { return hull([...ring(n), new THREE.Vector3(0, h, 0), new THREE.Vector3(0, -h, 0)]); }

// prism stacks two parallel n-gon rings; antiprism rotates the lower ring by
// half a step so the sides become triangles.
function prism(n, h) {
  const top = ring(n).map((p) => new THREE.Vector3(p.x, h, p.z));
  const bot = ring(n).map((p) => new THREE.Vector3(p.x, -h, p.z));
  return hull([...top, ...bot]);
}
function antiprism(n, h) {
  const pts = [];
  for (let i = 0; i < n; i++) {
    const a = (i / n) * Math.PI * 2;
    pts.push(new THREE.Vector3(Math.cos(a), h, Math.sin(a)));
    const b = a + Math.PI / n;
    pts.push(new THREE.Vector3(Math.cos(b), -h, Math.sin(b)));
  }
  return hull(pts);
}

// truncTetra is the truncated tetrahedron: permutations of (0,±1,±3) with an
// even number of minus signs.
function truncTetra() {
  const v = [
    [0, 1, 3], [0, -1, -3], [0, 3, 1], [0, -3, -1],
    [1, 0, 3], [-1, 0, -3], [3, 0, 1], [-3, 0, -1],
    [1, 3, 0], [-1, -3, 0], [3, 1, 0], [-3, -1, 0],
  ];
  return hull(v.map((p) => new THREE.Vector3(p[0], p[1], p[2])));
}

function normalize(geo, r) {
  geo.computeBoundingSphere();
  const s = (r || 0.5) / (geo.boundingSphere.radius || 1);
  geo.scale(s, s, s);
  geo.computeVertexNormals();
  return geo;
}

function buildShapes() {
  const xi = S2 - 1;
  // [key, short label (for the swatch button), full name (tooltip), geometry].
  // 20 curated solids: 5 Platonic, 6 Archimedean, 3 prisms, 2 antiprisms, and
  // 4 pyramids/bipyramids.
  const defs = [
    ['tetra', 'Tetra', 'Tetrahedron', new THREE.TetrahedronGeometry(0.6)],
    ['cube', 'Cube', 'Cube', new THREE.BoxGeometry(0.8, 0.8, 0.8)],
    ['octa', 'Octa', 'Octahedron', new THREE.OctahedronGeometry(0.6)],
    ['dodeca', 'Dodec', 'Dodecahedron', new THREE.DodecahedronGeometry(0.6)],
    ['icosa', 'Icosa', 'Icosahedron', new THREE.IcosahedronGeometry(0.6)],
    ['trunctetra', 'Tr.Tet', 'Truncated tetrahedron', truncTetra()],
    ['cubocta', 'CubOct', 'Cuboctahedron', hull(allPerms(1, 1, 0))],
    ['truncocta', 'Tr.Oct', 'Truncated octahedron', hull(allPerms(0, 1, 2))],
    ['trunccube', 'Tr.Cub', 'Truncated cube', hull(allPerms(xi, 1, 1))],
    ['rhombi', 'Rhomb', 'Rhombicuboctahedron', hull(allPerms(1, 1, 1 + S2))],
    ['greatrhombi', 'Gt.Rh', 'Truncated cuboctahedron', hull(allPerms(1, 1 + S2, 1 + 2 * S2))],
    ['triprism', 'Pri.3', 'Triangular prism', prism(3, 0.9)],
    ['pentprism', 'Pri.5', 'Pentagonal prism', prism(5, 0.9)],
    ['hexprism', 'Pri.6', 'Hexagonal prism', prism(6, 0.9)],
    ['sqanti', 'Ant.4', 'Square antiprism', antiprism(4, 0.7)],
    ['pentanti', 'Ant.5', 'Pentagonal antiprism', antiprism(5, 0.7)],
    ['pyramid', 'Pyr.4', 'Square pyramid', pyramid(4, 1.3)],
    ['tribipyr', 'BiP.3', 'Triangular bipyramid', bipyramid(3, 1.0)],
    ['pentbipyr', 'BiP.5', 'Pentagonal bipyramid', bipyramid(5, 1.1)],
    ['hexbipyr', 'BiP.6', 'Hexagonal bipyramid', bipyramid(6, 1.1)],
  ];
  const map = {};
  const keys = [];
  for (const [key, short, label, geo] of defs) {
    normalize(geo, 0.5);
    map[key] = { key, short, label, geo };
    keys.push(key);
  }
  return { map, keys };
}

export const SHAPES = buildShapes();

// shapeKeyFor resolves a shape: the chosen key, or a stable default from a seed.
export function shapeKeyFor(key, seed) {
  if (key && SHAPES.map[key]) return key;
  return SHAPES.keys[Math.abs(seed | 0) % SHAPES.keys.length];
}

export function shapeGeo(key) {
  const s = SHAPES.map[key];
  return s ? s.geo : SHAPES.map[SHAPES.keys[0]].geo;
}

// 14 body colors (RYBitten-flavored spectrum). Length == pond.bodyPaletteLen.
export const BODY_PALETTE = [
  '#c0392b', '#d35400', '#e67e22', '#f1c40f', '#7f9a3a', '#27ae60',
  '#16a085', '#2980b9', '#5b6dc8', '#8e44ad', '#c0398f', '#7d5a3c',
  '#9aa7b0', '#34495e',
];

// 9 skin tones: emoji yellow + 5 Fitzpatrick + 3 alien. Length == pond.skinPaletteLen.
export const SKIN_PALETTE = [
  '#ffc83d', // emoji default yellow
  '#f7dece', '#e0bb95', '#c99a6b', '#a16e4b', '#5c3d2e', // Fitzpatrick light→dark
  '#9b59b6', '#4aa3df', '#58d68d', // alien purple / blue / green
];

function rand01(seed) {
  const x = Math.sin(seed * 12.9898) * 43758.5453;
  return x - Math.floor(x);
}

// bodyColor returns the palette color at idx, nudged a little per-seed so two
// visitors who pick the same swatch still look individual.
export function bodyColor(idx, seed) {
  const hex = BODY_PALETTE[((idx % BODY_PALETTE.length) + BODY_PALETTE.length) % BODY_PALETTE.length] || '#888888';
  const c = new THREE.Color(hex);
  const hsl = {};
  c.getHSL(hsl);
  hsl.h = (hsl.h + (rand01(seed) - 0.5) * 0.06 + 1) % 1;
  hsl.s = Math.min(1, Math.max(0.2, hsl.s + (rand01(seed + 7) - 0.5) * 0.12));
  hsl.l = Math.min(0.82, Math.max(0.22, hsl.l + (rand01(seed + 13) - 0.5) * 0.12));
  return new THREE.Color().setHSL(hsl.h, hsl.s, hsl.l);
}

export function skinColor(idx) {
  const hex = SKIN_PALETTE[((idx % SKIN_PALETTE.length) + SKIN_PALETTE.length) % SKIN_PALETTE.length] || '#f1c27d';
  return new THREE.Color(hex);
}
