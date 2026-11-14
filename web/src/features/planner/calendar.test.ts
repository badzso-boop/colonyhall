// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { dayKey } from "@/shared/lib";
import { eventDays, eventsByDay, layoutDay, minutesToNext, monthGrid, rangeTitle, shift, startOfWeek, viewRange, type CalEvent } from "./calendar";

const ev = (title: string, start: Date, end: Date, all_day = false): CalEvent => ({
  calendar_id: "c", title, start: start.toISOString(), end: end.toISOString(), all_day, location: "", description: "",
});
const at = (d: number, h: number, m = 0) => new Date(2026, 9, d, h, m);

test("the month grid starts on a Monday and has 42 days", () => {
  const g = monthGrid(new Date(2026, 9, 15));
  expect(g).toHaveLength(42);
  expect(dayKey(g[0]!)).toBe("2026-09-28");
  expect(g[0]!.getDay()).toBe(1);
  expect(dayKey(g[41]!)).toBe("2026-11-08");
  expect(dayKey(startOfWeek(new Date(2026, 10, 1)))).toBe("2026-10-26"); // a Sunday
});

test("ranges, steps and titles", () => {
  const r = viewRange("week", new Date(2026, 9, 30, 15));
  expect([dayKey(r.from), dayKey(r.to)]).toEqual(["2026-10-26", "2026-11-02"]);
  // October 2026's grid crosses the clock change: still at most 42 × 24 h.
  const oct = viewRange("month", new Date(2026, 9, 1));
  expect(oct.to.getTime() - oct.from.getTime()).toBeLessThanOrEqual(42 * 86_400_000);
  expect(dayKey(shift("month", new Date(2026, 0, 31), 1))).toBe("2026-02-01");
  expect(dayKey(shift("three", new Date(2026, 9, 30), -1))).toBe("2026-10-27");
  expect(rangeTitle("month", new Date(2026, 9, 3), "hu")).toBe("2026. október");
  expect(rangeTitle("week", new Date(2026, 9, 30), "hu")).toBe("okt. 26. – nov. 1.");
});

test("an all-day event's end is exclusive; events by day put all-day first", () => {
  expect(eventDays(ev("x", new Date(2026, 9, 30), new Date(2026, 10, 1), true))).toEqual(["2026-10-30", "2026-10-31"]);
  expect(eventDays(ev("y", at(30, 23), at(31, 1)))).toEqual(["2026-10-30", "2026-10-31"]);
  const m = eventsByDay([ev("late", at(30, 9), at(30, 10)), ev("allday", new Date(2026, 9, 30), new Date(2026, 9, 31), true)]);
  expect(m.get("2026-10-30")!.map((e) => e.title)).toEqual(["allday", "late"]);
});

test("overlapping events sit side by side; a separate one gets the full width", () => {
  const placed = layoutDay([ev("a", at(30, 9), at(30, 11)), ev("b", at(30, 10), at(30, 12)), ev("c", at(30, 10, 30), at(30, 10, 45)), ev("d", at(30, 14), at(30, 15))], new Date(2026, 9, 30));
  const by = Object.fromEntries(placed.map((p) => [p.event.title, p]));
  expect([by.a!.col, by.b!.col, by.c!.col, by.a!.cols]).toEqual([0, 1, 2, 3]);
  expect([by.d!.col, by.d!.cols, by.d!.top, by.d!.height]).toEqual([0, 1, 840, 60]);
  // Clipped at midnight.
  expect(layoutDay([ev("n", at(29, 23), at(30, 2))], new Date(2026, 9, 30))[0]).toMatchObject({ top: 0, height: 120 });
});

test("the badge: minutes to an event within the hour", () => {
  const now = at(30, 9);
  expect(minutesToNext([ev("a", at(30, 9, 12), at(30, 10)), ev("b", at(30, 9, 40), at(30, 10))], now)).toBe(12);
  expect(minutesToNext([ev("far", at(30, 11), at(30, 12)), ev("past", at(30, 8), at(30, 10))], now)).toBeNull();
});
