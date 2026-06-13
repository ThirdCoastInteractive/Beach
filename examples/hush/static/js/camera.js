// hush camera — the client-side island. Opens the camera, runs each frame
// through a WebGPU (WGSL) filter pipeline, and on capture grabs the *filtered*
// canvas as a JPEG blob and posts it to the same /upload endpoint the wall's
// plain upload uses. Everything else in hush is SSR; this is the only real JS.
//
// "Capture the raw image, then modify": the shader modifies the live preview, so
// the filtered canvas already IS the modified still — toBlob needs no post-step.
//
// No WebGPU? The capture button is disabled and the page points back to the
// wall's plain (unfiltered) upload, so a device without WebGPU can still post.

const canvas = document.getElementById("cam-canvas");
const statusEl = document.getElementById("cam-status");
const shootBtn = document.getElementById("cam-shoot");
const filterSel = document.getElementById("cam-filter");

// Filter presets → shader params: vec4(brightness, contrast, saturation, temp).
const FILTERS = {
  none:   [0.0, 1.0, 1.0, 0.0],
  noir:   [0.0, 1.2, 0.0, 0.0],
  bleach: [0.05, 1.3, 0.6, 0.02],
  dream:  [0.08, 0.9, 1.3, 0.05],
};

const WGSL = `
struct Params { adj: vec4<f32> };
@group(0) @binding(0) var samp: sampler;
@group(0) @binding(1) var tex: texture_external;
@group(0) @binding(2) var<uniform> p: Params;

struct VSOut { @builtin(position) pos: vec4<f32>, @location(0) uv: vec2<f32> };

@vertex fn vs(@builtin(vertex_index) i: u32) -> VSOut {
  var corners = array<vec2<f32>, 3>(vec2(-1.0, -1.0), vec2(3.0, -1.0), vec2(-1.0, 3.0));
  let xy = corners[i];
  var out: VSOut;
  out.pos = vec4(xy, 0.0, 1.0);
  out.uv = vec2((xy.x + 1.0) * 0.5, 1.0 - (xy.y + 1.0) * 0.5);
  return out;
}

@fragment fn fs(in: VSOut) -> @location(0) vec4<f32> {
  var c = textureSampleBaseClampToEdge(tex, samp, in.uv).rgb;
  c = c + p.adj.x;                                   // brightness
  c = (c - 0.5) * p.adj.y + 0.5;                     // contrast
  let l = dot(c, vec3(0.299, 0.587, 0.114));
  c = mix(vec3(l), c, p.adj.z);                      // saturation
  c = c + vec3(p.adj.w, 0.0, -p.adj.w);              // temperature
  return vec4(clamp(c, vec3(0.0), vec3(1.0)), 1.0);
}`;

function setStatus(msg) {
  if (statusEl) statusEl.textContent = msg;
}

async function main() {
  if (!navigator.gpu) {
    setStatus("WebGPU isn't available in this browser — use “upload image” on the wall instead.");
    if (shootBtn) shootBtn.disabled = true;
    return;
  }

  let stream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "user" }, audio: false });
  } catch (err) {
    setStatus("couldn't open the camera: " + err.message);
    if (shootBtn) shootBtn.disabled = true;
    return;
  }

  const video = document.createElement("video");
  video.srcObject = stream;
  video.muted = true;
  video.playsInline = true;
  await video.play();

  const adapter = await navigator.gpu.requestAdapter();
  const device = await adapter.requestDevice();
  const ctx = canvas.getContext("webgpu");
  const format = navigator.gpu.getPreferredCanvasFormat();
  ctx.configure({ device, format, alphaMode: "opaque" });

  const sampler = device.createSampler({ magFilter: "linear", minFilter: "linear" });
  const params = device.createBuffer({ size: 16, usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST });

  const module = device.createShaderModule({ code: WGSL });
  const pipeline = device.createRenderPipeline({
    layout: "auto",
    vertex: { module, entryPoint: "vs" },
    fragment: { module, entryPoint: "fs", targets: [{ format }] },
    primitive: { topology: "triangle-list" },
  });

  function applyFilter() {
    const f = FILTERS[filterSel ? filterSel.value : "none"] || FILTERS.none;
    device.queue.writeBuffer(params, 0, new Float32Array(f));
  }
  if (filterSel) filterSel.addEventListener("change", applyFilter);
  applyFilter();

  let pendingCapture = false;
  if (shootBtn) shootBtn.addEventListener("click", () => { pendingCapture = true; });

  setStatus("pick a filter, then capture.");

  function frame() {
    // Wait until the video has real dimensions.
    if (video.readyState < 2) { requestAnimationFrame(frame); return; }

    const external = device.importExternalTexture({ source: video });
    const bind = device.createBindGroup({
      layout: pipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: sampler },
        { binding: 1, resource: external },
        { binding: 2, resource: { buffer: params } },
      ],
    });

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
    pass.setBindGroup(0, bind);
    pass.draw(3);
    pass.end();
    device.queue.submit([encoder.finish()]);

    if (pendingCapture) {
      pendingCapture = false;
      capture(stream);
      return; // capture() tears down and navigates
    }
    requestAnimationFrame(frame);
  }
  requestAnimationFrame(frame);
}

function capture(stream) {
  // the § 2257 certification gates the shutter; the server refuses uploads
  // without the field regardless, this just tells the truth before the trip
  const cert = document.getElementById("cam-2257");
  if (cert && !cert.checked) { setStatus("confirm the § 2257 certification first."); return; }
  setStatus("posting…");
  canvas.toBlob((blob) => {
    if (!blob) { setStatus("capture failed — try again."); requestAnimationFrame(() => {}); return; }
    const fd = new FormData();
    fd.append("image", blob, "capture.jpg");
    fd.append("cert2257", "on");
    fetch("/upload", { method: "POST", body: fd })
      .then(() => { stream.getTracks().forEach((t) => t.stop()); window.location = "/"; })
      .catch((err) => setStatus("upload failed: " + err.message));
  }, "image/jpeg", 0.9);
}

main();
