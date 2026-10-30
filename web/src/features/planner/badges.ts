import type { WindowKind } from "@/shared/windows";
import { useTasks } from "./api";
import { useCalendarBadge } from "./calendarBadge";

/** The minimised circle's badge: started tasks; the minutes to the next event within the hour. */
export function useWindowBadge(kind: WindowKind): { text: string | null; news: boolean } {
  const tasks = useTasks();
  const cal = useCalendarBadge(kind === "calendar");
  if (kind === "tasks") {
    const started = (tasks.data ?? []).filter((t) => t.status === "started").length;
    return { text: started ? String(started) : null, news: false };
  }
  if (kind === "calendar") return cal;
  return { text: null, news: false };
}
