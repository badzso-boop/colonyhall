import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, api, unwrap } from "@/shared/api";
import { useLiveCompactions } from "./store";

export const compactionKeys = {
  all: ["compaction"] as const,
  status: (channelId: string, threadRootId: string | null) => ["compaction", "status", channelId, threadRootId ?? ""] as const,
  settings: ["compaction", "settings"] as const,
  summary: (fileId: string) => ["compaction", "summary", fileId] as const,
};

/**
 * The compaction counter of a channel's main stream or a thread: tokens
 * since the last summary, the threshold in force (the channel's own or the
 * workspace default) and whether one is running.
 */
export function useCompactionStatus(channelId: string, threadRootId: string | null = null, enabled = true) {
  return useQuery({
    queryKey: compactionKeys.status(channelId, threadRootId),
    enabled: enabled && Boolean(channelId),
    retry: false,
    queryFn: async () =>
      unwrap(
        await api.GET("/channels/{channelId}/compaction", {
          params: { path: { channelId }, query: threadRootId ? { thread_root_id: threadRootId } : {} },
        }),
      ),
  });
}

/** The workspace default threshold and scribe (shown next to a channel's own threshold). */
export function useCompactionSettings(enabled = true) {
  return useQuery({
    queryKey: compactionKeys.settings,
    enabled,
    retry: false,
    staleTime: 60_000,
    queryFn: async () => unwrap(await api.GET("/settings/compaction")),
  });
}

/**
 * "Tömörítés most" (humans who can write the channel): starts a compaction
 * of the main stream, or of a thread when given. The pending card shows
 * until compaction.done or compaction.failed.
 */
export function useCompactNow(channelId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (threadRootId: string | null = null) =>
      unwrap(
        await api.POST("/channels/{channelId}/compactions", {
          params: { path: { channelId } },
          body: threadRootId ? { thread_root_id: threadRootId } : {},
        }),
      ),
    onSuccess: (c) => {
      useLiveCompactions.getState().add(c);
      void qc.invalidateQueries({ queryKey: compactionKeys.status(channelId, c.thread_root_id ?? null) });
    },
  });
}

/** The channel's own threshold; null returns to the workspace default (admins or channel managers). */
export function useSetCompactThreshold(channelId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (tokens: number | null) =>
      unwrap(await api.PATCH("/channels/{channelId}/compaction", { params: { path: { channelId } }, body: { compact_threshold_tokens: tokens } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: [...compactionKeys.all, "status", channelId] });
      // The channel list carries compact_threshold_tokens.
      void qc.invalidateQueries({ queryKey: ["conversation", "channels"] });
    },
  });
}

/** The summary's markdown (a stored file). */
export function useSummary(fileId: string | null) {
  return useQuery({
    queryKey: compactionKeys.summary(fileId ?? ""),
    enabled: fileId !== null,
    staleTime: Infinity,
    queryFn: async () => {
      const res = await fetch(`/api/v1/files/${fileId}/content`, { credentials: "same-origin" });
      if (!res.ok) throw new ApiError(res.status, undefined);
      return res.text();
    },
  });
}

export function useEditSummary(compactionId: string) {
  return useMutation({
    mutationFn: async (markdown: string) =>
      unwrap(await api.PUT("/compactions/{id}/summary", { params: { path: { id: compactionId } }, body: { markdown } })),
    onSuccess: (c) => useLiveCompactions.getState().add(c),
  });
}

export function useRegenerate(compactionId: string) {
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/compactions/{id}/regenerate", { params: { path: { id: compactionId } } })),
    onSuccess: (c) => useLiveCompactions.getState().add(c),
  });
}
