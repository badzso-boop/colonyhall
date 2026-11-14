// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { firstLine, groupTasks, neighbours, nextStatus, type Task } from "./tasks";

const task = (id: string, status: Task["status"], position: number): Task => ({
  id, title: id, description: "", status, position, status_changed_at: "", created_at: "", updated_at: "",
});

test("groupTasks keeps the owner's order in each group", () => {
  const g = groupTasks([task("a", "scheduled", 2), task("b", "started", 1), task("c", "scheduled", 1), task("d", "done", 1)]);
  expect(g.scheduled.map((t) => t.id)).toEqual(["c", "a"]);
  expect(g.started.map((t) => t.id)).toEqual(["b"]);
  expect(g.done.map((t) => t.id)).toEqual(["d"]);
});

test("nextStatus cycles", () => {
  expect(["scheduled", "started", "done"].map((s) => nextStatus(s as Task["status"]))).toEqual(["started", "done", "scheduled"]);
});

test("neighbours for moves within and into a group", () => {
  const list = [task("a", "scheduled", 1), task("b", "scheduled", 2), task("c", "scheduled", 3)];
  expect(neighbours(list, "c", 0)).toEqual({ before_id: "a" }); // c to the top
  expect(neighbours(list, "a", 1)).toEqual({ after_id: "b", before_id: "c" }); // a one down
  expect(neighbours(list, "x", 3)).toEqual({ after_id: "c" }); // a new one at the end
  expect(neighbours([], "x", 0)).toEqual({});
});

test("firstLine skips blank lines", () => {
  expect(firstLine("\n\n  Első sor\nmásodik")).toBe("Első sor");
  expect(firstLine("")).toBe("");
});
