import { applyActivity } from "./activity";

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
