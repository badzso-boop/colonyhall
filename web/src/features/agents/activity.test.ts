// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { applyActivity, applyProgress, stepKey } from "./activity";

const frame = (agent: string, state: string, channel = "c1", thread?: string) => ({
  type: "agent.activity" as const, agent_id: agent, channel_id: channel, state, ...(thread ? { thread_root_id: thread } : {}),
});

test("working adds the place once, idle removes it, and an idle agent disappears", () => {
  let w = applyActivity({}, frame("a", "working"));
  w = applyActivity(w, frame("a", "working"));
  w = applyActivity(w, frame("a", "working", "c1", "t1"));
  expect(w.a).toEqual([{ channelId: "c1" }, { channelId: "c1", threadRootId: "t1" }]);
  w = applyActivity(w, frame("a", "idle"));
  expect(w.a).toEqual([{ channelId: "c1", threadRootId: "t1" }]);
  w = applyActivity(w, frame("a", "idle", "c1", "t1"));
  expect(w).toEqual({});
});

test("progress keeps the newest step per agent and place, and drops it when done", () => {
  const step = (label: string) => ({ kind: "command" as const, label, state: "active" as const, started_at: "2026-10-30T10:00:00Z" });
  const f = (steps: ReturnType<typeof step>[], done = false) => ({ type: "agent.progress" as const, agent_id: "a", channel_id: "c1", run_key: "r", steps, done });
  let s = applyProgress({}, f([step("go build"), step("go test")]));
  expect(s[stepKey("a", { channelId: "c1" })]).toEqual({ label: "go test", kind: "command", startedAt: "2026-10-30T10:00:00Z" });
  s = applyProgress(s, f([step("x")], true));
  expect(s).toEqual({});
});
