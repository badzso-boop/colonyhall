// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { describe, expect, it } from "vitest";
import { fromLocalInput, reminderTime, zonedTime } from "./model";

const tz = "Europe/Budapest";

describe("reminder times in the person's time zone", () => {
  it("converts a wall-clock time in winter and summer", () => {
    expect(zonedTime(2026, 11, 15, 9, 0, tz).toISOString()).toBe("2026-11-15T08:00:00.000Z");
    expect(zonedTime(2026, 7, 1, 9, 0, tz).toISOString()).toBe("2026-07-01T07:00:00.000Z");
    expect(fromLocalInput("2026-11-15T09:30", tz)?.toISOString()).toBe("2026-11-15T08:30:00.000Z");
    expect(fromLocalInput("soon", tz)).toBeNull();
  });

  it("offers in 20 minutes, in an hour, tomorrow 9:00 and next Monday 9:00", () => {
    // Saturday 2026-11-14 23:30 in Budapest (22:30 UTC).
    const now = new Date("2026-11-14T22:30:00Z");
    expect(reminderTime("20m", now, tz).toISOString()).toBe("2026-11-14T22:50:00.000Z");
    expect(reminderTime("1h", now, tz).toISOString()).toBe("2026-11-14T23:30:00.000Z");
    expect(reminderTime("tomorrow", now, tz).toISOString()).toBe("2026-11-15T08:00:00.000Z");
    expect(reminderTime("monday", now, tz).toISOString()).toBe("2026-11-16T08:00:00.000Z");
  });

  it("uses the zone's date, not UTC's, near midnight", () => {
    // 00:30 Budapest on Sunday is still Saturday in UTC.
    const now = new Date("2026-11-14T23:30:00Z");
    expect(reminderTime("tomorrow", now, tz).toISOString()).toBe("2026-11-16T08:00:00.000Z");
    // On a Monday, "next Monday" is a week later.
    expect(reminderTime("monday", new Date("2026-11-16T10:00:00Z"), tz).toISOString()).toBe("2026-11-23T08:00:00.000Z");
  });

  it("keeps the wall-clock time across a DST change", () => {
    expect(zonedTime(2026, 3, 29, 9, 0, tz).toISOString()).toBe("2026-03-29T07:00:00.000Z");
    expect(zonedTime(2026, 3, 28, 9, 0, tz).toISOString()).toBe("2026-03-28T08:00:00.000Z");
  });
});
