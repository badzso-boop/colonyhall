// The colony: every ant on the page, one canvas, one requestAnimationFrame
// loop, and the name tags as DOM text (docs/18, 4.1 and 8). The loop runs
// only while an ant (or a layer, such as a pheromone trail) is on screen,
// and never while the tab is hidden, the visitor has paused motion, or the
// colony is stopped at the gate.
import { Ant, Path, AGENTS } from "./ants.js";
import { motion } from "./motion.js";
import { t } from "./i18n.js";

const canvas = document.getElementById("ants");
const tags = document.getElementById("ant-tags");
const ctx = canvas.getContext("2d");
const root = document.documentElement;

const ants = new Set();
const crumbs = new Set();
const layers = new Set(); // fn(ctx, scrollY, time, dt) -> true while it wants frames
let running = false;
let last = 0;
let dpr = 1;
let pal = palette();
let cursor = null; // document coordinates, mouse only
let cursorTimer = 0;
let lastSpawn = 0;
let nextGap = 2500;
let agentTurn = Math.floor(Math.random() * AGENTS.length);

const phone = () => innerWidth < 640;
export const limits = () => ({ max: phone() ? 5 : 12, scale: phone() ? 0.7 : 1 });

function palette() {
  const cs = getComputedStyle(root);
  return { red: cs.getPropertyValue("--accent").trim() || "#c1121f", eye: "#ffffff", pupil: "#0b0a0a", paper: "#ffffff" };
}

function resize() {
  dpr = Math.min(devicePixelRatio || 1, 2);
  canvas.width = Math.round(innerWidth * dpr);
  canvas.height = Math.round(innerHeight * dpr);
  draw(performance.now() / 1000);
}

export const onScreen = (y, margin = 80) => y > scrollY - margin && y < scrollY + innerHeight + margin;
const anyVisible = () => [...ants].some((a) => !a.hidden && onScreen(a.y));

/** The next agent that has no ant on the page (a name is one ant), or null. */
export function nextAgent() {
  const present = new Set([...ants].map((a) => a.agent));
  for (let i = 0; i < AGENTS.length; i++) {
    agentTurn = (agentTurn + 1) % AGENTS.length;
    if (!present.has(AGENTS[agentTurn])) return AGENTS[agentTurn];
  }
  return null;
}

/** Puts an ant on the page. opts: { agent, path, carry, speed, scale, onDone, still } */
export function spawn(opts) {
  const { scale } = limits();
  const ant = new Ant({ scale, ...opts, agent: opts.agent || nextAgent() });
  ant.still = !!opts.still;
  ant.untagged = !!opts.untagged;
  const tag = document.createElement("div");
  tag.className = "ant-tag";
  tag.style.visibility = "hidden"; // until its first frame places it
  // a hatchling has no name yet: no tag
  if (opts.untagged) tag.style.display = "none";
  else tag.textContent = t(`agents.${ant.agent}`); // i18n-prefix: agents
  tags.appendChild(tag);
  ant.tag = tag;
  ants.add(ant);
  wake();
  return ant;
}

/**
 * Calls in an agent's ant for a scene (the monitor, the gate). A name is one
 * ant: if it is already wandering on screen it comes from where it is;
 * otherwise a new one starts at `start`. The ant is kept (not dropped when
 * off screen) until the scene removes it.
 */
export function recruit(agent, start) {
  let ant = [...ants].find((a) => a.agent === agent && !a.keep && !a.still && onScreen(a.y));
  if (ant) {
    ant.target = null;
    ant.resume = null;
  } else {
    for (const a of [...ants]) if (a.agent === agent && !a.keep) remove(a);
    ant = spawn({ agent, path: new Path([start, { x: start.x + 1, y: start.y }]) });
    ant.x = start.x; ant.y = start.y;
  }
  ant.keep = true;
  return ant;
}

/** Walks an ant along a new path; resolves at its end (holding while frozen). */
export function walk(ant, path) {
  return new Promise((resolve) => {
    ant.path = path; ant.d = 0; ant.done = false; ant.resume = null;
    ant.onDone = (a) => { a.done = false; a.onDone = null; resolve(); };
    wake();
  });
}

export function remove(ant) {
  ants.delete(ant);
  ant.tag?.remove();
}

export function all() { return [...ants]; }

/** A drawing layer under the ants; it keeps the loop running while it returns true. */
export function addLayer(fn) { layers.add(fn); wake(); return () => layers.delete(fn); }

/** Starts the loop if anything should move, or draws one still frame. */
export function wake() {
  if (running) return;
  if (motion.still || motion.stopped || document.hidden) { draw(performance.now() / 1000); return; }
  running = true;
  last = performance.now();
  requestAnimationFrame(frame);
}

function frame(now) {
  const dt = Math.min(0.05, (now - last) / 1000);
  last = now;
  const time = now / 1000;
  if (motion.still || motion.stopped || document.hidden) { running = false; draw(time); return; }
  const c = { time, cursor };
  for (const a of [...ants]) {
    if (a.still || a.hidden) continue;
    a.update(dt, c);
    if (a.done) { remove(a); continue; }
    if (onScreen(a.y)) a.lastSeen = now;
    else if (now - a.lastSeen > 4000 && !a.keep) remove(a);
  }
  const wantLayers = draw(time, dt);
  if (anyVisible() || wantLayers) requestAnimationFrame(frame);
  else running = false;
}

function draw(time, dt = 0) {
  const sy = scrollY;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, innerWidth, innerHeight);
  let want = false;
  for (const l of layers) if (l(ctx, sy, time, dt)) want = true;
  for (const c of crumbs) {
    if (!onScreen(c.y)) continue;
    ctx.save();
    ctx.translate(c.x, c.y - sy);
    ctx.beginPath(); ctx.roundRect(-4, -4, 8, 8, 2);
    ctx.fillStyle = pal.paper; ctx.fill();
    ctx.strokeStyle = pal.red; ctx.lineWidth = 1.5; ctx.stroke();
    ctx.restore();
  }
  for (const a of ants) {
    const vis = !a.hidden && onScreen(a.y, 40);
    a.tag.style.visibility = vis ? "visible" : "hidden";
    if (!vis) continue;
    a.draw(ctx, pal, motion.still || motion.stopped ? 0 : time, sy);
    if (a.untagged) continue;
    const w = a.tag.offsetWidth, h = a.tag.offsetHeight;
    a.tag.style.transform = `translate(${Math.round(a.x - w / 2)}px, ${Math.round(a.y - sy - 30 * a.scale - h - 4)}px)`;
  }
  return want;
}

// --- routes for the ants that wander in while you scroll -------------------

const rand = (a, b) => a + Math.random() * (b - a);

/** Across the screen, gently winding, from one edge to the other. */
export function crossRoute(yDoc, leftToRight = Math.random() < 0.5, amp = 18) {
  const pts = [];
  const x0 = -60, x1 = innerWidth + 60, k = rand(0.006, 0.014), ph = rand(0, 6);
  for (let x = x0; x <= x1; x += 12) pts.push({ x, y: yDoc + Math.sin(x * k + ph) * amp });
  if (!leftToRight) pts.reverse();
  return new Path(pts);
}

function visible(selector) {
  return [...document.querySelectorAll(selector)].filter((el) => {
    const r = el.getBoundingClientRect();
    return r.bottom > 40 && r.top < innerHeight - 40 && r.width > 0;
  });
}

/** Up the monitor's stand, over the screen, and off to the side. */
function monitorRoute() {
  const m = document.querySelector(".monitor__screen");
  if (!m) return null;
  const r = m.getBoundingClientRect();
  if (r.bottom < 0 || r.top > innerHeight) return null;
  const sy = scrollY, fromLeft = Math.random() < 0.5;
  const base = r.bottom + sy + 30, midX = r.left + r.width / 2;
  const edge = fromLeft ? -60 : innerWidth + 60, other = fromLeft ? innerWidth + 60 : -60;
  const pts = [
    { x: edge, y: base + 10 },
    { x: midX + (fromLeft ? -20 : 20), y: base + 8 },
    { x: midX, y: r.bottom + sy - 6 },
    { x: midX + (fromLeft ? 1 : -1) * r.width * 0.12, y: r.top + sy + r.height * 0.55 },
    { x: (fromLeft ? r.right : r.left) - (fromLeft ? 30 : -30), y: r.top + sy + 8 },
    { x: other, y: r.top + sy - 30 },
  ];
  return smooth(pts);
}

/** Along a tunnel between two chambers. */
function tunnelRoute() {
  const ts = visible(".tunnel path");
  if (!ts.length) return null;
  const p = ts[Math.floor(Math.random() * ts.length)];
  return Path.fromSVG(p, 6, Math.random() < 0.4);
}

/** Past a chamber's title. */
function titleRoute() {
  const hs = visible(".chamber h2");
  if (!hs.length) return null;
  const h = hs[Math.floor(Math.random() * hs.length)].getBoundingClientRect();
  return crossRoute(h.bottom + scrollY + 10, Math.random() < 0.5, 6);
}

/** Catmull-Rom through the points, sampled every few pixels. */
export function smooth(pts, step = 6) {
  const out = [];
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] || pts[i], p1 = pts[i], p2 = pts[i + 1], p3 = pts[i + 2] || p2;
    const n = Math.max(2, Math.ceil(Math.hypot(p2.x - p1.x, p2.y - p1.y) / step));
    for (let k = 0; k < n; k++) {
      const t = k / n, t2 = t * t, t3 = t2 * t;
      out.push({
        x: 0.5 * (2 * p1.x + (-p0.x + p2.x) * t + (2 * p0.x - 5 * p1.x + 4 * p2.x - p3.x) * t2 + (-p0.x + 3 * p1.x - 3 * p2.x + p3.x) * t3),
        y: 0.5 * (2 * p1.y + (-p0.y + p2.y) * t + (2 * p0.y - 5 * p1.y + 4 * p2.y - p3.y) * t2 + (-p0.y + 3 * p1.y - 3 * p2.y + p3.y) * t3),
      });
    }
  }
  out.push(pts[pts.length - 1]);
  return new Path(out);
}

function ambient() {
  if (motion.still || motion.stopped || document.hidden) return;
  const now = performance.now();
  const visibleAnts = [...ants].filter((a) => onScreen(a.y)).length;
  if (now - lastSpawn < nextGap || visibleAnts >= Math.min(limits().max, phone() ? 2 : 4) || ants.size >= limits().max) return;
  const picks = [monitorRoute, tunnelRoute, titleRoute, () => crossRoute(scrollY + rand(0.25, 0.85) * innerHeight)];
  const order = picks.sort(() => Math.random() - 0.5);
  let path = null;
  for (const p of order) { path = p(); if (path) break; }
  if (!path) return;
  const agent = nextAgent();
  if (!agent) return;
  lastSpawn = now;
  nextGap = rand(2500, 6500);
  spawn({ path, agent });
}

// --- cursor and clicks -------------------------------------------------------

function nearest(x, y, maxDist, filter = () => true) {
  let best = null, bd = maxDist;
  for (const a of ants) {
    if (a.still || !filter(a)) continue;
    const d = Math.hypot(a.x - x, a.y - y);
    if (d < bd && onScreen(a.y)) { bd = d; best = a; }
  }
  return best;
}

addEventListener("pointermove", (e) => {
  if (e.pointerType !== "mouse") return;
  cursor = { x: e.clientX, y: e.clientY + scrollY };
  clearTimeout(cursorTimer);
  // a curious ant comes over when the cursor rests
  cursorTimer = setTimeout(() => {
    if (motion.still || motion.stopped || !cursor) return;
    const a = nearest(cursor.x, cursor.y, 360, (x) => !x.target && !x.carry);
    if (!a) return;
    const at = { ...cursor };
    a.goTo(at.x, at.y, (ant) => { ant.pauseLeft = 1.4; ant.look = [1, 0]; });
    wake();
  }, 1300);
}, { passive: true });
addEventListener("pointerleave", () => { cursor = null; });
document.addEventListener("mouseleave", () => { cursor = null; });

/** Where a crumb is taken: the nearest tunnel opening, or off the nearest edge. */
function burrow(x, y) {
  let best = null, bd = innerHeight * 0.9;
  for (const el of document.querySelectorAll(".tunnel, .monitor__burrow")) {
    const r = el.getBoundingClientRect();
    const cx = r.left + r.width / 2, cy = r.top + scrollY + Math.min(r.height / 2, 30);
    const d = Math.hypot(cx - x, cy - y);
    if (d < bd) { bd = d; best = { x: cx, y: cy }; }
  }
  return best || { x: x < innerWidth / 2 ? -60 : innerWidth + 60, y };
}

document.addEventListener("click", (e) => {
  if (motion.still || motion.stopped) return;
  if (e.target.closest("a, button, input, textarea, select, label, summary, [tabindex], .monitor, .card, pre, code")) return;
  if (getSelection()?.toString()) return;
  const crumb = { x: e.clientX, y: e.clientY + scrollY };
  crumbs.add(crumb);
  let a = nearest(crumb.x, crumb.y, innerWidth, (x) => !x.carry);
  if (!a) {
    const agent = nextAgent();
    if (ants.size >= limits().max || !agent) { crumbs.delete(crumb); return; }
    a = spawn({ agent, path: crossRoute(crumb.y, crumb.x <= innerWidth / 2, 0) });
  }
  a.goTo(crumb.x, crumb.y, (ant) => {
    crumbs.delete(crumb);
    ant.carry = true;
    ant.pauseLeft = 0.3;
    const hole = burrow(ant.x, ant.y);
    ant.goTo(hole.x, hole.y, (x) => remove(x));
  });
  wake();
});

// --- the reduced-motion colony: ants standing still, with their names -------

let posed = [];
export function pose() {
  for (const a of posed) remove(a);
  posed = [];
  if (!motion.reduced) return;
  const { scale } = limits();
  const spots = [];
  const mon = document.querySelector(".monitor__screen");
  if (mon) {
    const r = mon.getBoundingClientRect();
    spots.push({ x: r.left + r.width * 0.7, y: r.top + scrollY - 2 * scale - 8, angle: Math.PI });
  }
  for (const h of document.querySelectorAll(".chamber h2")) {
    // right after the title's text, not after its (full-width) box
    const range = document.createRange();
    range.selectNodeContents(h);
    const rs = [...range.getClientRects()];
    const r = rs[rs.length - 1] || h.getBoundingClientRect();
    spots.push({ x: Math.min(innerWidth - 50, r.right + 46 * scale), y: r.top + scrollY + r.height / 2 + 4, angle: Math.PI * 0.95 });
  }
  for (const s of spots.slice(0, phone() ? 5 : 12)) {
    const a = spawn({ path: new Path([{ x: s.x, y: s.y }, { x: s.x + 1, y: s.y }]), still: true });
    a.angle = s.angle;
    a.keep = true;
    posed.push(a);
  }
  draw(0);
}

// --- wiring --------------------------------------------------------------------

let scrollQueued = false;
addEventListener("scroll", () => {
  if (scrollQueued) return;
  scrollQueued = true;
  requestAnimationFrame(() => {
    scrollQueued = false;
    if (!running) draw(performance.now() / 1000);
    ambient();
    if (anyVisible()) wake();
  });
}, { passive: true });
addEventListener("resize", () => { resize(); if (motion.reduced) pose(); });
document.addEventListener("visibilitychange", () => { if (!document.hidden) wake(); });
motion.subscribe(() => { pal = palette(); if (motion.reduced && !posed.length) pose(); if (!motion.reduced && posed.length) pose(); wake(); });
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => { pal = palette(); wake(); });
new MutationObserver(() => { pal = palette(); wake(); }).observe(root, { attributes: true, attributeFilter: ["data-theme"] });

export function start() {
  resize();
  if (motion.reduced) pose();
  else setTimeout(() => { lastSpawn = 0; ambient(); }, 1200);
  setInterval(ambient, 1500);
}
