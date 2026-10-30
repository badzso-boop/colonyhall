import { MAX_TABS, empty, fromQuery, place, reduce, toQuery, type PaneAction, type PaneState, type WorkTab } from "./panes";

const term = (name: string): WorkTab => ({ kind: "terminal", agentId: "a1", ref: name });
const files: WorkTab = { kind: "files", agentId: "a1", ref: "src/main.go" };
const run = (...actions: PaneAction[]) => actions.reduce(reduce, empty);

test("rule 1: a pane opens on the right when the right is empty", () => {
  expect(place(run({ type: "openThread", rootId: "t" }), "wide")).toMatchObject({ left: "sidebar", right: "thread" });
  expect(place(run({ type: "openWork", tab: term("build") }), "wide")).toMatchObject({ left: "sidebar", right: "work" });
});

test("rule 2: with a thread on the right, a work pane goes left and the thread stays", () => {
  const s = run({ type: "openThread", rootId: "t" }, { type: "openWork", tab: term("build") });
  expect(place(s, "wide")).toMatchObject({ left: "work", right: "thread" });
});

test("rule 3: with work on the right, a thread takes the right and work moves left, keeping its tabs", () => {
  const s = run({ type: "openWork", tab: term("build") }, { type: "openWork", tab: files }, { type: "openThread", rootId: "t" });
  expect(place(s, "wide")).toMatchObject({ left: "work", right: "thread" });
  expect(s.tabs).toHaveLength(2);
  expect(s.active).toBe(1);
});

test("rule 4: one work pane with tabs; reopening selects; more than the maximum drops the oldest", () => {
  let s = run({ type: "openWork", tab: term("a") }, { type: "openWork", tab: term("b") }, { type: "openWork", tab: term("a") });
  expect(s.tabs.map((t) => t.ref)).toEqual(["a", "b"]);
  expect(s.active).toBe(0);
  for (let i = 0; i < MAX_TABS; i++) s = reduce(s, { type: "openWork", tab: term(`n${i}`) });
  expect(s.tabs).toHaveLength(MAX_TABS);
  expect(s.tabs[0]!.ref).toBe("n0");
  expect(s.active).toBe(MAX_TABS - 1);
});

test("closing tabs keeps a sensible active tab; the last one closes the work pane", () => {
  let s = run({ type: "openWork", tab: term("a") }, { type: "openWork", tab: term("b") }, { type: "openWork", tab: term("c") });
  s = reduce(s, { type: "closeTab", index: 0 });
  expect(s.tabs.map((t) => t.ref)).toEqual(["b", "c"]);
  expect(s.active).toBe(1);
  s = reduce(s, { type: "closeTab", index: 1 });
  expect(s.active).toBe(0);
  s = reduce(reduce(s, { type: "openThread", rootId: "t" }), { type: "closeTab", index: 0 });
  expect(s.tabs).toEqual([]);
  expect(s.focus).toBe("thread");
});

test("rule 5: a second thread replaces the first", () => {
  expect(run({ type: "openThread", rootId: "t1" }, { type: "openThread", rootId: "t2" }).thread).toBe("t2");
});

test("closing the thread brings the sidebar back and work to the right", () => {
  const s = run({ type: "openThread", rootId: "t" }, { type: "openWork", tab: files }, { type: "closeThread" });
  expect(place(s, "wide")).toMatchObject({ left: "sidebar", right: "work" });
});

test("rule 7: medium width has one slot; the latest pane wins and the other is one tap away", () => {
  const s = run({ type: "openThread", rootId: "t" }, { type: "openWork", tab: files });
  expect(place(s, "medium")).toMatchObject({ left: "sidebar", right: "work", other: "thread" });
  expect(place(reduce(s, { type: "focus", pane: "thread" }), "medium")).toMatchObject({ right: "thread", other: "work" });
});

test("rule 8: narrow width is a stack, conversation → thread → work", () => {
  expect(place(empty, "narrow").screen).toBe("main");
  const s = run({ type: "openThread", rootId: "t" });
  expect(place(s, "narrow")).toMatchObject({ screen: "thread", other: null });
  const w = reduce(s, { type: "openWork", tab: files });
  expect(place(w, "narrow")).toMatchObject({ screen: "work", other: "thread" });
  expect(place(reduce(w, { type: "closeWork" }), "narrow").screen).toBe("thread");
});

test("rule 9: the state round-trips through the URL, odd characters included", () => {
  const s: PaneState = run(
    { type: "openThread", rootId: "t" },
    { type: "openWork", tab: { kind: "files", agentId: "a1", ref: "dir with space/a,b:c.md" } },
    { type: "openWork", tab: term("build") },
    { type: "selectTab", index: 0 },
  );
  const q = new URLSearchParams(toQuery(s));
  expect(fromQuery(q)).toEqual(s);
  expect(fromQuery(new URLSearchParams())).toEqual(empty);
});

test("fromQuery ignores what it does not understand", () => {
  const q = new URLSearchParams({ work: "bogus:x:y,terminal::z,terminal:a1:%E0%A4%A", tab: "9", focus: "work" });
  const s = fromQuery(q);
  expect(s.tabs).toEqual([{ kind: "terminal", agentId: "a1", ref: "" }]);
  expect(s.active).toBe(0);
});
