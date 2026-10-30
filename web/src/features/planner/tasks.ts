import type { Schemas } from "@/shared/api";

export type Task = Schemas["PlannerTask"];
export type Status = Task["status"];

export const statusOrder: Status[] = ["started", "scheduled", "done"];

/** The three groups, each in the owner's order (position). */
export function groupTasks(tasks: Task[]): Record<Status, Task[]> {
  const out: Record<Status, Task[]> = { started: [], scheduled: [], done: [] };
  for (const t of tasks) out[t.status].push(t);
  for (const s of statusOrder) out[s].sort((a, b) => a.position - b.position);
  return out;
}

/** The status checkbox of the floating window: ütemezve → elkezdve → befejezve → ütemezve. */
export function nextStatus(s: Status): Status {
  return s === "scheduled" ? "started" : s === "started" ? "done" : "scheduled";
}

/**
 * The neighbours for a move: the task lands at `index` in `list` (the
 * target group, without the moved task). The server places it between
 * after_id and before_id; either is absent at the ends.
 */
export function neighbours(list: Task[], movedId: string, index: number): { after_id?: string; before_id?: string } {
  const rest = list.filter((t) => t.id !== movedId);
  const i = Math.max(0, Math.min(index, rest.length));
  return { ...(i > 0 ? { after_id: rest[i - 1]!.id } : {}), ...(i < rest.length ? { before_id: rest[i]!.id } : {}) };
}

/** The first line of a description, for cards. */
export function firstLine(text: string): string {
  return text.split("\n").find((l) => l.trim())?.trim() ?? "";
}
