import { BUBBLE, MIN, deserialize, initial, reduce, serialize, stack, type Viewport, type WinAction, type Windows } from "./windows";

const vp: Viewport = { w: 1440, h: 900 };
const run = (...as: WinAction[]) => as.reduce((s, a) => reduce(s, a, vp), initial(vp));

test("open, one window per kind, focus brings to the front", () => {
  let s = run({ type: "open", kind: "tasks" }, { type: "open", kind: "calendar" });
  expect(stack(s)).toEqual(["tasks", "calendar"]);
  s = reduce(s, { type: "focus", kind: "tasks" }, vp);
  expect(stack(s)).toEqual(["calendar", "tasks"]);
  s = reduce(s, { type: "open", kind: "tasks" }, vp);
  expect(stack(s)).toEqual(["calendar", "tasks"]);
  expect(Object.values(s).filter((w) => w.state === "open")).toHaveLength(2);
});

test("moving keeps the title bar reachable and snaps to the edges", () => {
  let s = run({ type: "open", kind: "tasks" }, { type: "move", kind: "tasks", x: 5000, y: -300 });
  const r = s.tasks.rect;
  expect(r.y).toBe(0);
  expect(r.x).toBeLessThanOrEqual(vp.w - 96);
  s = reduce(s, { type: "move", kind: "tasks", x: 10, y: 400 }, vp);
  expect(s.tasks.rect.x).toBe(0); // snapped to the left edge
  s = reduce(s, { type: "move", kind: "tasks", x: vp.w - s.tasks.rect.w - 9, y: 400 }, vp);
  expect(s.tasks.rect.x + s.tasks.rect.w).toBe(vp.w); // snapped to the right edge
});

test("resizing respects the minimum and the viewport", () => {
  let s = run({ type: "open", kind: "notes" }, { type: "resize", kind: "notes", rect: { x: 100, y: 100, w: 50, h: 20 } });
  expect([s.notes.rect.w, s.notes.rect.h]).toEqual([MIN.w, MIN.h]);
  s = reduce(s, { type: "resize", kind: "notes", rect: { x: 0, y: 0, w: 9999, h: 9999 } }, vp);
  expect([s.notes.rect.w, s.notes.rect.h]).toEqual([vp.w, vp.h]);
});

test("minimise to a circle and restore at the same place", () => {
  let s = run({ type: "open", kind: "calendar" }, { type: "move", kind: "calendar", x: 300, y: 200 });
  const before = s.calendar.rect;
  s = reduce(s, { type: "minimize", kind: "calendar" }, vp);
  expect(s.calendar.state).toBe("min");
  expect(stack(s)).toEqual([]);
  s = reduce(s, { type: "moveBubble", kind: "calendar", x: 200, y: 5000 }, vp);
  expect(s.calendar.bubble).toEqual({ x: 8, y: vp.h - BUBBLE }); // nearest edge, inside
  s = reduce(s, { type: "restore", kind: "calendar" }, vp);
  expect(s.calendar.state).toBe("open");
  expect(s.calendar.rect).toEqual(before);
});

test("opacity cycles normal → auto → dim; the default is auto", () => {
  let s = run({ type: "open", kind: "tasks" });
  expect(s.tasks.opacity).toBe("auto");
  s = reduce(s, { type: "cycleOpacity", kind: "tasks" }, vp);
  expect(s.tasks.opacity).toBe("dim");
  s = reduce(s, { type: "cycleOpacity", kind: "tasks" }, vp);
  expect(s.tasks.opacity).toBe("normal");
});

test("a smaller viewport clamps every window again", () => {
  let s = run({ type: "open", kind: "tasks" }, { type: "move", kind: "tasks", x: 1300, y: 800 });
  const small = { w: 390, h: 700 };
  s = reduce(s, { type: "viewport" }, small);
  expect(s.tasks.rect.x).toBeLessThanOrEqual(small.w - 96);
  expect(s.tasks.rect.y).toBeLessThanOrEqual(small.h - 34);
  expect(s.tasks.rect.w).toBeLessThanOrEqual(small.w);
});

test("persistence round trip, and garbage gives the defaults", () => {
  const s: Windows = run({ type: "open", kind: "notes" }, { type: "move", kind: "notes", x: 400, y: 300 }, { type: "cycleOpacity", kind: "notes" }, { type: "open", kind: "tasks" }, { type: "minimize", kind: "tasks" });
  expect(deserialize(serialize(s), vp)).toEqual(s);
  expect(deserialize("{nope", vp)).toEqual(initial(vp));
  expect(deserialize('{"tasks":{"state":"open","rect":{"x":"a"}}}', vp).tasks.rect).toEqual(initial(vp).tasks.rect);
});

test("close hides the window", () => {
  const s = run({ type: "open", kind: "tasks" }, { type: "close", kind: "tasks" });
  expect(s.tasks.state).toBe("closed");
  expect(stack(s)).toEqual([]);
});

test("a window opening over another goes beside it, or cascades when there is no room", () => {
  const vp = { w: 1440, h: 900 };
  let s = reduce(initial(vp), { type: "open", kind: "tasks" }, vp);
  s = reduce(s, { type: "open", kind: "calendar" }, vp);
  const [t, c] = [s.tasks.rect, s.calendar.rect];
  expect(c.x + c.w).toBeLessThanOrEqual(t.x);
  expect(c.y).toBe(t.y);
  // Reopening keeps where the user put it.
  s = reduce(s, { type: "minimize", kind: "calendar" }, vp);
  s = reduce(s, { type: "restore", kind: "calendar" }, vp);
  expect(s.calendar.rect).toEqual(c);
  // A narrow screen has no room beside: cascade instead.
  const small = { w: 500, h: 900 };
  let n = reduce(initial(small), { type: "open", kind: "tasks" }, small);
  n = reduce(n, { type: "move", kind: "calendar", x: n.tasks.rect.x, y: n.tasks.rect.y }, small);
  n = reduce(n, { type: "open", kind: "calendar" }, small);
  expect(n.calendar.rect.y).toBeGreaterThan(n.tasks.rect.y);
});
