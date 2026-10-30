import type { Schemas } from "@/shared/api";
import { dayKey } from "@/shared/lib";

// The calendar's date arithmetic (docs/handoff/32, part 4). Everything is in
// the viewer's local time; weeks start on Monday.

export type CalEvent = Schemas["PlannerEvent"];
export type CalView = "month" | "week" | "day" | "three";

export function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

export function addDays(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n, d.getHours(), d.getMinutes());
}

/** The Monday on or before d. */
export function startOfWeek(d: Date): Date {
  const day = startOfDay(d);
  return addDays(day, -((day.getDay() + 6) % 7));
}

/** The 42 days (6 weeks from a Monday) of the month grid around anchor. */
export function monthGrid(anchor: Date): Date[] {
  const first = startOfWeek(new Date(anchor.getFullYear(), anchor.getMonth(), 1));
  return Array.from({ length: 42 }, (_, i) => addDays(first, i));
}

const spans: Record<Exclude<CalView, "month">, number> = { week: 7, three: 3, day: 1 };

/** The days a view shows. */
export function viewDays(view: CalView, anchor: Date): Date[] {
  if (view === "month") return monthGrid(anchor);
  const first = view === "week" ? startOfWeek(anchor) : startOfDay(anchor);
  return Array.from({ length: spans[view] }, (_, i) => addDays(first, i));
}

export const MaxRangeMs = 42 * 86_400_000;

/**
 * The [from, to) instant range to ask /planner/events for. The server allows
 * 42 × 24 hours; six weeks across the autumn clock change are an hour longer,
 * so the last hour is cut off.
 */
export function viewRange(view: CalView, anchor: Date): { from: Date; to: Date } {
  const days = viewDays(view, anchor);
  const from = days[0]!;
  const to = addDays(days[days.length - 1]!, 1);
  return { from, to: new Date(Math.min(to.getTime(), from.getTime() + MaxRangeMs)) };
}

/** The anchor one step back (-1) or forward (+1). */
export function shift(view: CalView, anchor: Date, dir: -1 | 1): Date {
  if (view === "month") return new Date(anchor.getFullYear(), anchor.getMonth() + dir, 1);
  return addDays(startOfDay(anchor), dir * (view === "week" ? 7 : view === "three" ? 3 : 1));
}

/** "2026. október", "okt. 26 – nov. 1.", "csütörtök, okt. 30." in the UI language. */
export function rangeTitle(view: CalView, anchor: Date, locale: string): string {
  if (view === "month") return anchor.toLocaleDateString(locale, { year: "numeric", month: "long" });
  if (view === "day") return anchor.toLocaleDateString(locale, { weekday: "long", month: "short", day: "numeric" });
  const days = viewDays(view, anchor);
  const f = (d: Date) => d.toLocaleDateString(locale, { month: "short", day: "numeric" });
  return `${f(days[0]!)} – ${f(days[days.length - 1]!)}`;
}

/** The local days an event touches; an all-day event's end is exclusive, a timed one's too. */
export function eventDays(e: CalEvent): string[] {
  const start = new Date(e.start);
  const end = new Date(e.end);
  const last = end.getTime() > start.getTime() ? new Date(end.getTime() - 1) : start;
  const out: string[] = [];
  for (let d = startOfDay(start); d.getTime() <= last.getTime() && out.length < 400; d = addDays(d, 1)) out.push(dayKey(d));
  return out;
}

/** Events by local day, all-day first, then by start time. */
export function eventsByDay(events: CalEvent[]): Map<string, CalEvent[]> {
  const out = new Map<string, CalEvent[]>();
  for (const e of events) for (const k of eventDays(e)) out.set(k, [...(out.get(k) ?? []), e]);
  for (const list of out.values()) list.sort((a, b) => Number(b.all_day) - Number(a.all_day) || a.start.localeCompare(b.start) || a.title.localeCompare(b.title));
  return out;
}

export type Placed = { event: CalEvent; top: number; height: number; col: number; cols: number };

/**
 * The timed events of one day on the hour grid: top and height in minutes
 * from midnight (clipped to the day), overlapping events side by side.
 */
export function layoutDay(events: CalEvent[], day: Date): Placed[] {
  const dayStart = startOfDay(day).getTime();
  const dayEnd = addDays(startOfDay(day), 1).getTime();
  const items = events
    .filter((e) => !e.all_day)
    .map((e) => {
      const s = Math.max(new Date(e.start).getTime(), dayStart);
      const en = Math.min(Math.max(new Date(e.end).getTime(), new Date(e.start).getTime() + 15 * 60_000), dayEnd);
      return { event: e, s, en };
    })
    .filter((x) => x.en > x.s)
    .sort((a, b) => a.s - b.s || b.en - a.en);
  const out: Placed[] = [];
  // Clusters of transitively overlapping events share one column count.
  let cluster: { item: (typeof items)[number]; col: number }[] = [];
  let clusterEnd = -Infinity;
  const flush = () => {
    const cols = Math.max(1, ...cluster.map((c) => c.col + 1));
    for (const c of cluster)
      out.push({ event: c.item.event, top: (c.item.s - dayStart) / 60_000, height: Math.max(15, (c.item.en - c.item.s) / 60_000), col: c.col, cols });
    cluster = [];
  };
  for (const item of items) {
    if (item.s >= clusterEnd) {
      flush();
      clusterEnd = -Infinity;
    }
    const used = new Set(cluster.filter((c) => c.item.en > item.s).map((c) => c.col));
    let col = 0;
    while (used.has(col)) col++;
    cluster.push({ item, col });
    clusterEnd = Math.max(clusterEnd, item.en);
  }
  flush();
  return out;
}

/** The minutes to the next timed event that starts within 60 minutes, else null. */
export function minutesToNext(events: CalEvent[], now: Date): number | null {
  let best: number | null = null;
  for (const e of events) {
    if (e.all_day) continue;
    const m = Math.ceil((new Date(e.start).getTime() - now.getTime()) / 60_000);
    if (m >= 0 && m <= 60 && (best === null || m < best)) best = m;
  }
  return best;
}
