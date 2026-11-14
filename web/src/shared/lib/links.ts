// Links to a message (round 55): the conversation opens at the message;
// a thread reply also opens its thread. A person without access gets the
// conversation's normal "not found".

export type MessageRef = { channel_id: string; id: string; thread_root_id?: string | null };

/** The in-app path of a message (under the router's /app base). */
export function messageHref(m: MessageRef): string {
  const params = new URLSearchParams();
  if (m.thread_root_id) params.set("thread", m.thread_root_id);
  params.set("msg", m.id);
  return `/c/${m.channel_id}?${params.toString()}`;
}

/** The full, shareable URL of a message ("Copy link"). */
export function messageURL(m: MessageRef, origin: string): string {
  return `${origin}/app${messageHref(m)}`;
}
