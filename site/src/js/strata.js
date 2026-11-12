// Strata (docs/18, 4.7): the counters count up as they scroll in, and an
// ant digs along the layer of the round in progress. The layers and the
// final numbers are in the page itself (no JavaScript needed to read them);
// with reduced motion nothing counts or digs.
import { lang } from "./i18n.js";
import { motion, wait, onVisibility } from "./motion.js";
import * as colony from "./colony.js";
import { Path } from "./ants.js";

const fmt = new Intl.NumberFormat(lang);

function countUp(dd) {
  const target = Number(dd.dataset.count);
  if (!target || motion.reduced) return;
  const t0 = performance.now(), dur = 1300;
  const step = (now) => {
    const k = Math.min(1, (now - t0) / dur);
    dd.textContent = fmt.format(Math.round(target * (1 - (1 - k) ** 3)));
    if (k < 1 && !motion.paused) requestAnimationFrame(step);
    else dd.textContent = fmt.format(target);
  };
  requestAnimationFrame(step);
}

let digging = false;
async function dig(layer) {
  if (digging || motion.reduced) return;
  digging = true;
  const r = layer.getBoundingClientRect();
  const y = r.top + scrollY - 2;
  const left = r.left + 40, right = r.right - 40;
  const ant = colony.recruit("builder", { x: left, y });
  ant.scale = colony.limits().scale * 0.75;
  let toRight = true;
  while (layer.visible && !motion.reduced) {
    await colony.walk(ant, new Path([{ x: ant.x, y: ant.y }, { x: toRight ? right : left, y }]));
    ant.pauseLeft = 1.2; // digging
    await wait(1500, () => !layer.visible);
    toRight = !toRight;
  }
  colony.remove(ant);
  digging = false;
}

export function start() {
  for (const dd of document.querySelectorAll(".counter dd[data-count]")) {
    let done = false;
    onVisibility(dd, (v) => { if (v && !done) { done = true; countUp(dd); } }, "0px 0px -10% 0px");
  }
  const layer = document.querySelector(".layer--digging");
  if (layer) onVisibility(layer, (v) => { layer.visible = v; if (v) dig(layer); });
}
