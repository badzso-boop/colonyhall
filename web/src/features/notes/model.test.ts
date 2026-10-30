import { agentAuthors, groupNotes, searchNotes } from "./model";

const n = (id: string, agent: string, thread: string, at: string, text = "") => ({ id, agent_id: agent, thread_root_id: thread, text, created_at: at });

test("notes group by agent, then thread, newest thread first", () => {
  const g = groupNotes([n("1", "a", "t1", "2026-10-30T08:00:00Z"), n("2", "a", "t2", "2026-10-30T09:00:00Z"), n("3", "b", "t1", "2026-10-29T08:00:00Z"), n("4", "a", "t1", "2026-10-30T10:00:00Z")], ["b", "a", "c"]);
  expect(g.map((x) => x.agentId)).toEqual(["b", "a"]);
  expect(g[1]!.threads.map((t) => [t.thread, t.notes.map((x) => x.id)])).toEqual([["t1", ["4", "1"]], ["t2", ["2"]]]);
});

test("search ignores case and accents and needs every word", () => {
  const list = [n("1", "a", "t", "", "Ármány és SZERELEM"), n("2", "a", "t", "", "szerelem")];
  expect(searchNotes(list, "armany szerelem").map((x) => x.id)).toEqual(["1"]);
  expect(searchNotes(list, "  ")).toHaveLength(2);
});

test("only agent authors, once each", () => {
  expect(agentAuthors([{ author_id: "h" }, { author_id: "a" }, { author_id: "a" }, { author_id: "b" }], (id) => id !== "h")).toEqual(["a", "b"]);
});
