// Small surprises (docs/18, 4.8) and the theme switch.
//  - Typing an agent's name (outside a text field), e.g. "@scribe", brings
//    its ant out to wave.
//  - The sleeping ants in the footer wake up when pointed at or tapped.
//  - Night or dawn: the theme follows prefers-color-scheme; the switch
//    cycles system -> dawn -> night, for this page view only (no storage).
import { t } from "./i18n.js";
import { motion } from "./motion.js";
import * as colony from "./colony.js";
import { AGENTS } from "./ants.js";
import { smooth } from "./colony.js";

// --- typing a name -------------------------------------------------------------
let typed = "";
async function summon(agent) {
  if (motion.still || motion.stopped) return;
  // a name is one ant: if it is on screen already (even busy), it waves where it is
  const here = colony.all().find((a) => a.agent === agent && !a.hidden && colony.onScreen(a.y, 0));
  if (here) { here.wave = 2.4; colony.wake(); return; }
  const y = scrollY + innerHeight * 0.7, fromLeft = Math.random() < 0.5;
  const ant = colony.recruit(agent, { x: fromLeft ? -50 : innerWidth + 50, y });
  const mid = { x: innerWidth * (fromLeft ? 0.4 : 0.6), y };
  await colony.walk(ant, smooth([{ x: ant.x, y: ant.y }, mid]));
  ant.angle = Math.PI / 2; // it faces you
  ant.wave = 2.4;
  await new Promise((r) => setTimeout(r, 2600));
  await colony.walk(ant, smooth([{ x: ant.x, y: ant.y }, { x: fromLeft ? innerWidth + 60 : -60, y: y + 30 }]));
  colony.remove(ant);
}

function onKey(e) {
  if (e.ctrlKey || e.metaKey || e.altKey || e.key.length !== 1) return;
  if (e.target.closest("input, textarea, select, [contenteditable]")) return;
  typed = (typed + e.key.toLowerCase()).slice(-16);
  for (const agent of AGENTS) {
    if (typed.endsWith(`@${agent}`)) { typed = ""; summon(agent); return; }
  }
}

// --- the sleepers ----------------------------------------------------------------
function wireSleepers() {
  for (const s of document.querySelectorAll(".sleeper")) {
    let timer = 0;
    const wake = () => {
      if (motion.reduced) return;
      s.classList.add("is-awake");
      clearTimeout(timer);
      timer = setTimeout(() => s.classList.remove("is-awake"), 2600);
    };
    s.addEventListener("pointerenter", wake);
    s.addEventListener("click", wake);
  }
}

// --- night or dawn -----------------------------------------------------------------
const MODES = ["system", "light", "dark"];
function wireTheme() {
  const btn = document.getElementById("theme-toggle");
  if (!btn) return;
  let mode = "system";
  const apply = () => {
    if (mode === "system") delete document.documentElement.dataset.theme;
    else document.documentElement.dataset.theme = mode;
    btn.dataset.mode = mode;
    btn.setAttribute("aria-label", t(`theme.${mode}`)); // i18n-prefix: theme
    btn.title = t(`theme.${mode}`);
  };
  btn.hidden = false;
  apply();
  btn.addEventListener("click", () => {
    mode = MODES[(MODES.indexOf(mode) + 1) % MODES.length];
    apply();
  });
}

export function start() {
  addEventListener("keydown", onKey);
  wireSleepers();
  wireTheme();
}
