import type { Schemas } from "@/shared/api";

export type Approval = Schemas["ApprovalRequest"];
export type Delegation = Schemas["Delegation"];
export type Grant = Schemas["DelegationGrant"];

/**
 * The answers the server understands on a delegation's approval question
 * (its options, internal/modules/delegation). Protocol values, not UI text:
 * the buttons show translated labels.
 */
export const DELEGATION_APPROVE = "Jóváhagyom";
export const DELEGATION_DENY = "Elutasítom";

/** The delegation a message started: the source agent's message (or its Q message, by client id). */
export function delegationOf(list: Delegation[], m: { id: string; client_msg_id?: string }): Delegation | undefined {
  return list.find((d) => d.message_id === m.id || (m.client_msg_id !== undefined && m.client_msg_id === `q-${d.question_id}`));
}

/**
 * The active grants shown in a conversation (round 25): in a thread, the
 * grants of that thread; in the main stream of a DM, the DM's own grants
 * (a channel's main stream has none: grants are per DM or per thread).
 */
export function grantsHere(grants: Grant[], channelId: string, threadRootId: string | null, isDm: boolean, now: Date): Grant[] {
  return grants.filter((g) => {
    if (g.channel_id !== channelId || g.revoked_at) return false;
    if (new Date(g.effective_expires_at).getTime() <= now.getTime()) return false;
    return threadRootId ? g.thread_root_id === threadRootId : isDm && !g.thread_root_id;
  });
}

const summaryKeys = ["command", "file_path", "path", "url", "pattern", "query", "description", "prompt"];

/**
 * A one-line summary of a tool's input (already masked by the server): the
 * first telling field of a JSON object, else the text itself, shortened.
 */
export function inputSummary(input: string, max = 120): string {
  let text = input.trim();
  try {
    const v: unknown = JSON.parse(text);
    if (v && typeof v === "object" && !Array.isArray(v)) {
      const o = v as Record<string, unknown>;
      const key = summaryKeys.find((k) => typeof o[k] === "string" && o[k] !== "");
      if (key) text = String(o[key]);
    }
  } catch {
    // not JSON: the text itself
  }
  text = text.replace(/\s+/g, " ");
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

/** Pretty-prints JSON input for the details Sheet; other text unchanged. */
export function prettyInput(input: string): string {
  try {
    return JSON.stringify(JSON.parse(input), null, 2);
  } catch {
    return input;
  }
}

/** Time left as m:ss (h:mm:ss from an hour); "0:00" when past. */
export function timeLeft(until: string, now: Date): string {
  const s = Math.max(0, Math.floor((new Date(until).getTime() - now.getTime()) / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}
