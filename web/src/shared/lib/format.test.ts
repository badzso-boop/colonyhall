// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { dayKey, dayLabel, feedItems, fullTime, messageTime, relativeTime, sessionContext } from "./format";

// Local-time dates, so the tests hold in any time zone (as in round 30).
const now = new Date(2026, 9, 29, 15, 30, 0);
const words = { today: "Ma", yesterday: "Tegnap" };

test("dayKey is the local calendar day", () => {
  expect(dayKey(new Date(2026, 9, 29, 0, 0, 1))).toBe("2026-10-29");
  expect(dayKey(new Date(2026, 9, 28, 23, 59, 59))).toBe("2026-10-28");
  expect(dayKey(new Date(2026, 0, 5, 12))).toBe("2026-01-05");
  expect(dayKey("not a date")).toBe("");
});

test("messageTime shows the date with the time, and the year only for another year", () => {
  const same = messageTime(new Date(2026, 9, 28, 9, 5, 7), now, "en-GB");
  expect(same).toMatch(/28/);
  expect(same).toMatch(/Oct/);
  expect(same).toMatch(/09:05:07/);
  expect(same).not.toMatch(/2026/);
  expect(messageTime(new Date(2025, 11, 31, 23), now, "en-GB")).toMatch(/2025/);
  const hu = messageTime(new Date(2026, 9, 28, 9, 5, 7), now, "hu-HU");
  expect(hu).toMatch(/okt/);
  expect(messageTime("garbage", now, "hu-HU")).toBe("");
});

test("fullTime names the weekday and the year", () => {
  const s = fullTime(new Date(2026, 9, 27, 8), "en-GB");
  expect(s).toMatch(/Tuesday/);
  expect(s).toMatch(/2026/);
  expect(s).toMatch(/October/);
});

test("dayLabel: today and yesterday by word, other days by date and weekday", () => {
  expect(dayLabel(new Date(2026, 9, 29, 0), now, "en-GB", words)).toBe("Ma");
  expect(dayLabel(new Date(2026, 9, 28, 23, 59), now, "en-GB", words)).toBe("Tegnap");
  const older = dayLabel(new Date(2026, 9, 27, 12), now, "en-GB", words);
  expect(older).toMatch(/Tuesday/);
  expect(older).toMatch(/27 October/);
  expect(older).not.toMatch(/2026/);
  expect(dayLabel(new Date(2025, 9, 27, 12), now, "en-GB", words)).toMatch(/2025/);
  expect(dayLabel(new Date(2026, 9, 31, 22), new Date(2026, 10, 1, 10), "en-GB", words)).toBe("Tegnap");
});

test("feedItems puts one separator before the first message of each day", () => {
  const msgs = [
    { id: "a", created_at: new Date(2026, 9, 27, 10).toISOString() },
    { id: "b", created_at: new Date(2026, 9, 27, 18).toISOString() },
    { id: "c", created_at: new Date(2026, 9, 28, 0, 0, 1).toISOString() },
    { id: "d", created_at: new Date(2026, 9, 29, 9).toISOString() },
    { id: "e", created_at: new Date(2026, 9, 29, 9, 1).toISOString() },
  ];
  const items = feedItems(msgs, now, "en-GB", words);
  expect(items.map((i) => (i.separator ? `|${i.day}` : i.message.id))).toEqual(["|2026-10-27", "a", "b", "|2026-10-28", "c", "|2026-10-29", "d", "e"]);
  expect(items[3]).toMatchObject({ label: "Tegnap" });
  expect(items[5]).toMatchObject({ label: "Ma" });
  expect(feedItems([], now, "en-GB", words)).toEqual([]);
});

test("relativeTime: seconds, minutes, hours, then the date", () => {
  expect(relativeTime(new Date(now.getTime() - 120_000), now, "en-GB")).toBe("2 minutes ago");
  expect(relativeTime(new Date(now.getTime() - 3 * 3600_000), now, "en-GB")).toBe("3 hours ago");
  expect(relativeTime(new Date(2026, 9, 20, 9), now, "en-GB")).toMatch(/20 Oct/);
});

test("sessionContext compares the measured context, not the run's summed usage", () => {
  const c = sessionContext({ last_run_tokens: 112000, context_tokens: 40000, max_session_tokens: 150000 });
  expect(c).toEqual({ text: "40k / 150k", known: true, near: false, runTokens: "112k" });
  expect(sessionContext({ last_run_tokens: 1, context_tokens: 125000, max_session_tokens: 150000 }).near).toBe(true);
  expect(sessionContext({ last_run_tokens: 200000, max_session_tokens: 150000 })).toMatchObject({ text: "– / 150k", known: false, near: false });
  expect(sessionContext({ last_run_tokens: 0, context_tokens: 5000, max_session_tokens: 0 }).text).toBe("5k / ∞");
});
