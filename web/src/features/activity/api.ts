// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

export type ActivityItem = Schemas["ActivityItem"];
export type ActivityKind = Schemas["ActivityKind"];

export const activityKeys = {
  all: ["activity"] as const,
  list: (kind: ActivityKind | null, unread: boolean) => ["activity", "list", kind, unread] as const,
  unread: ["activity", "unread"] as const,
};

/** The viewer's feed, newest first, paged (round 55, part H). */
export function useActivity(kind: ActivityKind | null, unread: boolean, enabled = true) {
  return useInfiniteQuery({
    queryKey: activityKeys.list(kind, unread),
    enabled,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/me/activity", {
          params: { query: { ...(kind ? { kind } : {}), ...(unread ? { unread: true } : {}), ...(pageParam ? { cursor: pageParam } : {}) } },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor,
  });
}

/** The badge: the viewer's unread items (people only; undefined otherwise). */
export function useActivityUnread(enabled: boolean): number | undefined {
  const q = useQuery({
    queryKey: activityKeys.unread,
    enabled,
    queryFn: async () => unwrap(await api.GET("/me/activity", { params: { query: { limit: 1 } } })).unread_count,
  });
  return enabled ? q.data : undefined;
}

/** Marks items read: the given ones, a conversation's, or (neither) all. */
export function useMarkActivityRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["MarkActivityReadRequest"]) => unwrap(await api.POST("/me/activity/read", { body })).marked,
    onSettled: () => void qc.invalidateQueries({ queryKey: activityKeys.all }),
  });
}
