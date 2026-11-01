import { useInfiniteQuery, useMutation, useQuery, useQueryClient, type InfiniteData } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

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

export function useAddMembers(channelId: string) {
  const refresh = useChannelRefresh();
  return useMutation({
    mutationFn: async (actorIds: string[]) => {
      unwrap(await api.POST("/channels/{channelId}/members", { params: { path: { channelId } }, body: { actor_ids: actorIds, can_write: true } }));
    },
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
