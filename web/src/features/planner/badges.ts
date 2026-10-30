import type { WindowKind } from "@/shared/windows";

/** The minimised circle's badge for tasks and calendar (filled in when their data exists). */
export function useWindowBadge(kind: WindowKind): { text: string | null; news: boolean } {
  void kind;
  return { text: null, news: false };
}
