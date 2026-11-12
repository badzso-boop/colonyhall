// Entry point (an ES module, so deferred). The page is complete without it;
// this adds the copy buttons, the pause control and the colony.
import { t } from "./i18n.js";
import { motion } from "./motion.js";
import * as colony from "./colony.js";
import * as monitor from "./monitor.js";
import * as gate from "./gate.js";
import * as map from "./map.js";
import * as terminal from "./terminal.js";
import * as strata from "./strata.js";
import * as eggs from "./eggs.js";
import { onVisibility } from "./motion.js";

for (const el of document.querySelectorAll("[data-js-only]")) el.hidden = false;

// Copy buttons next to commands.
for (const btn of document.querySelectorAll("[data-copy]")) {
  btn.addEventListener("click", async () => {
    const code = btn.closest("[data-copy-root], .cmd")?.querySelector("[data-copy-text]");
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code.textContent.trim());
      btn.textContent = t("copy.done");
    } catch {
      const range = document.createRange();
      range.selectNodeContents(code);
      getSelection().removeAllRanges();
      getSelection().addRange(range);
      btn.textContent = t("copy.failed");
    }
    setTimeout(() => { btn.textContent = t("copy.button"); }, 1800);
  });
}

// The permanent pause control (WCAG 2.2.2): stops every moving part.
const pause = document.getElementById("motion-toggle");
const live = document.getElementById("live");
// One label ("Pause motion") with aria-pressed: pressed means paused.
function syncPause() {
  pause.setAttribute("aria-pressed", String(motion.paused));
}
pause.hidden = false;
syncPause();
pause.addEventListener("click", () => {
  motion.set({ paused: !motion.paused });
  syncPause();
  live.textContent = motion.paused ? t("motion.paused") : t("motion.playing");
});

// CSS animations follow the same switch: paused, stopped or reduced.
const root = document.documentElement;
const syncRoot = () => {
  root.classList.toggle("motion-off", motion.still || motion.stopped);
  root.classList.toggle("colony-stopped", motion.stopped);
  root.classList.toggle("reduced", motion.reduced);
};
motion.subscribe(syncRoot);
syncRoot();

// The hall's pheromone trail runs only while the thread is on screen.
const thread = document.getElementById("hall-thread");
if (thread) onVisibility(thread, (v) => thread.classList.toggle("is-live", v));

colony.start();
monitor.start();
gate.start();
map.start();
terminal.start();
strata.start();
eggs.start();
