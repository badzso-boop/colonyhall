// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The page-wide motion state: the visitor's pause button (WCAG 2.2.2), the
// system's reduced-motion setting, and the colony's emergency stop (the
// gate). Kept in memory for the life of the page only: no cookies, no
// storage (docs/18, 7).
const listeners = new Set();
const media = matchMedia("(prefers-reduced-motion: reduce)");

export const motion = {
  paused: false, // the visitor pressed Pause
  stopped: false, // the colony was stopped at the gate
  reduced: media.matches, // prefers-reduced-motion
  /** True when nothing should move on its own. */
  get still() { return this.paused || this.reduced; },
  set(patch) {
    Object.assign(this, patch);
    for (const fn of listeners) fn(this);
  },
  subscribe(fn) { listeners.add(fn); return () => listeners.delete(fn); },
};

media.addEventListener("change", () => motion.set({ reduced: media.matches }));

/**
 * Resolves after ms of running time: the clock waits while paused, stopped,
 * hidden, or while held() is true (e.g. the monitor is scrolled away).
 */
export function wait(ms, held = () => false) {
  return new Promise((resolve) => {
    let left = ms, last = performance.now();
    const tick = () => {
      const now = performance.now();
      if (!motion.paused && !motion.stopped && !document.hidden && !held()) left -= now - last;
      last = now;
      if (left <= 0) resolve();
      else setTimeout(tick, Math.min(left, 100));
    };
    setTimeout(tick, Math.min(left, 100));
  });
}

/** Resolves when the element is at least partly on screen. */
export function whenVisible(el) {
  return new Promise((resolve) => {
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) { io.disconnect(); resolve(); }
    });
    io.observe(el);
  });
}

/** Calls fn(true/false) whenever the element enters or leaves the screen. */
export function onVisibility(el, fn, rootMargin = "0px") {
  const io = new IntersectionObserver((entries) => fn(entries[entries.length - 1].isIntersecting), { rootMargin });
  io.observe(el);
  return () => io.disconnect();
}
