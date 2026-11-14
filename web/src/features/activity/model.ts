import type { ActivityItem } from "./api";

export const activityKinds = ["mention", "thread_reply", "reaction", "approval", "question", "important", "reminder"] as const;

/**
 * Where an item leads (round 55, part H): the message (in its thread for a
 * reply), else the conversation; null when the person can no longer read it.
 */
export function activityTarget(it: ActivityItem): string | null {
  if (!it.available || !it.channel_id) return null;
  const params = new URLSearchParams();
  if (it.thread_root_id) params.set("thread", it.thread_root_id);
  if (it.message_id) params.set("msg", it.message_id);
  const q = params.toString();
  return `/c/${it.channel_id}${q ? `?${q}` : ""}`;
}
