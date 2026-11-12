import { useInfiniteQuery, useMutation, useQuery, useQueryClient, type InfiniteData } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";
import { itemOrder, moveConversation, moveSection, sectionOrder, upsertSection } from "./sections";

export type Message = Schemas["Message"];
export const PAGE_SIZE = 50;

export const conversationKeys = {
  channels: ["conversation", "channels"] as const,
  sections: ["conversation", "sections"] as const,
  messages: (channelId: string) => ["conversation", "messages", channelId] as const,
  thread: (rootId: string) => ["conversation", "thread", rootId] as const,
  members: (channelId: string) => ["conversation", "members", channelId] as const,
  archived: ["conversation", "channels", "archived"] as const,
  myThreads: (unread: boolean) => ["conversation", "myThreads", unread] as const,
};

export function useChannels() {
  return useQuery({
    queryKey: conversationKeys.channels,
    queryFn: async () => unwrap(await api.GET("/channels")).channels,
  });
}

export function useSections() {
  return useQuery({
    queryKey: conversationKeys.sections,
    queryFn: async () => unwrap(await api.GET("/sidebar/sections")).sections,
  });
}

type Section = Schemas["SidebarSection"];

/**
 * A change to the viewer's sections, applied to the cache at once and rolled
 * back when the server refuses it (round 34, part 2). `send` gets the
 * sections as they are after the change. The caller shows the toast; the
 * list is refetched afterwards either way.
 */
function useSectionChange<V>(apply: (sections: Section[], v: V) => Section[], send: (v: V, after: Section[]) => Promise<void>) {
  const qc = useQueryClient();
  return useMutation({
    onMutate: async (v: V) => {
      await qc.cancelQueries({ queryKey: conversationKeys.sections });
      const before = qc.getQueryData<Section[]>(conversationKeys.sections);
      if (before) qc.setQueryData<Section[]>(conversationKeys.sections, apply(before, v));
      return { before };
    },
    // onMutate has run by now, so the cache holds the changed state.
    mutationFn: async (v: V) => send(v, qc.getQueryData<Section[]>(conversationKeys.sections) ?? []),
    onError: (_e, _v, ctx) => {
      if (ctx?.before) qc.setQueryData(conversationKeys.sections, ctx.before);
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: conversationKeys.sections }),
  });
}

/** Reorders the sections: the server gets the full new order. */
export function useReorderSections() {
  return useSectionChange<{ sectionId: string; toIndex: number }>(
    (s, v) => moveSection(s, v.sectionId, v.toIndex),
    async (_v, after) => {
      unwrap(await api.PUT("/sidebar/sections/reorder", { body: { section_ids: sectionOrder(after) } }));
    },
  );
}

/**
 * Moves a conversation into a section at an index (the end when omitted), or
 * back to its default group (target null). `from` is where it is now. The
 * server takes it out of any other section itself.
 */
export function useMoveConversation() {
  return useSectionChange<{ channelId: string; from: string | null; target: string | null; index?: number }>(
    (s, v) => moveConversation(s, v.channelId, v.target, v.index),
    async ({ channelId, from, target }, after) => {
      if (target === null) {
        if (from) unwrap(await api.DELETE("/sidebar/sections/{sectionId}/channels/{channelId}", { params: { path: { sectionId: from, channelId } } }));
        return;
      }
      const order = itemOrder(after, target);
      if (from !== target) {
        const position = order.indexOf(channelId);
        unwrap(await api.PUT("/sidebar/sections/{sectionId}/channels/{channelId}", { params: { path: { sectionId: target, channelId } }, body: { position } }));
      }
      unwrap(await api.PUT("/sidebar/sections/{sectionId}/reorder", { params: { path: { sectionId: target } }, body: { channel_ids: order } }));
    },
  );
}

/** Opens (or creates) the DM with one actor; the caller navigates to it. */
export function useOpenDm() {
  return useMutation({
    mutationFn: async (actorId: string) => unwrap(await api.POST("/dms", { body: { actor_ids: [actorId] } })),
  });
}

export function useCreateSection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) => unwrap(await api.POST("/sidebar/sections", { body: { name: name.trim() } })),
    onSuccess: (s) => qc.setQueryData<Section[]>(conversationKeys.sections, (list) => upsertSection(list ?? [], s)),
    onSettled: () => void qc.invalidateQueries({ queryKey: conversationKeys.sections }),
  });
}

export function useRenameSection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, name }: { id: string; name: string }) =>
      unwrap(await api.PATCH("/sidebar/sections/{sectionId}", { params: { path: { sectionId: id } }, body: { name: name.trim() } })),
    onSuccess: (s) => qc.setQueryData<Section[]>(conversationKeys.sections, (list) => upsertSection(list ?? [], s)),
    onSettled: () => void qc.invalidateQueries({ queryKey: conversationKeys.sections }),
  });
}

/** Deletes a section; its conversations go back to their default groups. */
export function useDeleteSection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(await api.DELETE("/sidebar/sections/{sectionId}", { params: { path: { sectionId: id } } }));
    },
    onSuccess: (_d, id) => qc.setQueryData<Section[]>(conversationKeys.sections, (list) => list?.filter((s) => s.id !== id)),
    onSettled: () => void qc.invalidateQueries({ queryKey: conversationKeys.sections }),
  });
}

/** A conversation's messages, oldest first; older pages load on demand. */
export function useMessages(channelId: string) {
  return useInfiniteQuery({
    queryKey: conversationKeys.messages(channelId),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/channels/{channelId}/messages", {
          params: { path: { channelId }, query: { limit: PAGE_SIZE, ...(pageParam ? { before: pageParam } : {}) } },
        }),
      ).messages,
    // Pages are fetched newest first; the next page is older than the oldest message seen.
    getNextPageParam: (last) => (last.length < PAGE_SIZE ? undefined : last[0]?.id),
    select: (data) => data.pages.slice().reverse().flat(),
  });
}

export function useThread(rootId: string | null) {
  return useQuery({
    queryKey: conversationKeys.thread(rootId ?? ""),
    enabled: rootId !== null,
    queryFn: async () => unwrap(await api.GET("/messages/{messageId}/thread", { params: { path: { messageId: rootId! } } })),
  });
}

/** The composer's /schedule, /daily and /cron (the builder is the schedules feature's). */
export function useCreateSchedule() {
  return useMutation({
    mutationFn: async (body: Schemas["CreateScheduleRequest"]) => unwrap(await api.POST("/schedules", { body })),
  });
}

export function useSendMessage(channelId: string) {
  return useMutation({
    mutationFn: async (body: Schemas["SendMessageRequest"]) =>
      unwrap(await api.POST("/channels/{channelId}/messages", { params: { path: { channelId } }, body })),
    // The message itself arrives over the WebSocket (events.ts), like everyone else's.
  });
}

export function useMarkRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (channelId: string) => {
      await api.POST("/channels/{channelId}/read", { params: { path: { channelId } } });
    },
    onSuccess: (_d, channelId) => {
      qc.setQueryData<Schemas["Channel"][]>(conversationKeys.channels, (list) =>
        list?.map((c) => (c.id === channelId ? { ...c, unread: 0, mentions: 0 } : c)),
      );
    },
  });
}

export type MessagePages = InfiniteData<Message[], string | undefined>;

export type Member = Schemas["ChannelMember"];

export function useMembers(channelId: string) {
  return useQuery({
    queryKey: conversationKeys.members(channelId),
    queryFn: async () => unwrap(await api.GET("/channels/{channelId}/members", { params: { path: { channelId } } })).members,
  });
}

/** Adds or removes the viewer's reaction; reaction.* events refresh the message. */
export function useToggleReaction() {
  return useMutation({
    mutationFn: async ({ messageId, emoji, mine }: { messageId: string; emoji: string; mine: boolean }) => {
      const params = { params: { path: { messageId, emoji } } };
      unwrap(mine ? await api.DELETE("/messages/{messageId}/reactions/{emoji}", params) : await api.PUT("/messages/{messageId}/reactions/{emoji}", params));
    },
  });
}

export function useRetract() {
  return useMutation({
    mutationFn: async (messageId: string) => {
      unwrap(await api.POST("/messages/{messageId}/retract", { params: { path: { messageId } } }));
    },
  });
}

/** Admins: every channel including the archived ones (channel settings, the archive list). */
export function useArchivedChannels(enabled: boolean) {
  return useQuery({
    queryKey: conversationKeys.archived,
    enabled,
    queryFn: async () => unwrap(await api.GET("/channels", { params: { query: { archived: true } } })).channels.filter((c) => c.archived_at),
  });
}

/** After a channel change: the list, its members and the archived list refetch (events do it for others). */
function useChannelRefresh() {
  const qc = useQueryClient();
  return (channelId: string) => {
    void qc.invalidateQueries({ queryKey: conversationKeys.channels });
    void qc.invalidateQueries({ queryKey: conversationKeys.members(channelId) });
  };
}

export function useUpdateChannel(channelId: string) {
  const refresh = useChannelRefresh();
  return useMutation({
    mutationFn: async (body: Schemas["UpdateChannelRequest"]) =>
      unwrap(await api.PATCH("/channels/{channelId}", { params: { path: { channelId } }, body })),
    onSuccess: () => refresh(channelId),
  });
}

export function useSetArchived(channelId: string) {
  const refresh = useChannelRefresh();
  return useMutation({
    mutationFn: async (archive: boolean) =>
      unwrap(
        archive
          ? await api.POST("/channels/{channelId}/archive", { params: { path: { channelId } } })
          : await api.POST("/channels/{channelId}/unarchive", { params: { path: { channelId } } }),
      ),
    onSuccess: () => refresh(channelId),
  });
}

/** The agent lock (admins): agents cannot write or wake in the channel. */
export function useSetLocked(channelId: string) {
  const refresh = useChannelRefresh();
  return useMutation({
    mutationFn: async (lock: boolean) =>
      unwrap(
        lock
          ? await api.POST("/channels/{channelId}/lock", { params: { path: { channelId } } })
          : await api.POST("/channels/{channelId}/unlock", { params: { path: { channelId } } }),
      ),
    onSuccess: () => refresh(channelId),
  });
}

export function useAddMembers(channelId: string) {
  const refresh = useChannelRefresh();
  return useMutation({
    mutationFn: async ({ actorIds, canWrite }: { actorIds: string[]; canWrite: boolean }) => {
      unwrap(await api.POST("/channels/{channelId}/members", { params: { path: { channelId } }, body: { actor_ids: actorIds, can_write: canWrite } }));
    },
    onSuccess: () => refresh(channelId),
  });
}

/** Admins: switch a member between read-only and read-write without re-adding it. */
export function useSetMemberCanWrite(channelId: string) {
  const refresh = useChannelRefresh();
  return useMutation({
    mutationFn: async ({ actorId, canWrite }: { actorId: string; canWrite: boolean }) =>
      unwrap(await api.PATCH("/channels/{channelId}/members/{actorId}", { params: { path: { channelId, actorId } }, body: { can_write: canWrite } })),
    onSuccess: () => refresh(channelId),
  });
}

export function useRemoveMember(channelId: string) {
  const refresh = useChannelRefresh();
  return useMutation({
    mutationFn: async (actorId: string) => {
      unwrap(await api.DELETE("/channels/{channelId}/members/{actorId}", { params: { path: { channelId, actorId } } }));
    },
    onSuccess: () => refresh(channelId),
  });
}

/** Mute for the viewer: until an ISO time, or forever (null); unmute with "off". */
export function useMute(channelId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (until: string | null | "off") => {
      if (until === "off") unwrap(await api.DELETE("/channels/{channelId}/mute", { params: { path: { channelId } } }));
      else unwrap(await api.PUT("/channels/{channelId}/mute", { params: { path: { channelId } }, body: until ? { until } : {} }));
      return until;
    },
    onSuccess: (until) => {
      qc.setQueryData<Schemas["Channel"][]>(conversationKeys.channels, (list) =>
        list?.map((c) => (c.id === channelId ? { ...c, muted: until === "off" ? undefined : true, muted_until: until === "off" || until === null ? undefined : until } : c)),
      );
      void qc.invalidateQueries({ queryKey: ["conversation", "myThreads"] });
    },
  });
}

export type MyThread = Schemas["MyThread"];

/** "Szálaim": the viewer's threads, newest reply first, paged by cursor. */
export function useMyThreads(unreadOnly = false) {
  return useInfiniteQuery({
    queryKey: conversationKeys.myThreads(unreadOnly),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(await api.GET("/me/threads", { params: { query: { limit: 20, ...(unreadOnly ? { unread: true } : {}), ...(pageParam ? { cursor: pageParam } : {}) } } })),
    getNextPageParam: (last) => last.next_cursor,
  });
}

/** The sidebar's "Szálak" count: threads with unread replies (muted channels only with a mention). */
export function useUnreadThreads(enabled: boolean): number {
  const q = useQuery({
    queryKey: [...conversationKeys.myThreads(true), "count"],
    enabled,
    queryFn: async () => unwrap(await api.GET("/me/threads", { params: { query: { limit: 1, unread: true } } })).unread_threads,
  });
  return q.data ?? 0;
}

export function useMarkThreadRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (rootId: string) => {
      unwrap(await api.POST("/messages/{messageId}/thread/read", { params: { path: { messageId: rootId } } }));
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["conversation", "myThreads"] }),
  });
}

export function useMarkAllThreadsRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/me/threads/read")).marked,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["conversation", "myThreads"] }),
  });
}
