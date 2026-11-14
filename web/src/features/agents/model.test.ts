// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Schemas } from "@/shared/api";
import { bumpInbox } from "./events";
import { contextPercent, displayState, firstLines, formatDuration, grantsOf, lastSevenDays, nearRotation, OFFLINE_AFTER_MS } from "./model";

const now = new Date("2026-11-01T10:00:00Z");

test("display state: archived and stopped win, then paused, working; a quiet machine is offline", () => {
  const base = { archived: false, state: "active", working: false };
  expect(displayState({ ...base, archived: true, working: true }, now)).toBe("archived");
  expect(displayState({ ...base, state: "stopped", working: true }, now)).toBe("stopped");
  expect(displayState({ ...base, state: "paused", working: true }, now)).toBe("paused");
  expect(displayState({ ...base, working: true }, now)).toBe("working");
  expect(displayState(base, now)).toBe("idle");
  expect(displayState({ ...base, hostLastSeen: new Date(now.getTime() - OFFLINE_AFTER_MS - 1).toISOString() }, now)).toBe("offline");
  expect(displayState({ ...base, hostLastSeen: "" }, now)).toBe("offline");
  expect(displayState({ ...base, hostLastSeen: now.toISOString() }, now)).toBe("idle");
});

test("context percent and rotation", () => {
  expect(contextPercent({ context_tokens: 80_000, max_session_tokens: 100_000 })).toBe(80);
  expect(contextPercent({ context_tokens: 0, max_session_tokens: 100_000 })).toBe(0);
  expect(contextPercent({ context_tokens: 5, max_session_tokens: 0 })).toBe(0);
  expect(nearRotation({ context_tokens: 85_000, max_session_tokens: 100_000 })).toBe(false);
  expect(nearRotation({ context_tokens: 95_000, max_session_tokens: 100_000 })).toBe(true);
});

test("seven days, oldest first, missing days zero, other agents ignored", () => {
  const row = (agent_id: string, day: string, total_tokens: number, run_count: number) =>
    ({ agent_id, day, total_tokens, run_count, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, error_count: 0 }) satisfies Schemas["DayUsage"];
  const days = lastSevenDays([row("a", "2026-11-01", 100, 2), row("a", "2026-10-28", 50, 1), row("b", "2026-11-01", 999, 9)], "a", "2026-11-01");
  expect(days.map((d) => d.day)).toEqual(["2026-10-26", "2026-10-27", "2026-10-28", "2026-10-29", "2026-10-30", "2026-10-31", "2026-11-01"]);
  expect(days[6]).toEqual({ day: "2026-11-01", tokens: 100, runs: 2 });
  expect(days[2]!.tokens).toBe(50);
  expect(days[0]!.tokens).toBe(0);
});

test("grants of an agent: active ones it is part of, soonest expiry first", () => {
  const g = (id: string, a: string, b: string, exp: string, revoked?: string) =>
    ({ id, agent_a_id: a, agent_b_id: b, effective_expires_at: exp, revoked_at: revoked ?? null }) as Schemas["DelegationGrant"];
  expect(grantsOf([g("1", "x", "y", "2026-11-01T12:00:00Z"), g("2", "z", "x", "2026-11-01T11:00:00Z"), g("3", "x", "y", "2026-11-01T10:00:00Z", "now"), g("4", "y", "z", "2026")], "x").map((x) => x.id)).toEqual(["2", "1"]);
});

test("first lines and durations", () => {
  expect(firstLines("a\n\nb\nc\nd\ne", 3)).toEqual({ lines: ["a", "b", "c"], more: true });
  expect(firstLines("", 3)).toEqual({ lines: [], more: false });
  expect(formatDuration(64_000)).toBe("1:04");
  expect(formatDuration(3_725_000)).toBe("1:02:05");
});

test("an inbox item counts one more and keeps the oldest time", () => {
  const o = { actor: {}, state: "active", inbox: { count: 2, oldest_at: "t0" } } as Schemas["AgentOverview"];
  expect(bumpInbox(o, "t1").inbox).toEqual({ count: 3, oldest_at: "t0" });
  expect(bumpInbox({ ...o, inbox: { count: 0 } }, "t1").inbox).toEqual({ count: 1, oldest_at: "t1" });
  expect(bumpInbox({ ...o, inbox: undefined }, "t1").inbox).toBeUndefined();
});
