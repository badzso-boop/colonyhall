// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { TFunction } from "i18next";
import { buildCron, describeRule, parseCron, quickChips, type Rule } from "./rule";

// A stand-in for t(): echoes the key and its options.
const t = ((key: string, o?: Record<string, unknown>) => `${key} ${JSON.stringify(o)}`) as unknown as TFunction<"schedules">;

test("each mode builds the cron (or the one-off time) the server reads", () => {
  expect(buildCron({ mode: "once", date: "2026-11-02", time: "9:30" })).toEqual({ at: "2026-11-02 09:30" });
  expect(buildCron({ mode: "daily", time: "09:00", workdays: false })).toEqual({ cron: "0 9 * * *" });
  expect(buildCron({ mode: "daily", time: "07:45", workdays: true })).toEqual({ cron: "45 7 * * 1-5" });
  expect(buildCron({ mode: "weekly", days: [1, 3, 5], time: "18:30" })).toEqual({ cron: "30 18 * * 1,3,5" });
  expect(buildCron({ mode: "weekly", days: [6, 0, 1, 2, 3], time: "08:00" })).toEqual({ cron: "0 8 * * 0-3,6" });
  expect(buildCron({ mode: "weekly", days: [2, 1], time: "08:00" })).toEqual({ cron: "0 8 * * 1,2" });
  expect(buildCron({ mode: "monthly", day: 31, time: "23:59" })).toEqual({ cron: "59 23 31 * *" });
  expect(buildCron({ mode: "advanced", cron: " */15  9-17 * * 1-5 " })).toEqual({ cron: "*/15 9-17 * * 1-5" });
});

test("incomplete choices build nothing", () => {
  expect(buildCron({ mode: "weekly", days: [], time: "08:00" })).toBeNull();
  expect(buildCron({ mode: "daily", time: "25:00", workdays: false })).toBeNull();
  expect(buildCron({ mode: "once", date: "", time: "09:00" })).toBeNull();
  expect(buildCron({ mode: "advanced", cron: "  " })).toBeNull();
});

test("the builder's own crons read back into its choices; others stay raw", () => {
  const rules: Rule[] = [
    { mode: "daily", time: "09:00", workdays: false },
    { mode: "daily", time: "07:45", workdays: true },
    { mode: "weekly", days: [1, 3, 5], time: "18:30" },
    { mode: "weekly", days: [1, 2, 3, 6, 0], time: "08:00" },
    { mode: "monthly", day: 1, time: "06:00" },
  ];
  for (const r of rules) {
    const c = buildCron(r) as { cron: string };
    expect(parseCron(c.cron)).toEqual(r);
  }
  expect(parseCron("0 9 * * 1,2,3,4,5")).toEqual({ mode: "daily", time: "09:00", workdays: true });
  expect(parseCron("*/15 9-17 * * 1-5")).toEqual({ mode: "advanced", cron: "*/15 9-17 * * 1-5" });
  expect(parseCron("@daily")).toEqual({ mode: "advanced", cron: "@daily" });
  expect(parseCron("0 9 1 1 *").mode).toBe("advanced");
});

test("the summary names the days Monday first", () => {
  expect(describeRule({ mode: "weekly", days: [3, 1], time: "18:30" }, t, "hu")).toBe('rule.weekly {"days":"hétfő és szerda","time":"18:30"}');
  expect(describeRule({ mode: "daily", time: "09:00", workdays: true }, t, "hu")).toBe('rule.workdays {"time":"09:00"}');
});

test("quick chips from a Thursday afternoon", () => {
  const chips = quickChips(new Date(2026, 9, 29, 14, 50));
  expect(chips.map((c) => [c.id, c.date, c.time])).toEqual([
    ["today", "2026-10-29", "18:00"],
    ["tomorrow", "2026-10-30", "09:00"],
    ["monday", "2026-11-02", "09:00"],
    ["hour", "2026-10-29", "16:00"],
  ]);
  // In the evening "today 18:00" is gone; on a Monday "Monday" means next week.
  const mon = quickChips(new Date(2026, 10, 2, 19, 0));
  expect(mon.map((c) => c.id)).toEqual(["tomorrow", "monday", "hour"]);
  expect(mon.find((c) => c.id === "monday")!.date).toBe("2026-11-09");
});
