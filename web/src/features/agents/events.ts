import type { QueryClient } from "@tanstack/react-query";
import { onActivity, onEvent, onResync, onUntypedEvent, type Schemas } from "@/shared/api";
import { agentKeys } from "./api";

type Overview = Schemas["AgentOverview"];

/** Pure: an inbox item arrived for the agent: one more, and the oldest stays (or is now). */
export function bumpInbox(o: Overview, at: string): Overview {
  if (!o.inbox) return o;
  return { ...o, inbox: { count: o.inbox.count + 1, oldest_at: o.inbox.oldest_at ?? at } };
}

/**
 * Live updates of the open overviews (docs/handoff/33, part 3): state and
 * inbox are patched in the cache; a finished run, a lease or a grant change
 * refetches only what it touches. Activity and progress live in activity.ts.
 */
export function registerOverviewEvents(qc: QueryClient): () => void {
  const patch = (id: string, f: (o: Overview) => Overview) => qc.setQueryData<Overview>(agentKeys.overview(id), (o) => (o ? f(o) : o));
  const runFinished = (agentId: string) => {
    void qc.invalidateQueries({ queryKey: agentKeys.overview(agentId) });
    void qc.invalidateQueries({ queryKey: agentKeys.runs(agentId) });
    void qc.invalidateQueries({ queryKey: agentKeys.usage });
  };
  const overviews = () => void qc.invalidateQueries({ queryKey: agentKeys.overviews });
  const offs = [
    onEvent("actor.state_changed", (e) => patch(e.data.actor_id, (o) => ({ ...o, state: e.data.state, actor: { ...o.actor, state: e.data.state } }))),
    onEvent("actor.profile_updated", (e) => void qc.invalidateQueries({ queryKey: agentKeys.overview(e.data.actor_id) })),
    onEvent("delivery.item_created", (e) => patch(e.data.recipient_id, (o) => bumpInbox(o, e.at))),
    // A reported run may carry the steps of an answer already shown (the step drawer).
    onUntypedEvent("run.reported", () => void qc.invalidateQueries({ queryKey: agentKeys.recentRuns })),
    // A run that ends leaves the place: its sessions, runs and usage changed.
    onActivity((f) => f.state !== "working" && runFinished(f.agent_id)),
    ...["delegation.grant_created", "delegation.grant_revoked"].map((t) => onUntypedEvent(t, () => void qc.invalidateQueries({ queryKey: agentKeys.grants }))),
    ...["lease.acquired", "lease.released", "question.asked", "question.answered", "question.expired", "question.withdrawn"].map((t) => onUntypedEvent(t, overviews)),
    onResync(() => void qc.invalidateQueries({ queryKey: agentKeys.all })),
  ];
  return () => offs.forEach((off) => off());
}
