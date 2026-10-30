/**
 * Floating windows (docs/handoff/32, part 1): pure state and rules, no React.
 *
 * Tasks, calendar and notes can float inside the page: one window of each
 * kind, several kinds at once. A window can be moved anywhere (its title bar
 * stays reachable, it snaps to the edges), resized (280×200 up to the
 * viewport), brought to the front, minimised to a 48 px circle (which can be
 * dragged and snaps to the nearest edge) and restored where it was, and it
 * has an opacity mode: normal, auto (55 % while the cursor is elsewhere) or
 * dim (20 %). Positions survive reloads (serialize/deserialize); after the
 * viewport shrinks they are clamped again. Phones have no floating windows.
 */

export type WindowKind = "tasks" | "calendar" | "notes";
export type Opacity = "normal" | "auto" | "dim";
export type Rect = { x: number; y: number; w: number; h: number };
export type Win = { state: "open" | "min" | "closed"; rect: Rect; bubble: { x: number; y: number }; opacity: Opacity; z: number };
export type Windows = Record<WindowKind, Win>;
export type Viewport = { w: number; h: number };

export const KINDS: WindowKind[] = ["tasks", "calendar", "notes"];
export const MIN = { w: 280, h: 200 };
export const TITLE = 34;
export const BUBBLE = 48;
export const SNAP = 16;
/** How much of the title bar must stay on screen horizontally. */
const GRIP = 96;

const defaults: Record<WindowKind, Rect> = {
  tasks: { x: -364, y: 68, w: 340, h: 440 },
  calendar: { x: -364, y: 120, w: 340, h: 460 },
  notes: { x: -384, y: 172, w: 360, h: 420 },
};

/** Default rects use negative x for "from the right edge". */
function initialRect(kind: WindowKind, vp: Viewport): Rect {
  const d = defaults[kind];
  return { ...d, x: d.x < 0 ? vp.w + d.x : d.x };
}

export function initial(vp: Viewport): Windows {
  const out = {} as Windows;
  KINDS.forEach((k, i) => {
    out[k] = { state: "closed", rect: initialRect(k, vp), bubble: clampBubble({ x: vp.w, y: vp.h - BUBBLE - 120 - i * 60 }, vp), opacity: "auto", z: 0 };
  });
  return out;
}

const clampN = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), Math.max(lo, hi));

/** Keeps a window on screen: the title bar reachable, the size within bounds; snaps to edges. */
export function clampRect(r: Rect, vp: Viewport): Rect {
  const w = clampN(r.w, MIN.w, vp.w);
  const h = clampN(r.h, MIN.h, vp.h);
  let x = clampN(r.x, GRIP - w, vp.w - GRIP);
  let y = clampN(r.y, 0, vp.h - TITLE);
  if (Math.abs(x) <= SNAP) x = 0;
  if (Math.abs(vp.w - (x + w)) <= SNAP) x = vp.w - w;
  if (Math.abs(y) <= SNAP) y = 0;
  if (Math.abs(vp.h - (y + h)) <= SNAP) y = vp.h - h;
  return { x, y, w, h };
}

/** A minimised circle: inside the viewport, snapped to the nearest side edge. */
export function clampBubble(b: { x: number; y: number }, vp: Viewport): { x: number; y: number } {
  const y = clampN(b.y, 0, vp.h - BUBBLE);
  const x = b.x + BUBBLE / 2 < vp.w / 2 ? 8 : vp.w - BUBBLE - 8;
  return { x, y };
}

export type WinAction =
  | { type: "open"; kind: WindowKind }
  | { type: "close"; kind: WindowKind }
  | { type: "minimize"; kind: WindowKind }
  | { type: "restore"; kind: WindowKind }
  | { type: "focus"; kind: WindowKind }
  | { type: "move"; kind: WindowKind; x: number; y: number }
  | { type: "resize"; kind: WindowKind; rect: Rect }
  | { type: "moveBubble"; kind: WindowKind; x: number; y: number }
  | { type: "cycleOpacity"; kind: WindowKind }
  | { type: "viewport" };

const nextOpacity: Record<Opacity, Opacity> = { normal: "auto", auto: "dim", dim: "normal" };

function top(s: Windows): number {
  return Math.max(0, ...KINDS.map((k) => s[k].z));
}

export function reduce(s: Windows, a: WinAction, vp: Viewport): Windows {
  if (a.type === "viewport") {
    const out = { ...s };
    for (const k of KINDS) out[k] = { ...s[k], rect: clampRect(s[k].rect, vp), bubble: clampBubble(s[k].bubble, vp) };
    return out;
  }
  const w = s[a.kind];
  const set = (patch: Partial<Win>): Windows => ({ ...s, [a.kind]: { ...w, ...patch } });
  switch (a.type) {
    case "open":
    case "restore":
    case "focus":
      if (a.type === "focus" && w.state !== "open") return s;
      return set({ state: "open", z: w.z === top(s) && w.state === "open" ? w.z : top(s) + 1, rect: clampRect(w.rect, vp) });
    case "close":
      return set({ state: "closed" });
    case "minimize":
      return set({ state: "min", bubble: clampBubble(w.bubble, vp) });
    case "move":
      return set({ rect: clampRect({ ...w.rect, x: a.x, y: a.y }, vp) });
    case "resize":
      return set({ rect: clampRect(a.rect, vp) });
    case "moveBubble":
      return set({ bubble: clampBubble({ x: a.x, y: a.y }, vp) });
    case "cycleOpacity":
      return set({ opacity: nextOpacity[w.opacity] });
  }
}

/** Open windows, back to front. */
export function stack(s: Windows): WindowKind[] {
  return KINDS.filter((k) => s[k].state === "open").sort((a, b) => s[a].z - s[b].z);
}

/* ---------- persistence (per viewer; the caller wraps storage in try/catch) ---------- */

export function serialize(s: Windows): string {
  return JSON.stringify(s);
}

export function deserialize(raw: string | null, vp: Viewport): Windows {
  const base = initial(vp);
  if (!raw) return base;
  try {
    const data = JSON.parse(raw) as Partial<Windows>;
    for (const k of KINDS) {
      const w = data[k];
      if (!w || typeof w !== "object") continue;
      const rect = w.rect && [w.rect.x, w.rect.y, w.rect.w, w.rect.h].every(Number.isFinite) ? w.rect : base[k].rect;
      base[k] = {
        state: w.state === "open" || w.state === "min" ? w.state : "closed",
        rect: clampRect(rect, vp),
        bubble: clampBubble(w.bubble && Number.isFinite(w.bubble.x) ? w.bubble : base[k].bubble, vp),
        opacity: w.opacity === "normal" || w.opacity === "dim" ? w.opacity : "auto",
        z: Number.isFinite(w.z) ? w.z! : 0,
      };
    }
  } catch {
    // unreadable: the defaults
  }
  return base;
}
