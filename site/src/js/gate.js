// The gate (docs/18, 4.4): an ant with a task waits at a red gate beside the
// web app's approval card. Approve opens the gate and the ant goes through.
// Stop halts the whole colony: every ant on the page stops, the trails go
// out, the monitor's running step turns "stopped", and the main message
// appears with a "Resume colony" button. Fully keyboard operable; with
// reduced motion the state still changes, without animation.
import { t } from "./i18n.js";
import { motion, wait, onVisibility } from "./motion.js";
import * as colony from "./colony.js";
import { smooth } from "./colony.js";

const stage = document.getElementById("gate-stage");
const approve = document.getElementById("gate-approve");
const stop = document.getElementById("gate-stop");
const done = document.getElementById("gate-approved");
const actions = stage?.querySelector(".card__actions");
const hint = stage?.querySelector(".card__hint");
const halt = document.getElementById("halt");
const resume = document.getElementById("halt-resume");
const live = document.getElementById("live");
const bars = document.getElementById("gate-bars");
const AGENT = "builder";

let ant = null;
let arriving = null;
let visible = false;

function docBox(el) {
  const r = el.getBoundingClientRect();
  return { left: r.left, right: r.right, top: r.top + scrollY, bottom: r.bottom + scrollY, width: r.width, height: r.height };
}

/** Where the ant waits: just before the bars, on the floor. */
function waitPoint() {
  const b = docBox(bars), scale = colony.limits().scale;
  return { x: b.left - 26 * scale, y: b.bottom - 14 * scale };
}

function bringAnt() {
  if (ant || arriving || motion.reduced || stage.classList.contains("is-open")) return;
  const w = waitPoint();
  const start = { x: -50, y: w.y };
  ant = colony.recruit(AGENT, start);
  ant.carry = true;
  stage.classList.add("has-live-ant");
  arriving = colony.walk(ant, smooth([{ x: ant.x, y: ant.y }, { x: (ant.x + w.x) / 2, y: w.y }, w])).then(() => {
    arriving = null;
    if (ant) { ant.look = [0.2, -1]; } // it looks up at the gate
  });
}

async function onApprove() {
  stage.classList.add("is-open");
  actions.hidden = true;
  hint.hidden = true;
  done.hidden = false;
  live.textContent = t("gate.liveApproved");
  if (ant) {
    if (arriving) await arriving;
    const a = ant;
    ant = null;
    a.look = null;
    const b = docBox(bars);
    await wait(motion.reduced ? 0 : 500);
    await colony.walk(a, smooth([{ x: a.x, y: a.y }, { x: b.right + 40, y: a.y }, { x: innerWidth + 60, y: a.y - 10 }]));
    colony.remove(a);
  }
  // a new task arrives after a while, so the gate can be tried again
  await wait(5000);
  stage.classList.remove("is-open", "has-live-ant");
  actions.hidden = false;
  hint.hidden = false;
  done.hidden = true;
  if (visible) bringAnt();
}

function setStopped(on) {
  motion.set({ stopped: on });
  halt.hidden = !on;
  live.textContent = on ? t("gate.liveStopped") : t("gate.liveResumed");
  if (on) resume.focus();
  else stop.focus();
}

export function start() {
  if (!stage) return;
  approve.addEventListener("click", onApprove);
  stop.addEventListener("click", () => setStopped(true));
  resume.addEventListener("click", () => setStopped(false));
  addEventListener("keydown", (e) => { if (e.key === "Escape" && motion.stopped) setStopped(false); });
  onVisibility(stage, (v) => { visible = v; if (v && !motion.stopped) bringAnt(); }, "0px 0px -20% 0px");
}
