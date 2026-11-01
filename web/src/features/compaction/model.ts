import type { Schemas } from "@/shared/api";

export type Compaction = Schemas["Compaction"];
export type UnitStatus = Schemas["CompactionUnitStatus"];

/**
 * A compaction event's payload. The contract has no Event schema for
 * compaction.* yet, so the payload is checked here, not cast.
 */
export type CompactionEvent = { compactionId: string; channelId: string; threadRootId: string | null; errorCode: string | null };

export function parseCompactionEvent(data: unknown): CompactionEvent | null {
  if (!data || typeof data !== "object") return null;
  const d = data as Record<string, unknown>;
  if (typeof d.compaction_id !== "string" || typeof d.channel_id !== "string") return null;
  return {
    compactionId: d.compaction_id,
    channelId: d.channel_id,
    threadRootId: typeof d.thread_root_id === "string" && d.thread_root_id ? d.thread_root_id : null,
    errorCode: typeof d.error_code === "string" && d.error_code ? d.error_code : null,
  };
}

/** A compaction the viewer saw start or fail in this session (the spinner and error cards). */
export type Live = { id: string; channelId: string; threadRootId: string | null; state: "pending" | "failed"; errorCode: string | null };

/**
 * created → a pending card; done → gone (the summary message replaces it);
 * failed → an error card. compaction.failed carries no thread, so a
 * failed one keeps the place it had while pending.
 */
export function applyCompactionEvent(live: Record<string, Live>, type: string, e: CompactionEvent): Record<string, Live> {
  const next = { ...live };
  if (type === "compaction.created") {
    next[e.compactionId] = { id: e.compactionId, channelId: e.channelId, threadRootId: e.threadRootId, state: "pending", errorCode: null };
  } else if (type === "compaction.done") {
    delete next[e.compactionId];
  } else if (type === "compaction.failed") {
    const prev = live[e.compactionId];
    next[e.compactionId] = { id: e.compactionId, channelId: e.channelId, threadRootId: prev?.threadRootId ?? e.threadRootId, state: "failed", errorCode: e.errorCode };
  } else {
    return live;
  }
  return next;
}

/** The live cards of one place: the main stream (threadRootId null) or a thread. */
export function liveHere(live: Record<string, Live>, channelId: string, threadRootId: string | null): Live[] {
  return Object.values(live).filter((l) => l.channelId === channelId && l.threadRootId === threadRootId);
}

/** The first non-empty lines of the summary for the card (markdown heading marks dropped). */
export function previewLines(markdown: string, n = 3): string[] {
  return markdown
    .split("\n")
    .map((l) => l.replace(/^#{1,6}\s+/, "").trim())
    .filter((l) => l !== "")
    .slice(0, n);
}
