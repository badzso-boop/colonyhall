import { create } from "zustand";
import { onActivity, type ActivityFrame } from "@/shared/api";

/** Where an agent is working: a channel, or a thread within it. */
export type WorkPlace = { channelId: string; threadRootId?: string };

type State = { working: Record<string, WorkPlace[]> };

const placeKey = (p: WorkPlace) => `${p.channelId}/${p.threadRootId ?? ""}`;

/** Pure: apply one agent.activity frame to the working map. */
export function applyActivity(working: Record<string, WorkPlace[]>, f: ActivityFrame): Record<string, WorkPlace[]> {
  const place: WorkPlace = { channelId: f.channel_id, ...(f.thread_root_id ? { threadRootId: f.thread_root_id } : {}) };
  const current = (working[f.agent_id] ?? []).filter((p) => placeKey(p) !== placeKey(place));
  const next = f.state === "working" ? [...current, place] : current;
  const out = { ...working };
  if (next.length) out[f.agent_id] = next;
  else delete out[f.agent_id];
  return out;
}

/** Live "who works where", from the ephemeral agent.activity frames. */
export const useActivity = create<State>(() => ({ working: {} }));

export function registerAgentEvents(): () => void {
  return onActivity((f) => useActivity.setState((s) => ({ working: applyActivity(s.working, f) })));
}
