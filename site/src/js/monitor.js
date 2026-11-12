// The live monitor in the hero (docs/18, 4.2). A prewritten script with an
// invented person and invented agents: the ants carry the messages in, and
// a working agent's ant goes into the workroom under the monitor, from where
// its steps arrive. The script loops; it holds while the monitor is
// scrolled away and continues from the same place. With reduced motion (or
// without JavaScript) the monitor shows the script's end state.
import { data, t } from "./i18n.js";
import { motion, wait, onVisibility } from "./motion.js";
import * as colony from "./colony.js";
import { smooth } from "./colony.js";
import { lay, anchor } from "./trails.js";

const root = document.getElementById("monitor");
const msgs = root ? [...root.querySelectorAll("[data-msg]")] : [];
const steps = root ? [...root.querySelectorAll("[data-step]")] : [];
const composer = root?.querySelector(".app__composer");
const screen = root?.querySelector(".monitor__screen");
const burrow = root?.querySelector(".monitor__burrow");
let visible = true;
const held = () => !visible;
const sleep = (ms) => wait(ms, held);
let live = null; // the step that is running (it turns "stopped" at the gate)

function docRect(el) {
  const r = el.getBoundingClientRect();
  return { left: r.left, right: r.right, top: r.top + scrollY, bottom: r.bottom + scrollY, width: r.width, height: r.height };
}

/** Waits for an ant to finish its path; the promise holds while it is frozen. */
function walk(ant, path) {
  return new Promise((resolve) => {
    ant.path = path; ant.d = 0; ant.done = false; ant.resume = null;
    ant.onDone = (a) => { a.done = false; a.onDone = null; resolve(); };
    colony.wake();
  });
}

function burrowPoint() {
  const b = docRect(burrow);
  return { x: b.left + b.width / 2, y: b.top + b.height / 2 };
}

function edgePoint(fromLeft, y) {
  return { x: fromLeft ? -50 : innerWidth + 50, y };
}

/**
 * A new ant walks in at ground level and disappears into the workroom.
 * Returns the ant at once and a promise for its arrival.
 */
function goToWork(agent) {
  const b = burrowPoint();
  // a name is one ant: if this agent's ant is already wandering about, it is called in
  let ant = colony.all().find((a) => a.agent === agent && !a.keep && !a.still && colony.onScreen(a.y));
  let start;
  if (ant) {
    start = { x: ant.x, y: ant.y };
    ant.target = null;
  } else {
    for (const a of colony.all()) if (a.agent === agent && !a.keep) colony.remove(a);
    start = edgePoint(Math.random() < 0.5, b.y - 6);
    ant = colony.spawn({ agent, path: smooth([start, b]) });
  }
  ant.keep = true;
  ant.speedMul = 1.8;
  ant.path = smooth([start, { x: (start.x + b.x) / 2, y: b.y - 14 }, b]);
  const arrived = walk(ant, ant.path).then(() => { ant.hidden = true; return ant; });
  return { ant, arrived };
}

/** The ant comes out of the workroom with a parcel and climbs to the newest message. */
async function carry(ant, msg) {
  const b = burrowPoint(), s = docRect(screen), stand = { x: s.left + s.width / 2, y: s.bottom + 4 };
  const list = docRect(root.querySelector(".app__msgs"));
  const slot = { x: Math.min(list.right - 40, list.left + list.width * 0.55), y: list.bottom - 26 };
  ant.hidden = false;
  ant.carry = true;
  ant.x = b.x; ant.y = b.y;
  await walk(ant, smooth([b, stand, { x: (stand.x + slot.x) / 2, y: (stand.y + slot.y) / 2 + 10 }, slot]));
  ant.carry = false;
  msg.classList.add("is-in");
  ant.pauseLeft = 0.4;
  return ant;
}

async function leave(ant) {
  const s = docRect(screen);
  const right = Math.random() < 0.5;
  await walk(ant, smooth([{ x: ant.x, y: ant.y }, { x: right ? s.right - 20 : s.left + 20, y: s.top + 10 }, edgePoint(right, s.top - 40)]));
  colony.remove(ant);
}

async function typeMessage(msg) {
  const text = msg.querySelector(".msg__text").textContent;
  composer.classList.add("is-typing");
  for (let i = 1; i <= text.length; i += 2) {
    composer.textContent = text.slice(0, i);
    await sleep(28);
  }
  await sleep(300);
  composer.textContent = t("monitor.composer");
  composer.classList.remove("is-typing");
  msg.classList.add("is-in");
}

function avatarOf(agent) {
  return msgs.find((m) => m.querySelector(".msg__who")?.textContent === t(`agents.${agent}`))?.querySelector(".av"); // i18n-prefix: agents
}

const workers = new Map(); // agent -> promise of its ant, once in the workroom

async function run(step) {
  const msg = step.msg && root.querySelector(`[data-msg="${step.msg}"]`);
  switch (step.do) {
    case "type":
      await typeMessage(msg);
      break;
    case "work": {
      if (!workers.has(step.agent)) workers.set(step.agent, goToWork(step.agent).arrived);
      await workers.get(step.agent);
      avatarOf(step.agent)?.classList.add("is-working");
      for (const id of step.steps) {
        const li = root.querySelector(`[data-step="${id}"]`);
        live?.classList.remove("is-live");
        li.classList.add("is-in", "is-live");
        live = li;
        await sleep(1400);
      }
      live?.classList.remove("is-live");
      live = null;
      avatarOf(step.agent)?.classList.remove("is-working");
      break;
    }
    case "carry": {
      const ant = await (workers.get(step.agent) || goToWork(step.agent).arrived);
      workers.delete(step.agent);
      await carry(ant, msg);
      if (step.handoff) {
        // the task goes on to another agent: its ant heads for the workroom,
        // and a pheromone trail runs from this message to it
        const next = goToWork(step.handoff);
        workers.set(step.handoff, next.arrived);
        lay(anchor(msg.querySelector(".av")), () => ({ x: next.ant.x, y: next.ant.y }), 5);
      }
      await sleep(500);
      leave(ant);
      break;
    }
    case "rest":
      await sleep(step.ms);
      break;
  }
}

function reset() {
  for (const m of msgs) m.classList.remove("is-in");
  for (const s of steps) s.classList.remove("is-in", "is-live");
}

function showEnd() {
  for (const m of msgs) m.classList.add("is-in");
  for (const s of steps) s.classList.add("is-in");
}

let looping = false;
async function loop() {
  if (looping) return;
  looping = true;
  const script = data.hero?.loop || [];
  while (!motion.reduced) {
    reset();
    for (const step of script) {
      if (motion.reduced) break;
      await run(step);
    }
  }
  looping = false;
  showEnd();
}

// At the gate's Stop the running step turns "stopped"; Resume restores it.
let liveText = "";
motion.subscribe(() => {
  if (motion.stopped && live && !live.classList.contains("is-stopped")) {
    liveText = live.lastChild.textContent;
    live.lastChild.textContent = ` ${t("monitor.stopped")}`;
    live.classList.add("is-stopped");
  } else if (!motion.stopped && live?.classList.contains("is-stopped")) {
    live.lastChild.textContent = liveText;
    live.classList.remove("is-stopped");
  }
  if (!motion.reduced && !looping) loop();
});

export function start() {
  if (!root) return;
  onVisibility(root, (v) => { visible = v; if (v) colony.wake(); });
  if (motion.reduced) showEnd();
  else loop();
}
