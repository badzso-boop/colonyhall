import { create } from "zustand";
import { applyCompactionEvent, type Compaction, type CompactionEvent, type Live } from "./model";

type State = {
  live: Record<string, Live>;
  apply: (type: string, e: CompactionEvent) => void;
  /** A compaction this viewer started (the POST answer): shown as pending until its event. */
  add: (c: Compaction) => void;
  dismiss: (id: string) => void;
};

export const useLiveCompactions = create<State>((set) => ({
  live: {},
  apply: (type, e) => set((s) => ({ live: applyCompactionEvent(s.live, type, e) })),
  add: (c) =>
    set((s) =>
      c.state === "pending" && !s.live[c.id]
        ? { live: applyCompactionEvent(s.live, "compaction.created", { compactionId: c.id, channelId: c.channel_id, threadRootId: c.thread_root_id ?? null, errorCode: null }) }
        : s,
    ),
  dismiss: (id) =>
    set((s) => {
      const live = { ...s.live };
      delete live[id];
      return { live };
    }),
}));
