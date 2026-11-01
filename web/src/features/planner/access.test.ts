import type { Schemas } from "@/shared/api";
import { accessCandidates, accessDraft, applyAccess, toggleId, type PlannerAccess } from "./access";

const base: PlannerAccess = { task_all: true, task_agent_ids: [], cal_agent_ids: [] };

test("the defaults: tasks for every agent, the calendar for nobody", () => {
  expect(accessDraft(base, "tasks")).toEqual({ mode: "all", ids: [] });
  expect(accessDraft(base, "calendar")).toEqual({ mode: "none", ids: [] });
  expect(accessDraft({ ...base, task_all: false, task_agent_ids: ["a"] }, "tasks")).toEqual({ mode: "selected", ids: ["a"] });
  expect(accessDraft({ ...base, cal_agent_ids: ["b"] }, "calendar")).toEqual({ mode: "selected", ids: ["b"] });
});

test("applyAccess changes one half and keeps the other", () => {
  const withCal = { ...base, cal_agent_ids: ["c"] };
  expect(applyAccess(withCal, "tasks", { mode: "selected", ids: ["b", "a", "a"] })).toEqual({ task_all: false, task_agent_ids: ["a", "b"], cal_agent_ids: ["c"] });
  expect(applyAccess(withCal, "calendar", { mode: "none", ids: ["x"] })).toEqual({ ...base, cal_agent_ids: [] });
  expect(applyAccess({ ...base, task_all: false, task_agent_ids: ["a"] }, "tasks", { mode: "all", ids: [] })).toEqual({ task_all: true, task_agent_ids: ["a"], cal_agent_ids: [] });
});

test("toggleId", () => {
  expect(toggleId(["a"], "b")).toEqual(["a", "b"]);
  expect(toggleId(["a", "b"], "a")).toEqual(["b"]);
});

const actor = (id: string, kind: string, handle: string, extra = {}) => ({ id, kind, handle, display_name: handle, role: "member", state: "active", created_at: "", ...extra }) as Schemas["Actor"];

test("accessCandidates: agents only, archived ones only when already chosen, filtered and sorted", () => {
  const list = [actor("1", "agent", "zeta"), actor("2", "human", "anna"), actor("3", "agent", "alpha"), actor("4", "agent", "old", { archived_at: "2026-10-01T00:00:00Z" })];
  expect(accessCandidates(list, [], "").map((a) => a.handle)).toEqual(["alpha", "zeta"]);
  expect(accessCandidates(list, ["4"], "").map((a) => a.handle)).toEqual(["alpha", "old", "zeta"]);
  expect(accessCandidates(list, [], "ZE").map((a) => a.handle)).toEqual(["zeta"]);
});
