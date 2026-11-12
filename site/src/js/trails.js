// Pheromone trails (docs/18, 4.3): when an agent hands a task to another, a
// faintly glowing red trail runs between the two, then fades. The only loud
// use of the red in the chambers. Stopping the colony puts every trail out.
import { addLayer } from "./colony.js";
import { motion } from "./motion.js";

const trails = new Set();
let red = "#d42a2e";

/**
 * Lays a trail between two moving things. from/to: () => {x, y} in document
 * coordinates (an ant, a message). Lives for `life` seconds of running time.
 */
export function lay(from, to, life = 4.5) {
  red = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || red;
  trails.add({ from, to, life, age: 0 });
  ensure();
}

let remove = null;
function ensure() {
  if (!remove) remove = addLayer(draw);
}

function draw(ctx, sy, time, dt) {
  if (motion.stopped) trails.clear();
  for (const tr of [...trails]) {
    tr.age += dt;
    if (tr.age >= tr.life) { trails.delete(tr); continue; }
    const a = tr.from(), b = tr.to();
    if (!a || !b) continue;
    const k = tr.age / tr.life;
    const alpha = k < 0.15 ? k / 0.15 : 1 - (k - 0.15) / 0.85;
    const mx = (a.x + b.x) / 2, my = (a.y + b.y) / 2;
    const dx = b.x - a.x, dy = b.y - a.y, len = Math.hypot(dx, dy) || 1;
    const bend = Math.min(80, len * 0.25);
    const cx = mx - (dy / len) * bend, cy = my + (dx / len) * bend - sy;
    ctx.save();
    ctx.globalAlpha = Math.max(0, alpha) * 0.85;
    ctx.strokeStyle = red;
    ctx.shadowColor = red;
    ctx.shadowBlur = 10;
    ctx.lineWidth = 2.5;
    ctx.lineCap = "round";
    ctx.setLineDash([2, 7]);
    ctx.lineDashOffset = -time * 22; // the droplets drift towards the receiver
    ctx.beginPath();
    ctx.moveTo(a.x, a.y - sy);
    ctx.quadraticCurveTo(cx, cy, b.x, b.y - sy);
    ctx.stroke();
    ctx.restore();
  }
  if (!trails.size && remove) { remove(); remove = null; }
  return trails.size > 0;
}

/** Document-coordinate anchor at the centre of an element. */
export function anchor(el) {
  return () => {
    const r = el.getBoundingClientRect();
    return { x: r.left + r.width / 2, y: r.top + r.height / 2 + scrollY };
  };
}
