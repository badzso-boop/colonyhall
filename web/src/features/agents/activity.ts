import { create } from "zustand";
import { onActivity, onProgress, type ActivityFrame, type ProgressFrame } from "@/shared/api";

/** Where an agent is working: a channel, or a thread within it. */
export type WorkPlace = { channelId: string; threadRootId?: string };

/** The latest live step of an agent's run in a place, and when the run started. */
export type LiveStep = { label: string; kind: string; startedAt: string };

type State = { working: Record<string, WorkPlace[]>; steps: Record<string, LiveStep> };

const placeKey = (p: WorkPlace) => `${p.channelId}/${p.threadRootId ?? ""}`;

/** The key of a live step: agent and place. */
export const stepKey = (agentId: string, p: WorkPlace) => `${agentId}@${placeKey(p)}`;

/** Pure: apply one agent.progress frame (the newest step; removed when done). */
export function applyProgress(steps: Record<string, LiveStep>, f: ProgressFrame): Record<string, LiveStep> {
  const key = stepKey(f.agent_id, { channelId: f.channel_id, ...(f.thread_root_id ? { threadRootId: f.thread_root_id } : {}) });
  const out = { ...steps };
  const last = f.steps[f.steps.length - 1];
  if (f.done || !last) delete out[key];
  else out[key] = { label: last.label, kind: last.kind, startedAt: f.steps[0]!.started_at };
  return out;
}

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
export const useActivity = create<State>(() => ({ working: {}, steps: {} }));

export function registerAgentEvents(): () => void {
  const offs = [
    onActivity((f) =>
      useActivity.setState((s) => {
        const working = applyActivity(s.working, f);
        // An idle agent has no live step in that place any more.
        const steps = f.state === "working" ? s.steps : applyProgress(s.steps, { ...f, type: "agent.progress", run_key: "", steps: [], done: true });
        return { working, steps };
      }),
    ),
    onProgress((f) => useActivity.setState((s) => ({ steps: applyProgress(s.steps, f) }))),
  ];
  return () => offs.forEach((off) => off());
}
