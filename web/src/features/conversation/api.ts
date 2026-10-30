import { useInfiniteQuery, useMutation, useQuery, useQueryClient, type InfiniteData } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

export type Message = Schemas["Message"];
export const PAGE_SIZE = 50;

export const conversationKeys = {
  channels: ["conversation", "channels"] as const,
  sections: ["conversation", "sections"] as const,
  messages: (channelId: string) => ["conversation", "messages", channelId] as const,
  thread: (rootId: string) => ["conversation", "thread", rootId] as const,
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
    queryKey: ["conversation", "members", channelId],
    queryFn: async () => unwrap(await api.GET("/channels/{channelId}/members", { params: { path: { channelId } } })).members,
  });
}

export function useRetract() {
  return useMutation({
    mutationFn: async (messageId: string) => {
      unwrap(await api.POST("/messages/{messageId}/retract", { params: { path: { messageId } } }));
    },
  });
}
