// The tunnel map (docs/18, 4.5) and the workrooms (docs/18, 3, chamber 4).
// The map's tunnels are dug as it scrolls in; then ants carry a message
// from the hall (the server) to an outpost and a reply back. In the
// homelab outpost's sandbox an ant works behind a sealed wall: it walks
// into the wall and turns back. Pointing at or focusing a part of the map
// highlights its line in the legend, and the other way round.
// In each workroom an ant does that room's work while the room is on screen.
import { motion, wait, onVisibility } from "./motion.js";
import * as colony from "./colony.js";
import { Path } from "./ants.js";
import { smooth } from "./colony.js";

const map = document.getElementById("map");
const legend = document.getElementById("map-legend");
let mapVisible = false;

const OUTPOSTS = [
  { tunnel: "laptop", agent: "scribe" },
  { tunnel: "homelab", agent: "homelab" },
  { tunnel: "gpu", agent: "builder" },
];
const SANDBOX_AGENT = "reviewer";

/** The map drawing on screen now: the wide one, or the tall one on a phone. */
function activeSVG() {
  return [...map.querySelectorAll(".map__svg")].find((s) => s.getBoundingClientRect().width > 0);
}

function tunnelPath(name, reverse) {
  const svg = activeSVG();
  const el = svg && (svg.querySelector(`#tun-${name}`) || svg.querySelector(`#tun-${name}-t`));
  return el ? Path.fromSVG(el, 5, reverse) : null;
}

const held = () => !mapVisible;
const sleep = (ms) => wait(ms, held);
const smallScale = () => colony.limits().scale * 0.7;

/** One outpost's round trip: hall -> outpost with a message, a reply back. */
async function errand({ tunnel, agent }) {
  const out = tunnelPath(tunnel, true); // the paths are drawn outpost -> hall
  if (!out) return;
  const start = out.at(0);
  const ant = colony.recruit(agent, start);
  ant.scale = smallScale();
  ant.carry = true;
  if (Math.hypot(ant.x - start.x, ant.y - start.y) > 4) await colony.walk(ant, smooth([{ x: ant.x, y: ant.y }, start]));
  await colony.walk(ant, out);
  ant.carry = false;
  ant.pauseLeft = 0.6;
  await sleep(900);
  const back = tunnelPath(tunnel, false);
  ant.carry = true;
  if (back) await colony.walk(ant, back);
  colony.remove(ant);
}

let sandboxAnt = null;
/** The sealed sandbox: the ant walks into the wall, bumps, and turns back. */
async function sandbox() {
  const svg = activeSVG();
  const box = svg?.querySelector(".map-sandbox");
  if (!box) return;
  const r = box.getBoundingClientRect();
  const y = r.top + scrollY + r.height * 0.62;
  const left = r.left + 18, right = r.right - 18;
  const ant = colony.recruit(SANDBOX_AGENT, { x: left + 10, y });
  sandboxAnt = ant;
  ant.scale = smallScale() * 0.9;
  ant.x = left + 10; ant.y = y;
  let toRight = true;
  while (mapVisible && !motion.reduced) {
    const from = { x: ant.x, y }, to = { x: toRight ? right : left, y: y + (Math.random() - 0.5) * 6 };
    await colony.walk(ant, new Path([from, to]));
    // the bump: a little jolt back, a pause, then it turns round
    ant.pauseLeft = 0.5;
    ant.x += toRight ? -2 : 2;
    await sleep(700);
    toRight = !toRight;
  }
  colony.remove(ant);
  sandboxAnt = null;
}

let running = false;
async function runMap() {
  if (running) return;
  running = true;
  sandbox();
  let i = 0;
  while (mapVisible && !motion.reduced) {
    await sleep(600);
    errand(OUTPOSTS[i % OUTPOSTS.length]);
    i++;
    await sleep(2600);
  }
  running = false;
}

function highlight(part, on) {
  for (const el of map.querySelectorAll(`[data-part="${part}"]`)) el.classList.toggle("is-active", on);
  for (const el of legend.querySelectorAll(`[data-part="${part}"]`)) el.classList.toggle("is-active", on);
}

function wireHighlights() {
  for (const el of [...map.querySelectorAll(".map-part, .map-tunnel"), ...legend.querySelectorAll("[data-part]")]) {
    const part = el.dataset.part;
    el.addEventListener("pointerenter", () => highlight(part, true));
    el.addEventListener("pointerleave", () => highlight(part, false));
    el.addEventListener("focus", () => highlight(part, true));
    el.addEventListener("blur", () => highlight(part, false));
  }
}

// --- workrooms ------------------------------------------------------------------

const ROOM_AGENTS = { steps: "assistant", terminals: "builder", files: "reviewer", planner: "scribe", search: "homelab" };

/** An ant paces along the bottom of its room's drawing, stopping to work. */
async function work(room, agent) {
  const art = room.querySelector(".room__art");
  const r = art.getBoundingClientRect();
  const y = r.bottom + scrollY - 6;
  const left = r.left + r.width * 0.18, right = r.right - r.width * 0.18;
  const ant = colony.recruit(agent, { x: left, y });
  ant.scale = smallScale();
  ant.x = left; ant.y = y;
  room.ant = ant;
  let toRight = true;
  while (room.visible && !motion.reduced) {
    await colony.walk(ant, new Path([{ x: ant.x, y: ant.y }, { x: toRight ? right : left, y }]));
    ant.pauseLeft = 0.9 + Math.random() * 0.8;
    ant.look = [0.3, -1]; // it looks up at its work
    await wait(1400);
    ant.look = null;
    toRight = !toRight;
  }
  colony.remove(ant);
  room.ant = null;
}

export function start() {
  if (map) {
    wireHighlights();
    onVisibility(map, (v) => {
      mapVisible = v;
      if (v) { map.classList.add("is-dug"); if (!motion.reduced) setTimeout(runMap, motion.reduced ? 0 : 1200); }
    }, "0px 0px -15% 0px");
  }
  for (const room of document.querySelectorAll(".room")) {
    const agent = ROOM_AGENTS[room.dataset.room];
    if (!agent) continue; // five agents, so the sixth room shows its own pulse instead
    onVisibility(room, (v) => {
      room.visible = v;
      if (v && !room.ant && !motion.reduced) work(room, agent);
    }, "0px 0px -10% 0px");
  }
  motion.subscribe(() => {
    if (motion.reduced) return;
    if (mapVisible && !running) runMap();
  });
}
