import type { Schemas } from "@/shared/api";

/**
 * Who may read the owner's tasks and calendar (docs/handoff/34, part 8).
 * Tasks: every agent (the default) or selected ones. Calendar: nobody (the
 * default) or selected agents.
 */
export type PlannerAccess = Schemas["PlannerAccess"];
export type AccessKind = "tasks" | "calendar";
export type AccessDraft = { mode: "all" | "none" | "selected"; ids: string[] };

export function accessDraft(a: PlannerAccess, kind: AccessKind): AccessDraft {
  if (kind === "tasks") return a.task_all ? { mode: "all", ids: [...a.task_agent_ids] } : { mode: "selected", ids: [...a.task_agent_ids] };
  return a.cal_agent_ids.length ? { mode: "selected", ids: [...a.cal_agent_ids] } : { mode: "none", ids: [] };
}

/** The full body for PUT /planner/access: the other half is left as it was. */
export function applyAccess(a: PlannerAccess, kind: AccessKind, d: AccessDraft): PlannerAccess {
  const ids = d.mode === "selected" ? [...new Set(d.ids)].sort() : [];
  if (kind === "tasks") return { ...a, task_all: d.mode === "all", task_agent_ids: d.mode === "all" ? a.task_agent_ids : ids };
  return { ...a, cal_agent_ids: ids };
}

export function toggleId(ids: string[], id: string): string[] {
  return ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id];
}

type Actor = Schemas["Actor"];

/** The agents to pick from: live ones, plus any already chosen (even archived), by handle. */
export function accessCandidates(actors: Iterable<Actor>, chosen: string[], query: string): Actor[] {
  const q = query.trim().toLowerCase();
  return [...actors]
    .filter((a) => a.kind === "agent" && (!a.archived_at || chosen.includes(a.id)))
    .filter((a) => !q || a.handle.toLowerCase().includes(q) || a.display_name.toLowerCase().includes(q))
    .sort((a, b) => a.handle.localeCompare(b.handle));
}
