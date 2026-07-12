// lotophages — a 3D lotus on the golden angle, 1-bit dithered, cyanotype.
(() => {
  "use strict";
  const THREE = window.THREE;
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const canvas = document.getElementById("c");
  const PHI = (1 + Math.sqrt(5)) / 2;            // golden ratio
  const GA = Math.PI * (3 - Math.sqrt(5));       // golden angle ≈ 137.507°
  const TAU = Math.PI * 2;

  const renderer = new THREE.WebGLRenderer({ canvas, antialias: false, powerPreference: "high-performance" });
  const PR = Math.min(devicePixelRatio || 1, 1.25);
  renderer.setPixelRatio(PR);
  renderer.outputEncoding = THREE.sRGBEncoding;
  renderer.toneMapping = THREE.NoToneMapping;

  const scene = new THREE.Scene();
  scene.background = new THREE.Color(0x000205);

  const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 100);
  camera.position.set(0, 3.3, 4.7);
  camera.lookAt(0, 0.05, 0);

  scene.add(new THREE.AmbientLight(0x2a3c5c, 0.16));      // low fill -> deep shadows between petals
  const key = new THREE.DirectionalLight(0xffffff, 1.6);
  key.position.set(1.7, 2.4, 1.5); scene.add(key);
  const rim = new THREE.DirectionalLight(0x8fc0ff, 0.75); // back-rim picks out petal edges
  rim.position.set(-1.7, 0.5, -1.5); scene.add(rim);

  const lotusMat = new THREE.MeshStandardMaterial({
    color: 0x8fb2da, roughness: 0.68, metalness: 0.0, side: THREE.DoubleSide,
    emissive: 0x060f1f, emissiveIntensity: 0.04,
  });
  // the seed pod glows so its golden-angle spiral reads
  const seedMat = new THREE.MeshStandardMaterial({
    color: 0xbfe0ff, roughness: 0.6, metalness: 0.0,
    emissive: 0x2a5fa0, emissiveIntensity: 0.55,
  });

  // ── petals: rings advanced by the golden angle ──
  function petalGeometry() {
    const g = new THREE.PlaneGeometry(1, 1.85, 10, 22);
    const p = g.attributes.position;
    for (let i = 0; i < p.count; i++) {
      const x = p.getX(i), y = p.getY(i);
      const t = (y + 0.925) / 1.85;
      const w = Math.pow(Math.sin(Math.PI * Math.min(1, Math.max(0, t))), 0.62);
      const nx = x * w;
      const cup = (1 - Math.cos(nx * Math.PI)) * 0.28 * (0.35 + 0.65 * t);
      const curl = 0.55 * t * t;
      const belly = Math.sin(t * Math.PI) * 0.12;
      p.setX(i, nx); p.setZ(i, cup + curl - belly);
    }
    g.translate(0, 0.925, 0); g.computeVertexNormals();
    return g;
  }
  const petalGeo = petalGeometry();

  const lotus = new THREE.Group();
  const petals = [];
  let az = 0;
  function ring(count, tilt, scale, yBase, amp) {
    for (let i = 0; i < count; i++) {
      const m = new THREE.Mesh(petalGeo, lotusMat);
      m.scale.setScalar(scale); m.rotation.x = tilt;
      const pivot = new THREE.Object3D();
      pivot.add(m); pivot.position.y = yBase;
      pivot.rotation.y = (az += GA);              // each petal advanced by golden angle
      lotus.add(pivot);
      petals.push({ m, tilt, amp });
    }
  }
  ring(13, 1.52, 1.00, 0.02, 0.08);   // reflexed — laid back past flat
  ring(9,  1.30, 0.78, 0.05, 0.10);
  ring(6,  1.06, 0.56, 0.08, 0.12);

  // ── the seed pod: phyllotaxis (golden-angle spiral), like a lotus seedhead ──
  const SEEDS = 540;
  const seedGeo = new THREE.SphereGeometry(1, 7, 6);
  const seeds = new THREE.InstancedMesh(seedGeo, seedMat, SEEDS);
  const mtx = new THREE.Matrix4(), q = new THREE.Quaternion(), v = new THREE.Vector3(), s = new THREE.Vector3();
  const POD_R = 0.72;
  for (let i = 0; i < SEEDS; i++) {
    const f = i / SEEDS;
    const r = Math.sqrt(f) * POD_R;
    const a = i * GA;
    const x = Math.cos(a) * r, z = Math.sin(a) * r;
    const y = 0.26 + (POD_R * POD_R - r * r) * 0.30;      // raised above the petals, domed
    const sc = THREE.MathUtils.lerp(0.05, 0.026, f);
    v.set(x, y, z); s.setScalar(sc);
    mtx.compose(v, q, s); seeds.setMatrixAt(i, mtx);
  }
  seeds.instanceMatrix.needsUpdate = true;
  lotus.add(seeds);

  lotus.position.y = 0.1;
  lotus.rotation.x = 0.0;
  lotus.scale.setScalar(0.82);
  scene.add(lotus);

  // ── grid-free 1-bit dither, with depth-edge outlines + subtle artifacts ──
  let rt;
  const ditherMat = new THREE.ShaderMaterial({
    uniforms: {
      tDiffuse: { value: null },
      tDepth: { value: null },
      uScale: { value: 1.3 * PR },
      uTexel: { value: new THREE.Vector2() },
      uCam: { value: new THREE.Vector2(camera.near, camera.far) },
      uPhase: { value: 0 },
      uTime: { value: 0 },
      uDark: { value: new THREE.Color(0x0c2f63) },
      uLight: { value: new THREE.Color(0xdcefff) },
    },
    vertexShader: `varying vec2 vUv; void main(){ vUv = uv; gl_Position = vec4(position.xy, 0.0, 1.0); }`,
    fragmentShader: /* glsl */`
      precision highp float;
      uniform sampler2D tDiffuse, tDepth;
      uniform float uScale, uPhase, uTime;
      uniform vec2 uTexel, uCam;
      uniform vec3 uDark, uLight;
      varying vec2 vUv;
      float ign(vec2 p){ return fract(52.9829189 * fract(dot(p, vec2(0.06711056, 0.00583715)))); }
      float hash(float n){ return fract(sin(n) * 43758.5453); }
      float lin(vec2 uv){
        float z = texture2D(tDepth, uv).x * 2.0 - 1.0;
        return (2.0 * uCam.x * uCam.y) / (uCam.y + uCam.x - z * (uCam.y - uCam.x));
      }
      void main(){
        vec2 uv = vUv;
        // rare horizontal slip — a quiet signal tear
        float row = floor(gl_FragCoord.y / 3.0);
        uv.x += step(0.991, hash(row + floor(uTime * 5.0))) * (hash(row * 1.7) - 0.5) * 0.012;

        vec3 c = texture2D(tDiffuse, uv).rgb;
        float lum = dot(c, vec3(0.299, 0.587, 0.114));
        lum = clamp((lum - 0.05) * 1.4, 0.0, 1.0);

        // depth-edge outline: dark contour where petals overlap / meet the void
        float d0 = lin(uv);
        float e = 0.0;
        e = max(e, abs(d0 - lin(uv + vec2( uTexel.x, 0.0))));
        e = max(e, abs(d0 - lin(uv + vec2(-uTexel.x, 0.0))));
        e = max(e, abs(d0 - lin(uv + vec2(0.0,  uTexel.y))));
        e = max(e, abs(d0 - lin(uv + vec2(0.0, -uTexel.y))));
        float edge = smoothstep(0.04, 0.16, e);
        lum *= 1.0 - 0.92 * edge;                                // burn the outlines in

        vec2 qv = vUv - 0.5;
        lum *= 1.0 - dot(qv, qv) * 0.35;                         // soft vignette

        float th = fract(ign(floor(gl_FragCoord.xy / uScale)) + uPhase);
        vec3 col = mix(uDark, uLight, step(th, lum));
        col *= 0.92 + 0.08 * sin(gl_FragCoord.y * 2.094);        // subtle scanlines
        gl_FragColor = vec4(col, 1.0);
      }
    `,
  });
  ditherMat.toneMapped = false;
  const postScene = new THREE.Scene();
  postScene.add(new THREE.Mesh(new THREE.PlaneGeometry(2, 2), ditherMat));
  const postCam = new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1);

  function makeRT(w, h) {
    if (rt) rt.dispose();
    const depth = new THREE.DepthTexture(w, h);
    rt = new THREE.WebGLRenderTarget(w, h, {
      minFilter: THREE.NearestFilter, magFilter: THREE.NearestFilter, depthTexture: depth, depthBuffer: true,
    });
    ditherMat.uniforms.tDiffuse.value = rt.texture;
    ditherMat.uniforms.tDepth.value = rt.depthTexture;
    ditherMat.uniforms.uTexel.value.set(1 / w, 1 / h);
  }

  function resize() {
    const w = innerWidth, h = innerHeight;
    renderer.setSize(w, h, false);
    camera.aspect = w / h; camera.updateProjectionMatrix();
    makeRT(Math.floor(w * PR), Math.floor(h * PR));
  }
  addEventListener("resize", resize); resize();

  // ── animate ──
  let t0 = 0, phase = 0;
  function frame(ms) {
    const t = ms * 0.001;
    const dt = Math.min(0.05, t - t0); t0 = t;
    const slow = reduce ? 0.03 : 0.12;
    lotus.rotation.y += dt * slow;
    lotus.position.y = -0.1 + Math.sin(t * 0.5) * 0.04;
    const open = reduce ? 0 : Math.sin(t * 0.32);
    for (const pt of petals) pt.m.rotation.x = pt.tilt + open * pt.amp;
    ditherMat.uniforms.uTime.value = reduce ? 0 : t;
    if (!reduce) { phase = (phase + dt * 0.61803399 * 0.5) % 1; ditherMat.uniforms.uPhase.value = phase; }

    renderer.setRenderTarget(rt);
    renderer.render(scene, camera);
    renderer.setRenderTarget(null);
    renderer.render(postScene, postCam);
    requestAnimationFrame(frame);
  }
  requestAnimationFrame(frame);
})();
