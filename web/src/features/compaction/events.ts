import type { QueryClient } from "@tanstack/react-query";
import { onResync, onUntypedEvent } from "@/shared/api";
import { compactionKeys } from "./api";
import { parseCompactionEvent } from "./model";
import { useLiveCompactions } from "./store";

/**
 * compaction.created / done / failed (round 21): the live cards and the
 * counters. The summary itself arrives as a system message
 * (message.created, the conversation feature).
 */
export function registerCompactionEvents(qc: QueryClient): () => void {
  const offs = ["compaction.created", "compaction.done", "compaction.failed"].map((type) =>
    onUntypedEvent(type, (data) => {
      const e = parseCompactionEvent(data);
      if (!e) return;
      useLiveCompactions.getState().apply(type, e);
      void qc.invalidateQueries({ queryKey: [...compactionKeys.all, "status", e.channelId] });
    }),
  );
  offs.push(onUntypedEvent("compaction.settings_updated", () => void qc.invalidateQueries({ queryKey: compactionKeys.all })));
  offs.push(onResync(() => void qc.invalidateQueries({ queryKey: compactionKeys.all })));
  return () => offs.forEach((off) => off());
}
