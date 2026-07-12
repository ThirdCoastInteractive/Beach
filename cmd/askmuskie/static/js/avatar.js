// avatar.js — builds a visitor avatar (polyhedron body + head with a face) from
// the shape/color/skin palettes. Used for your own avatar, the crowd, and the
// customize preview. The face (two eyes + a snout) sits on the local +Z side so
// the avatar's heading is readable — the controller rotates the group to face the
// travel direction, and +Z is "forward".

import * as THREE from '/static/js/vendor/three/three.module.js';
import { shapeGeo, shapeKeyFor, bodyColor, skinColor } from '/static/js/shapes.js';

const EYE_GEO = new THREE.SphereGeometry(0.055, 10, 8);
const EYE_MAT = new THREE.MeshStandardMaterial({ color: 0x161616, roughness: 0.35 });
const NOSE_GEO = new THREE.ConeGeometry(0.075, 0.2, 14);

// makeAvatar builds an avatar group. seed (the visitor's short id) drives the
// per-user color jitter and the default shape when none is chosen.
export function makeAvatar(shapeKey, bcIdx, skIdx, seed) {
  const g = new THREE.Group();
  const skin = skinColor(skIdx);

  const body = new THREE.Mesh(
    shapeGeo(shapeKeyFor(shapeKey, seed)),
    new THREE.MeshStandardMaterial({ color: bodyColor(bcIdx, seed), roughness: 0.5, metalness: 0.05, flatShading: true }),
  );
  body.position.y = 0.55;
  g.add(body);

  const head = new THREE.Mesh(
    new THREE.SphereGeometry(0.26, 18, 16),
    new THREE.MeshStandardMaterial({ color: skin, roughness: 0.7 }),
  );
  head.position.y = 1.15;
  g.add(head);

  // Face on the +Z front: two eyes and a snout, so you can read which way the
  // avatar is heading (its own forward).
  const eyeL = new THREE.Mesh(EYE_GEO, EYE_MAT);
  eyeL.position.set(-0.1, 1.2, 0.21);
  const eyeR = new THREE.Mesh(EYE_GEO, EYE_MAT);
  eyeR.position.set(0.1, 1.2, 0.21);
  g.add(eyeL, eyeR);

  const nose = new THREE.Mesh(NOSE_GEO, new THREE.MeshStandardMaterial({ color: skin, roughness: 0.7 }));
  nose.rotation.x = Math.PI / 2; // cone apex points +Z (forward)
  nose.position.set(0, 1.11, 0.3);
  g.add(nose);

  g.userData.body = body;
  g.userData.head = head;
  g.userData.nose = nose;
  return g;
}

// applyAvatar mutates an avatar group's appearance in place (live preview).
export function applyAvatar(g, shapeKey, bcIdx, skIdx, seed) {
  g.userData.body.geometry = shapeGeo(shapeKeyFor(shapeKey, seed));
  g.userData.body.material.color = bodyColor(bcIdx, seed);
  const skin = skinColor(skIdx);
  g.userData.head.material.color = skin;
  if (g.userData.nose) g.userData.nose.material.color = skin;
}
