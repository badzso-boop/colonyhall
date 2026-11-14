// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// One ant: the "Mark" character chosen at the design gate (docs/18, 12),
// drawn in Canvas 2D from code. Seen from above, the BrandMark's proportions
// with big eyes; legs follow an alternating-tripod gait whose phase advances
// with the distance walked, so planted feet do not slide.

const TAU = Math.PI * 2;
const TRIPOD = [0, 0.5, 0, 0.5, 0, 0.5]; // L1 L2 L3 R1 R2 R3: L1 R2 L3 move together
const STRIDE = 8; // ant units
const HIPS = [[3, 2.5], [0, 3], [-3, 2.5]];
const FEET = [[13, 11], [0, 14], [-12, 12]];
const FEMUR = 8.5;
const TILT = [0.55, 0, -0.6];

/** Personalities (docs/18, 4.1): speed in ant lengths per second, how often it stops. */
export const PERSONALITY = {
  assistant: { speed: 2.4, jitter: 0.25, pauseRate: 0.08, pauseFor: [250, 600] }, // quick and nervous
  reviewer: { speed: 1.2, jitter: 0.04, pauseRate: 0.3, pauseFor: [900, 1600], looks: true }, // slow, stops to look
  scribe: { speed: 1.6, jitter: 0.08, pauseRate: 0.06, pauseFor: [400, 800], note: true }, // carries a note
  builder: { speed: 1.8, jitter: 0.1, pauseRate: 0.05, pauseFor: [300, 700] },
  homelab: { speed: 1.6, jitter: 0.12, pauseRate: 0.4, pauseFor: [500, 1100], sniffs: true }, // keeps stopping to sniff
};
export const AGENTS = Object.keys(PERSONALITY);
const ANT_LENGTH = 50; // units from gaster tip to antenna tip, roughly

function footCycle(t, i) {
  const p = (t + TRIPOD[i]) % 1;
  if (p < 0.5) return { dx: STRIDE * (0.5 - p * 2), lift: 0 };
  const q = (p - 0.5) * 2;
  return { dx: STRIDE * (q * q * (3 - 2 * q) - 0.5), lift: Math.sin(Math.PI * q) };
}

function line(ctx, pts, color, width) {
  ctx.beginPath();
  ctx.moveTo(pts[0][0], pts[0][1]);
  for (let i = 1; i < pts.length; i++) ctx.lineTo(pts[i][0], pts[i][1]);
  ctx.strokeStyle = color;
  ctx.lineWidth = width;
  ctx.stroke();
}
function oval(ctx, x, y, rx, ry, fill) {
  ctx.beginPath();
  ctx.ellipse(x, y, rx, ry, 0, 0, TAU);
  ctx.fillStyle = fill;
  ctx.fill();
}

/**
 * Draws an ant at the origin, facing +x, in ant units.
 * s: { phase, time, seed, carry, note, wave, look }
 * pal: { red, eye, pupil, paper }
 */
export function drawAnt(ctx, s, pal) {
  ctx.lineCap = "round";
  ctx.lineJoin = "round";
  // legs
  for (let i = 0; i < 6; i++) {
    const side = i < 3 ? -1 : 1, j = i % 3;
    let c = footCycle(s.phase, i);
    let fx = FEET[j][0] + c.dx, fy = (FEET[j][1] - c.lift * 1.6) * side;
    if (s.wave && i === 3) {
      // waving: the right front leg is raised and swings
      const w = Math.sin(s.time * 12);
      fx = 18 + w * 3; fy = 15 + w * 2; c = { dx: 3 };
    }
    const th = side * (Math.PI / 2 - (TILT[j] + c.dx * 0.035));
    const hx = HIPS[j][0], hy = HIPS[j][1] * side;
    line(ctx, [[hx, hy], [hx + Math.cos(th) * FEMUR, hy + Math.sin(th) * FEMUR], [fx, fy]], pal.red, 2);
  }
  // antennae: elbowed, tapping
  for (const side of [-1, 1]) {
    const tap = Math.sin(s.time * 4 + (side > 0 ? 2.1 : 0) + s.seed) * 0.3;
    const a0 = side * (0.6 + tap * 0.3), a1 = side * (0.12 + tap);
    const bx = 17, by = 2.5 * side;
    const kx = bx + Math.cos(a0) * 6, ky = by + Math.sin(a0) * 6;
    line(ctx, [[bx, by], [kx, ky], [kx + Math.cos(a1) * 8, ky + Math.sin(a1) * 8]], pal.red, 1.8);
  }
  const sway = Math.sin(s.phase * TAU * 2) * 0.05;
  ctx.save(); ctx.rotate(-sway); oval(ctx, -16, 0, 11, 8.2, pal.red); ctx.restore();
  oval(ctx, 0, 0, 5.8, 4.6, pal.red);
  ctx.save(); ctx.rotate(sway);
  oval(ctx, 13, 0, 7.6, 7.4, pal.red);
  // eyes, with a blink every few seconds
  const bp = (s.time + s.seed * 3.7) % 4.2;
  const shut = bp < 0.12 ? 1 - Math.abs(bp - 0.06) / 0.06 : 0;
  for (const side of [-1, 1]) {
    const ex = 15.5, ey = 3.4 * side;
    oval(ctx, ex, ey, 2.8, 2.6 * (1 - shut * 0.9), pal.eye);
    if (shut < 0.6) {
      const lx = s.look ? s.look[0] : 1, ly = s.look ? s.look[1] : 0;
      oval(ctx, ex + 0.4 + lx * 0.9, ey + ly * 0.9, 1.5, 1.5, pal.pupil);
      oval(ctx, ex + 0.9 + lx * 0.9, ey - 0.5 + ly * 0.9, 0.5, 0.5, pal.eye);
    }
  }
  if (s.carry || s.note) {
    // a white parcel (or a note) with a red outline, so it stands out from the red head
    ctx.beginPath();
    ctx.roundRect(20, -4.5, 8, 9, 1.8);
    ctx.fillStyle = pal.paper; ctx.fill();
    ctx.strokeStyle = pal.red; ctx.lineWidth = 1.4; ctx.stroke();
    if (s.note) { line(ctx, [[22, -2], [26, -2]], pal.red, 0.8); line(ctx, [[22, 0.5], [26, 0.5]], pal.red, 0.8); line(ctx, [[22, 3], [25, 3]], pal.red, 0.8); }
    else line(ctx, [[20, 0], [28, 0]], pal.red, 1);
  }
  ctx.restore();
}

/** A polyline in document coordinates, walked by distance. */
export class Path {
  constructor(points) {
    this.points = points;
    this.cum = [0];
    for (let i = 1; i < points.length; i++) {
      this.cum.push(this.cum[i - 1] + Math.hypot(points[i].x - points[i - 1].x, points[i].y - points[i - 1].y));
    }
    this.length = this.cum[this.cum.length - 1];
  }
  at(d) {
    const { points: p, cum } = this;
    if (d <= 0) return { x: p[0].x, y: p[0].y, angle: Math.atan2(p[1].y - p[0].y, p[1].x - p[0].x) };
    let i = 1;
    while (i < cum.length - 1 && cum[i] < d) i++;
    const seg = cum[i] - cum[i - 1] || 1;
    const k = Math.min(1, (d - cum[i - 1]) / seg);
    const a = p[i - 1], b = p[i];
    return { x: a.x + (b.x - a.x) * k, y: a.y + (b.y - a.y) * k, angle: Math.atan2(b.y - a.y, b.x - a.x) };
  }
  /** Samples an SVG path element into document coordinates. */
  static fromSVG(pathEl, step = 6, reverse = false) {
    const total = pathEl.getTotalLength();
    const m = pathEl.getScreenCTM();
    const pts = [];
    for (let d = 0; d <= total; d += step) {
      const q = pathEl.getPointAtLength(d);
      pts.push({ x: m.a * q.x + m.c * q.y + m.e + scrollX, y: m.b * q.x + m.d * q.y + m.f + scrollY });
    }
    if (reverse) pts.reverse();
    return new Path(pts);
  }
}

const rand = (a, b) => a + Math.random() * (b - a);
const angleLerp = (a, b, k) => a + Math.atan2(Math.sin(b - a), Math.cos(b - a)) * k;

export class Ant {
  constructor({ agent, path, scale = 1, carry = false, speed, onDone, seed }) {
    this.agent = agent;
    this.p = PERSONALITY[agent] || PERSONALITY.builder;
    this.path = path;
    this.scale = scale;
    this.carry = carry;
    this.speedMul = speed ?? 1;
    this.onDone = onDone;
    this.seed = seed ?? Math.random() * 10;
    this.d = 0;
    const s = path.at(0);
    this.x = s.x; this.y = s.y; this.angle = s.angle;
    this.phase = Math.random();
    this.offset = 0; // sideways, away from the cursor
    this.pauseLeft = 0;
    this.target = null; // { x, y, onArrive } when it leaves its path
    this.resume = null; // where to go back to on the path
    this.wave = 0; // seconds of waving left
    this.look = null;
    this.done = false;
    this.lastSeen = performance.now();
  }

  get speed() { return this.p.speed * ANT_LENGTH * this.scale * this.speedMul; }

  /** Leaves the path to walk to a point (a crumb, the cursor), then calls onArrive. */
  goTo(x, y, onArrive) {
    if (!this.target) this.resume = this.d;
    this.target = { x, y, onArrive };
  }

  update(dt, ctx) {
    if (this.grow) {
      // hatching: the ant grows to its size
      this.grow.t = Math.min(1, this.grow.t + dt / this.grow.dur);
      const e = 1 - (1 - this.grow.t) ** 3;
      this.scale = this.grow.from + (this.grow.to - this.grow.from) * e;
      if (this.grow.t >= 1) this.grow = null;
    }
    if (this.wave > 0) { this.wave -= dt; return; }
    if (this.pauseLeft > 0) { this.pauseLeft -= dt; return; }
    let move = this.speed * dt * (1 + Math.sin(ctx.time * 9 + this.seed) * this.p.jitter);

    if (this.target) {
      const dx = this.target.x - this.x, dy = this.target.y - this.y, dist = Math.hypot(dx, dy);
      const stopAt = 22 * this.scale; // the head reaches the point
      if (dist <= stopAt) {
        const t = this.target;
        this.target = null;
        t.onArrive?.(this);
        return;
      }
      move = Math.min(move, dist - stopAt);
      const want = Math.atan2(dy, dx);
      this.angle = angleLerp(this.angle, want, Math.min(1, dt * 8));
      this.x += Math.cos(this.angle) * move;
      this.y += Math.sin(this.angle) * move;
      this.phase = (this.phase + move / (2 * STRIDE * this.scale)) % 1;
      return;
    }
    if (this.resume !== null) {
      // walk back to where it left the path
      const r = this.path.at(this.resume);
      const dx = r.x - this.x, dy = r.y - this.y, dist = Math.hypot(dx, dy);
      if (dist < 4) { this.resume = null; }
      else {
        move = Math.min(move, dist);
        const want = Math.atan2(dy, dx);
        this.angle = angleLerp(this.angle, want, Math.min(1, dt * 8));
        this.x += Math.cos(this.angle) * move;
        this.y += Math.sin(this.angle) * move;
        this.phase = (this.phase + move / (2 * STRIDE * this.scale)) % 1;
        return;
      }
    }

    // a kept ant (in a scene) waits at the end of its path until given a new one
    if (this.keep && !this.onDone && this.d >= this.path.length) return;

    // random stops: sniffing or looking
    if (Math.random() < this.p.pauseRate * dt) {
      this.pauseLeft = rand(...this.p.pauseFor) / 1000;
      if (this.p.looks) this.look = [Math.random() - 0.3, Math.random() * 2 - 1];
      return;
    }
    if (this.pauseLeft <= 0) this.look = null;

    this.d += move;
    if (this.d >= this.path.length) { this.done = true; this.onDone?.(this); return; }
    const pt = this.path.at(this.d);
    // keep away from the cursor: shift sideways along the path's normal
    let want = 0;
    if (ctx.cursor) {
      const cx = ctx.cursor.x - pt.x, cy = ctx.cursor.y - pt.y, cd = Math.hypot(cx, cy);
      const r = 70 * this.scale;
      if (cd < r) {
        const nx = -Math.sin(pt.angle), ny = Math.cos(pt.angle);
        const sideOf = cx * nx + cy * ny > 0 ? -1 : 1;
        want = sideOf * (r - cd) * 0.9;
      }
    }
    this.offset += (want - this.offset) * Math.min(1, dt * 6);
    const nx = -Math.sin(pt.angle), ny = Math.cos(pt.angle);
    const px = pt.x + nx * this.offset, py = pt.y + ny * this.offset;
    const realMove = Math.hypot(px - this.x, py - this.y);
    this.x = px; this.y = py;
    this.angle = angleLerp(this.angle, pt.angle, Math.min(1, dt * 7));
    this.phase = (this.phase + realMove / (2 * STRIDE * this.scale)) % 1;
  }

  draw(ctx, pal, time, scrollY) {
    ctx.save();
    ctx.translate(this.x, this.y - scrollY);
    ctx.rotate(this.angle);
    ctx.scale(this.scale, this.scale);
    drawAnt(ctx, { phase: this.phase, time, seed: this.seed, carry: this.carry, note: this.p.note && !this.carry, wave: this.wave > 0, look: this.look }, pal);
    ctx.restore();
  }
}
