// Ant sketches for the design gate (docs/18, section 11, step 0).
// Not part of the build. Every ant is drawn from code in Canvas 2D; the legs
// follow a procedural alternating-tripod gait: the front and hind leg on one
// side and the middle leg on the other move together, and a planted foot
// stays fixed to the ground (the gait phase advances with the distance
// walked, so feet never slide). The engine of step 2 starts from this file.
"use strict";

const TAU = Math.PI * 2;

// Palettes per theme, from the PWA tokens (docs/07).
const PALETTE = {
  dark: {
    body: "#2e2828", line: "#d9d2d1", leg: "#b8afae", eye: "#fbf8f7", pupil: "#0b0a0a",
    red: "#d42a2e", redLine: "#ff6b6e", bg: "#0b0a0a", ground: "#2a2525", blush: "#ff6b6e55",
  },
  light: {
    body: "#1c1919", line: "#141111", leg: "#141111", eye: "#ffffff", pupil: "#0b0a0a",
    red: "#c1121f", redLine: "#a40f1a", bg: "#faf8f7", ground: "#e6e0de", blush: "#c1121f40",
  },
};

// Gait: legs 0..5 = L1 L2 L3 R1 R2 R3 (front, middle, hind). Tripod A = L1 R2 L3.
const TRIPOD = [0, 0.5, 0, 0.5, 0, 0.5];

// Foot offset along the body axis and lift (0..1) for leg i at gait phase t.
function footCycle(t, i, stride) {
  const p = (t + TRIPOD[i]) % 1;
  if (p < 0.5) {
    // stance: planted, moves backwards relative to the body
    return { dx: stride * (0.5 - p * 2), lift: 0 };
  }
  // swing: lifted, moves forwards (eased)
  const q = (p - 0.5) * 2;
  const e = q * q * (3 - 2 * q);
  return { dx: stride * (e - 0.5), lift: Math.sin(Math.PI * q) };
}

// Two-bone IK: returns the knee, choosing the candidate `pick` prefers.
function knee(hx, hy, fx, fy, a, b, pick) {
  let dx = fx - hx, dy = fy - hy;
  let d = Math.hypot(dx, dy);
  const max = a + b - 0.01;
  if (d > max) { dx *= max / d; dy *= max / d; d = max; }
  const th = Math.atan2(dy, dx);
  const al = Math.acos(Math.max(-1, Math.min(1, (a * a + d * d - b * b) / (2 * a * d || 1))));
  const k1 = [hx + a * Math.cos(th + al), hy + a * Math.sin(th + al)];
  const k2 = [hx + a * Math.cos(th - al), hy + a * Math.sin(th - al)];
  return pick(k1, k2) ? k1 : k2;
}

function ellipse(ctx, x, y, rx, ry, rot = 0) {
  ctx.beginPath();
  ctx.ellipse(x, y, rx, ry, rot, 0, TAU);
}

function shape(ctx, fill, stroke, width) {
  if (fill) { ctx.fillStyle = fill; ctx.fill(); }
  if (stroke) { ctx.strokeStyle = stroke; ctx.lineWidth = width; ctx.stroke(); }
}

function polyline(ctx, pts, color, width) {
  ctx.beginPath();
  ctx.moveTo(pts[0][0], pts[0][1]);
  for (let i = 1; i < pts.length; i++) ctx.lineTo(pts[i][0], pts[i][1]);
  ctx.strokeStyle = color;
  ctx.lineWidth = width;
  ctx.lineCap = "round";
  ctx.lineJoin = "round";
  ctx.stroke();
}

// Blinks: closed for ~120 ms every few seconds, different per ant.
function blink(s) {
  const p = (s.time + s.seed * 3.7) % 4.2;
  return p < 0.12 ? 1 - Math.abs(p - 0.06) / 0.06 : 0;
}

// A cartoon eye: sclera, pupil looking ahead, highlight, optional lid.
function eye(ctx, x, y, rx, ry, pal, s, opts = {}) {
  const shut = Math.max(blink(s), opts.lid || 0);
  ellipse(ctx, x, y, rx, ry);
  shape(ctx, pal.eye, opts.ring ? pal.line : null, opts.ring || 0);
  if (shut < 0.95) {
    const pr = Math.min(rx, ry) * (opts.pupil || 0.55);
    const px = x + (rx - pr) * 0.55 + (opts.lookX || 0);
    const py = y + (opts.lookY || 0);
    ctx.save();
    ellipse(ctx, x, y, rx, ry);
    ctx.clip();
    ellipse(ctx, px, py, pr, pr);
    shape(ctx, pal.pupil);
    ellipse(ctx, px + pr * 0.35, py - pr * 0.4, pr * 0.32, pr * 0.32);
    shape(ctx, "#ffffff");
    ctx.restore();
  }
  if (shut > 0) {
    // the lid comes down from the top
    ctx.save();
    ellipse(ctx, x, y, rx, ry);
    ctx.clip();
    ctx.fillStyle = opts.lidColor || pal.body;
    ctx.fillRect(x - rx - 1, y - ry - 1, rx * 2 + 2, ry * 2 * shut + 1);
    ctx.restore();
    if (opts.ring) { ellipse(ctx, x, y, rx, ry); shape(ctx, null, pal.line, opts.ring); }
  }
}

// ---------------------------------------------------------------------------
// Side view helpers (facing +x, ground at y = 0, up is -y).

function sideLegs(ctx, s, pal, cfg, far) {
  const { hips, feet, a, b, stride, lift, width } = cfg;
  for (let j = 0; j < 3; j++) {
    const i = far ? 3 + j : j;
    const c = footCycle(s.phase, i, stride);
    const hx = hips[j][0] + (far ? -1.2 : 0), hy = hips[j][1] + s.bob;
    const fx = feet[j] + c.dx + (far ? -1.5 : 0), fy = -c.lift * lift;
    const k = knee(hx, hy, fx, fy, a, b, (k1, k2) => k1[1] < k2[1]);
    polyline(ctx, [[hx, hy], k, [fx, fy]], far ? cfg.farColor : cfg.color, width);
    if (cfg.paws) { ellipse(ctx, fx + 0.6, fy - 0.2, width * 0.7, width * 0.5); shape(ctx, far ? cfg.farColor : cfg.color); }
  }
}

function sideAntenna(ctx, s, x, y, cfg, pal, which) {
  const tap = Math.sin(s.time * cfg.tapSpeed + which * 1.9 + s.seed) * cfg.tap;
  const a0 = cfg.scapeAngle + tap * 0.4 + (which ? -0.12 : 0);
  const kx = x + Math.cos(a0) * cfg.scape, ky = y + Math.sin(a0) * cfg.scape;
  const a1 = cfg.tipAngle + tap + (which ? 0.15 : 0);
  const pts = [[x, y], [kx, ky]];
  if (cfg.curl) {
    // a soft curve: three short segments turning further each time
    let px = kx, py = ky, ang = a1;
    for (let n = 0; n < 3; n++) { px += Math.cos(ang) * cfg.tip / 3; py += Math.sin(ang) * cfg.tip / 3; ang += cfg.curl; pts.push([px, py]); }
  } else {
    pts.push([kx + Math.cos(a1) * cfg.tip, ky + Math.sin(a1) * cfg.tip]);
  }
  polyline(ctx, pts, which ? cfg.farColor : cfg.color, cfg.width);
  const end = pts[pts.length - 1];
  if (cfg.knob) { ellipse(ctx, end[0], end[1], cfg.knob, cfg.knob); shape(ctx, cfg.knobColor || cfg.color); }
}

function sideParcel(ctx, x, y, pal) {
  ctx.beginPath();
  ctx.roundRect(x, y - 4, 8, 7, 1.6);
  shape(ctx, pal.red);
  polyline(ctx, [[x + 4, y - 4], [x + 4, y + 3]], "#ffffff99", 0.9);
}

// ---------------------------------------------------------------------------
// Top view helpers (facing +x, y across the body).

function topLegs(ctx, s, cfg) {
  const { hips, feet, a, stride, width } = cfg;
  for (let i = 0; i < 6; i++) {
    const side = i < 3 ? -1 : 1, j = i % 3;
    const c = footCycle(s.phase, i, stride);
    const hx = hips[j][0], hy = hips[j][1] * side;
    // a lifted foot is pulled slightly in, which reads as "up" from above
    const fx = feet[j][0] + c.dx, fy = (feet[j][1] - c.lift * 1.6) * side;
    // Seen from above, the femur points outwards (tilted forward on the front
    // legs, back on the hind legs) and swings a little with the foot; the
    // tibia reaches the foot. Cleaner than IK, whose knees cross the body.
    const tilt = [0.55, 0, -0.6][j] + c.dx * 0.035;
    const th = side * (Math.PI / 2 - tilt);
    const k = [hx + Math.cos(th) * a, hy + Math.sin(th) * a];
    polyline(ctx, [[hx, hy], k, [fx, fy]], cfg.color, width);
  }
}

function topAntennae(ctx, s, x, y, cfg) {
  for (const side of [-1, 1]) {
    const tap = Math.sin(s.time * cfg.tapSpeed + (side > 0 ? 2.1 : 0) + s.seed) * cfg.tap;
    const a0 = side * (cfg.spread + tap * 0.3);
    const bx = x, by = y * side;
    const kx = bx + Math.cos(a0) * cfg.scape, ky = by + Math.sin(a0) * cfg.scape;
    const pts = [[bx, by], [kx, ky]];
    let ang = side * (cfg.bend + tap), px = kx, py = ky;
    const n = cfg.wave ? 4 : 1;
    for (let k = 0; k < n; k++) {
      const w = cfg.wave ? side * Math.sin(s.time * 6 + k * 1.4 + s.seed) * cfg.wave : 0;
      px += Math.cos(ang + w) * cfg.tip / n; py += Math.sin(ang + w) * cfg.tip / n;
      pts.push([px, py]);
    }
    polyline(ctx, pts, cfg.color, cfg.width);
    if (cfg.knob) { ellipse(ctx, px, py, cfg.knob, cfg.knob); shape(ctx, cfg.knobColor || cfg.color); }
  }
}

function topParcel(ctx, x, pal) {
  ctx.beginPath();
  ctx.roundRect(x, -4.5, 8, 9, 1.8);
  shape(ctx, pal.red);
  polyline(ctx, [[x, 0], [x + 8, 0]], "#ffffff99", 0.9);
}

// ---------------------------------------------------------------------------
// The characters. Each draws in its own units (about 40 long), facing +x;
// `height` is how far above the origin the name tag floats, `stride` the step.

const CHARACTERS = [
  {
    key: "A", name: "Pip", agent: "@assistant", view: "side", height: 34, stride: 6,
    desc: "Side view, chibi: a huge round head, big glossy eyes, curly antennae with red tips, short quick legs. Red only on the antenna tips (and on what it carries).",
    draw(ctx, s, pal) {
      const bob = s.bob;
      const legs = { hips: [[5, -9], [2.5, -9], [0, -9]], feet: [12, 3, -7], a: 7.5, b: 7.5, stride: this.stride, lift: 3.2, width: 1.9, color: pal.leg, farColor: pal.leg + "88", paws: true };
      sideLegs(ctx, s, pal, legs, true);
      const ant = { scapeAngle: -1.6, scape: 6, tipAngle: -0.6, tip: 9, curl: 0.55, tap: 0.35, tapSpeed: 5, width: 1.6, color: pal.leg, farColor: pal.leg + "99", knob: 1.9, knobColor: pal.red };
      sideAntenna(ctx, s, 16, -26 + bob, ant, pal, 1);
      // gaster, petiole, thorax
      ellipse(ctx, -10, -13 + bob * 0.6, 8.5, 7.5, -0.15); shape(ctx, pal.body, pal.line, 1.4);
      ellipse(ctx, -1.5, -11 + bob, 2, 2); shape(ctx, pal.body, pal.line, 1.2);
      ellipse(ctx, 3.5, -12 + bob, 5.5, 4.2); shape(ctx, pal.body, pal.line, 1.4);
      sideLegs(ctx, s, pal, legs, false);
      // head
      ellipse(ctx, 14, -19 + bob, 10, 9.5); shape(ctx, pal.body, pal.line, 1.4);
      eye(ctx, 20.5, -20 + bob, 2.6, 3.6, pal, s, { pupil: 0.62 });
      eye(ctx, 16.5, -20.5 + bob, 3.6, 4.6, pal, s, { pupil: 0.6 });
      ellipse(ctx, 18, -14.2 + bob, 2.2, 1.2); shape(ctx, pal.blush);
      sideAntenna(ctx, s, 17, -27 + bob, ant, pal, 0);
      if (s.carry) sideParcel(ctx, 23, -12 + bob, pal);
    },
  },
  {
    key: "B", name: "Sarge", agent: "@reviewer", view: "side", height: 36, stride: 11,
    desc: "Side view, the soldier: a big red head with mandibles, a half-closed, focused eye, straight elbowed antennae, a slim waist and long legs. Red is the whole head.",
    draw(ctx, s, pal) {
      const bob = s.bob * 0.7;
      const legs = { hips: [[6, -14], [3, -14], [0, -14]], feet: [16, 4, -12], a: 11, b: 12, stride: this.stride, lift: 4, width: 1.6, color: pal.leg, farColor: pal.leg + "88" };
      sideLegs(ctx, s, pal, legs, true);
      const ant = { scapeAngle: -1.95, scape: 8, tipAngle: -0.45, tip: 11, tap: 0.25, tapSpeed: 3.2, width: 1.5, color: pal.leg, farColor: pal.leg + "99" };
      sideAntenna(ctx, s, 18, -27 + bob, ant, pal, 1);
      ellipse(ctx, -13, -18 + bob, 9.5, 6.5, -0.25); shape(ctx, pal.body, pal.line, 1.3);
      ellipse(ctx, -2.5, -16 + bob, 1.8, 2.4); shape(ctx, pal.body, pal.line, 1.1);
      ellipse(ctx, 4, -16.5 + bob, 7, 3.4, -0.1); shape(ctx, pal.body, pal.line, 1.3);
      sideLegs(ctx, s, pal, legs, false);
      // mandibles
      polyline(ctx, [[22, -15 + bob], [26, -13 + bob], [24.5, -11 + bob]], pal.redLine, 1.8);
      // head: a rounded, slightly square red head
      ctx.beginPath(); ctx.roundRect(10, -27 + bob, 15, 13, 5.5); shape(ctx, pal.red, pal.line, 1.3);
      eye(ctx, 19.5, -21.5 + bob, 3, 3, pal, s, { lid: 0.42, lidColor: pal.red, pupil: 0.6 });
      polyline(ctx, [[16, -25.2 + bob], [22.5, -23.6 + bob]], pal.pupil, 1.4); // brow
      sideAntenna(ctx, s, 19, -27 + bob, ant, pal, 0);
      if (s.carry) sideParcel(ctx, 25, -13 + bob, pal);
    },
  },
  {
    key: "C", name: "Mark", agent: "@builder", view: "top", height: 22, stride: 8,
    desc: "Seen from above: the BrandMark come alive. The logo's proportions (round head, small thorax, long gaster) with big eyes added. All red, like the logo; the most on-brand and the loudest.",
    draw(ctx, s, pal) {
      const legs = { hips: [[3, 2.5], [0, 3], [-3, 2.5]], feet: [[13, 11], [0, 14], [-12, 12]], a: 8.5, b: 9.5, stride: this.stride, width: 2, color: pal.red };
      topLegs(ctx, s, legs);
      topAntennae(ctx, s, 17, 2.5, { spread: 0.6, scape: 6, bend: 0.12, tip: 8, tap: 0.3, tapSpeed: 4, width: 1.8, color: pal.red });
      const sway = Math.sin(s.phase * TAU * 2) * 0.05;
      ctx.save(); ctx.rotate(-sway);
      ellipse(ctx, -16, 0, 11, 8.2); shape(ctx, pal.red);
      ctx.restore();
      ellipse(ctx, 0, 0, 5.8, 4.6); shape(ctx, pal.red);
      ctx.save(); ctx.rotate(sway);
      ellipse(ctx, 13, 0, 7.6, 7.4); shape(ctx, pal.red);
      eye(ctx, 15.5, -3.4, 2.8, 2.6, pal, s, { lidColor: pal.red });
      eye(ctx, 15.5, 3.4, 2.8, 2.6, pal, s, { lidColor: pal.red });
      if (s.carry) { ctx.globalAlpha = 1; topParcelOutlined(ctx, 20, pal); }
      ctx.restore();
    },
  },
  {
    key: "D", name: "Doodle", agent: "@scribe", view: "top", height: 24, stride: 7,
    desc: "Seen from above, line art: a hand-drawn outline in the text colour, a very big head with ringed eyes, wavy antennae. No red on the body at all; the red is only ever what it carries.",
    draw(ctx, s, pal) {
      const legs = { hips: [[2, 2.5], [0, 3], [-2, 2.5]], feet: [[12, 11], [0, 13], [-11, 11]], a: 8, b: 8.5, stride: this.stride, width: 1.6, color: pal.line };
      topLegs(ctx, s, legs);
      topAntennae(ctx, s, 20, 3, { spread: 0.8, scape: 5, bend: 0.3, tip: 9, tap: 0.25, tapSpeed: 3, width: 1.5, color: pal.line, wave: 0.35, knob: 1.3 });
      const sway = Math.sin(s.phase * TAU * 2) * 0.06;
      ctx.save(); ctx.rotate(-sway);
      ellipse(ctx, -14, 0, 9.5, 7.5); shape(ctx, pal.bg, pal.line, 1.7);
      polyline(ctx, [[-17, -6], [-17, 6]], pal.line, 1.1);
      polyline(ctx, [[-12, -6.9], [-12, 6.9]], pal.line, 1.1);
      ctx.restore();
      ellipse(ctx, -2.2, 0, 2, 1.8); shape(ctx, pal.bg, pal.line, 1.5);
      ellipse(ctx, 2, 0, 3.6, 3.2); shape(ctx, pal.bg, pal.line, 1.6);
      ctx.save(); ctx.rotate(sway);
      ellipse(ctx, 13, 0, 9.5, 9); shape(ctx, pal.bg, pal.line, 1.7);
      eye(ctx, 16, -3.8, 3.2, 3.2, pal, s, { ring: 1.3, pupil: 0.45, lidColor: pal.bg });
      eye(ctx, 16, 3.8, 3.2, 3.2, pal, s, { ring: 1.3, pupil: 0.45, lidColor: pal.bg });
      if (s.carry) topParcel(ctx, 22, pal);
      ctx.restore();
    },
  },
  {
    key: "E", name: "Stripe", agent: "@homelab", view: "side", height: 33, stride: 8,
    desc: "Side view, a rounder, friendlier worker: a big bulbous gaster with one red band, oval eyes with eyebrows, long elbowed antennae that tap the ground ahead. Red is the band.",
    draw(ctx, s, pal) {
      const bob = s.bob;
      const legs = { hips: [[5, -11], [2.5, -11], [0, -11]], feet: [14, 3, -9], a: 9, b: 9.5, stride: this.stride, lift: 3.5, width: 1.8, color: pal.leg, farColor: pal.leg + "88", paws: true };
      sideLegs(ctx, s, pal, legs, true);
      const ant = { scapeAngle: -1.25, scape: 7, tipAngle: 0.35, tip: 10, tap: 0.4, tapSpeed: 4.2, width: 1.5, color: pal.leg, farColor: pal.leg + "99", knob: 1.1 };
      sideAntenna(ctx, s, 17, -22 + bob, ant, pal, 1);
      // gaster with the red band
      ctx.save();
      ellipse(ctx, -12, -15 + bob * 0.5, 11, 9, -0.2);
      ctx.fillStyle = pal.body; ctx.fill(); ctx.clip();
      ctx.fillStyle = pal.red; ctx.fillRect(-12.5, -30, 5, 30);
      ctx.restore();
      ellipse(ctx, -12, -15 + bob * 0.5, 11, 9, -0.2); shape(ctx, null, pal.line, 1.4);
      ellipse(ctx, 0, -13 + bob, 1.8, 1.8); shape(ctx, pal.body, pal.line, 1.2);
      ellipse(ctx, 4.5, -14 + bob, 5.5, 4.5); shape(ctx, pal.body, pal.line, 1.4);
      sideLegs(ctx, s, pal, legs, false);
      ellipse(ctx, 15, -17 + bob, 7.5, 7); shape(ctx, pal.body, pal.line, 1.4);
      eye(ctx, 20.2, -18 + bob, 1.9, 2.8, pal, s, { pupil: 0.7 });
      eye(ctx, 17.2, -18.3 + bob, 2.4, 3.3, pal, s, { pupil: 0.65 });
      polyline(ctx, [[14.8, -22.6 + bob], [19, -23.4 + bob]], pal.line, 1.2);
      polyline(ctx, [[20, -23.2 + bob], [22, -22.2 + bob]], pal.line, 1.2);
      polyline(ctx, [[19.5, -12.8 + bob], [21.5, -12.2 + bob]], pal.line, 1.1); // a small smile
      sideAntenna(ctx, s, 18, -23 + bob, ant, pal, 0);
      if (s.carry) sideParcel(ctx, 21.5, -11 + bob, pal);
    },
  },
];

// The all-red ant needs its parcel outlined, or it disappears into the head.
function topParcelOutlined(ctx, x, pal) {
  ctx.beginPath();
  ctx.roundRect(x, -4.5, 8, 9, 1.8);
  shape(ctx, pal.eye, pal.red, 1.4);
  polyline(ctx, [[x, 0], [x + 8, 0]], pal.red, 1);
}

// ---------------------------------------------------------------------------
// Page: one stage per canvas, one shared animation loop.

const state = { paused: false, carry: false, speed: 1 };
const stages = [];

function mount() {
  const tpl = document.getElementById("char-tpl");
  const main = document.getElementById("chars");
  CHARACTERS.forEach((ch, idx) => {
    const node = tpl.content.cloneNode(true);
    node.querySelector(".letter").textContent = ch.key;
    node.querySelector(".name").textContent = `${ch.name} (shown as ${ch.agent})`;
    node.querySelector(".desc").textContent = ch.desc;
    node.querySelectorAll(".stage").forEach((el, n) => {
      const tag = document.createElement("div");
      tag.className = "tag";
      tag.textContent = ch.agent;
      el.appendChild(tag);
      stages.push({
        el, tag, ch,
        canvas: el.querySelector("canvas"),
        theme: el.classList.contains("dark") ? "dark" : "light",
        mode: el.dataset.mode,
        scale: parseFloat(el.dataset.scale),
        x: (n * 97 + idx * 61) % 300, dist: 0, seed: idx * 1.3 + n * 0.7,
      });
    });
    main.appendChild(node);
  });
}

function resize() {
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  for (const st of stages) {
    const w = st.el.clientWidth, h = st.el.clientHeight;
    st.w = w; st.h = h; st.dpr = dpr;
    st.canvas.width = Math.round(w * dpr);
    st.canvas.height = Math.round(h * dpr);
  }
}

const SPEED = 46; // px per second at scale 1

function frame(st, dt, time) {
  const { ch, scale } = st;
  const pal = PALETTE[st.theme];
  const ctx = st.canvas.getContext("2d");
  const v = SPEED * scale * state.speed * (ch.name === "Sarge" ? 0.8 : ch.name === "Pip" ? 1.15 : 1);
  const move = state.paused ? 0 : v * dt;
  st.dist += move / scale; // in ant units
  const s = {
    time, seed: st.seed, carry: state.carry,
    phase: (st.dist / (2 * ch.stride)) % 1,
  };
  s.bob = ch.view === "side" ? -Math.abs(Math.sin(s.phase * TAU)) * 0.8 : 0;

  ctx.setTransform(st.dpr, 0, 0, st.dpr, 0, 0);
  ctx.clearRect(0, 0, st.w, st.h);

  let x, y, heading = 0;
  if (st.mode === "treadmill") {
    x = st.w / 2 - 4 * scale;
    y = ch.view === "side" ? st.h * 0.74 : st.h * 0.5;
    // ground marks scroll with the walked distance: planted feet must stay on them
    ctx.fillStyle = pal.ground;
    const gap = 14 * scale, off = (st.dist * scale) % gap;
    const gy = ch.view === "side" ? y + 2 : st.h - 24;
    for (let gx = -off; gx < st.w + gap; gx += gap) ctx.fillRect(gx, gy, gap * 0.45, 3);
    if (ch.view === "side") ctx.fillRect(0, y, st.w, 1);
  } else {
    st.x += move;
    const span = st.w + 140 * scale;
    if (st.x > span) st.x -= span;
    x = st.x - 70 * scale;
    if (ch.view === "side") {
      y = st.h * 0.78;
      ctx.fillStyle = pal.ground; ctx.fillRect(0, y, st.w, 1);
    } else {
      // seen from above, the path curves, so the turning shows
      const k = 0.012 / scale, amp = st.h * 0.16;
      y = st.h * 0.58 + Math.sin(st.x * k) * amp;
      heading = Math.atan(Math.cos(st.x * k) * amp * k);
    }
  }

  ctx.translate(x, y);
  ctx.rotate(heading);
  ctx.scale(scale, scale);
  ch.draw(ctx, s, pal);

  const tw = st.tag.offsetWidth, th = st.tag.offsetHeight;
  const ty = y - ch.height * scale - th - 6;
  st.tag.style.transform = `translate(${Math.round(x + 4 * scale - tw / 2)}px, ${Math.round(ty)}px)`;
}

function setup() {
  mount();
  resize();
  window.addEventListener("resize", resize);
  let last = performance.now();
  const loop = (now) => {
    const dt = Math.min(0.05, (now - last) / 1000);
    last = now;
    const time = now / 1000;
    for (const st of stages) frame(st, dt, time);
    if (!document.hidden) requestAnimationFrame(loop);
  };
  requestAnimationFrame(loop);
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden) { last = performance.now(); requestAnimationFrame(loop); }
  });

  const pause = document.getElementById("pause");
  pause.addEventListener("click", () => {
    state.paused = !state.paused;
    pause.setAttribute("aria-pressed", String(state.paused));
    pause.textContent = state.paused ? "Resume" : "Pause";
  });
  const carry = document.getElementById("carry");
  carry.addEventListener("click", () => {
    state.carry = !state.carry;
    carry.setAttribute("aria-pressed", String(state.carry));
  });
  const speed = document.getElementById("speed");
  const speeds = [[1, "normal"], [0.3, "slow (to check the gait)"], [2, "fast"]];
  speed.addEventListener("click", () => {
    const i = (speeds.findIndex(([v]) => v === state.speed) + 1) % speeds.length;
    state.speed = speeds[i][0];
    speed.textContent = `Speed: ${speeds[i][1]}`;
  });
  // For screenshots: ?carry=1 starts with the parcel on.
  const q = new URLSearchParams(location.search);
  if (q.get("carry") === "1") carry.click();
}

setup();
