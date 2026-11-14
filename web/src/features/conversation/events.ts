// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { QueryClient } from "@tanstack/react-query";
import { onEvent, onResync, type AppEvent } from "@/shared/api";
import { conversationKeys, draftsKey, pinsKey, type Message, type MessagePages } from "./api";
import { applyReply } from "./model";

function fromEvent(e: AppEvent<"message.created">): Message {
  const d = e.data;
  return {
    id: d.message_id,
    channel_id: d.channel_id,
    thread_root_id: d.thread_root_id,
    author_id: d.author_id,
    client_msg_id: d.client_msg_id,
    kind: d.kind,
    body: d.body,
    mentions: d.mentions,
    reply_count: 0,
    created_at: d.created_at,
    reactions: [],
    attachments: d.attachments ?? [],
    ...(d.important ? { important: true, pinned: true } : {}),
    ...(d.also_in_channel ? { also_in_channel: true } : {}),
  };
}

/** Keeps conversations live: new messages, replies, retractions, reactions, channels. */
export function registerConversationEvents(qc: QueryClient): () => void {
  const channels = () => void qc.invalidateQueries({ queryKey: conversationKeys.channels });
  const reloadChannel = (channelId: string) => void qc.invalidateQueries({ queryKey: conversationKeys.messages(channelId) });
  const threads = () => void qc.invalidateQueries({ queryKey: ["conversation", "myThreads"] });
  // A rename, description, archive or membership change: the sidebar, the header and open dialogs refetch.
  const channelChanged = (channelId: string) => {
    channels();
    void qc.invalidateQueries({ queryKey: conversationKeys.members(channelId) });
    void qc.invalidateQueries({ queryKey: conversationKeys.archived });
  };

  const offs = [
    // Another tab or device saved, sent or removed a draft (the owner alone gets this).
    onEvent("draft.changed", () => void qc.invalidateQueries({ queryKey: draftsKey })),
    onEvent("message.created", (e) => {
      const m = fromEvent(e);
      // The newest page is pages[0]: append there (once, even if the event is replayed).
      qc.setQueryData<MessagePages>(conversationKeys.messages(m.channel_id), (data) => {
        if (!data || data.pages.some((p) => p.some((x) => x.id === m.id))) return data;
        const pages = data.pages.map((p) => (m.thread_root_id ? applyReply(p, m) : p));
        pages[0] = [...(pages[0] ?? []), m];
        return { ...data, pages };
      });
      if (m.thread_root_id) {
        qc.setQueryData(conversationKeys.thread(m.thread_root_id), (t: { root: Message; replies: Message[] } | undefined) =>
          t && !t.replies.some((r) => r.id === m.id) ? { root: applyReply([t.root], m)[0]!, replies: [...t.replies, m] } : t,
        );
      }
      channels();
      if (m.thread_root_id) threads();
    }),
    onEvent("message.retracted", (e) => {
      reloadChannel(e.data.channel_id);
      if (e.data.thread_root_id) void qc.invalidateQueries({ queryKey: conversationKeys.thread(e.data.thread_root_id) });
    }),
    // Round 54: a deleted file shows "File deleted" in every message that carried it.
    onEvent("file.deleted", (e) => {
      reloadChannel(e.data.channel_id);
      void qc.invalidateQueries({ queryKey: ["conversation", "thread"] });
    }),
    ...(["message.pinned", "message.unpinned"] as const).map((type) =>
      onEvent(type, (e) => {
        void qc.invalidateQueries({ queryKey: pinsKey(e.data.channel_id) });
        reloadChannel(e.data.channel_id);
        if (e.data.thread_root_id) void qc.invalidateQueries({ queryKey: conversationKeys.thread(e.data.thread_root_id) });
      }),
    ),
    onEvent("reaction.added", (e) => reloadChannel(e.data.channel_id)),
    onEvent("reaction.removed", (e) => reloadChannel(e.data.channel_id)),
    onEvent("channel.created", channels),
    onEvent("channel.member_added", (e) => channelChanged(e.data.channel_id)),
    onEvent("channel.member_updated", (e) => channelChanged(e.data.channel_id)),
    onEvent("channel.member_removed", (e) => {
      channelChanged(e.data.channel_id);
      threads();
    }),
    onEvent("channel.updated", (e) => channelChanged(e.data.channel_id)),
    onEvent("channel.archived", (e) => channelChanged(e.data.channel_id)),
    onEvent("channel.unarchived", (e) => channelChanged(e.data.channel_id)),
    onEvent("channel.locked", (e) => channelChanged(e.data.channel_id)),
    onEvent("channel.unlocked", (e) => channelChanged(e.data.channel_id)),
    // Only the viewer's own mutes arrive (their other tabs).
    onEvent("channel.mute_changed", () => {
      channels();
      threads();
    }),
    onResync(() => void qc.invalidateQueries({ queryKey: ["conversation"] })),
  ];
  return () => offs.forEach((off) => off());
}
