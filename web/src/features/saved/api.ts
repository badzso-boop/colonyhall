// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

export type SavedItem = Schemas["SavedItem"];
export type SavedFilter = "all" | "reminders" | "done";

export const savedKeys = {
  all: ["saved"] as const,
  list: (filter: SavedFilter) => ["saved", "list", filter] as const,
  index: ["saved", "index"] as const,
};

/** A page of the viewer's saved items, newest first. */
export function useSaved(filter: SavedFilter, enabled = true) {
  return useInfiniteQuery({
    queryKey: savedKeys.list(filter),
    enabled,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(await api.GET("/me/saved", { params: { query: { filter, ...(pageParam ? { cursor: pageParam } : {}) } } })),
    getNextPageParam: (last) => last.next_cursor,
  });
}

/** Message id → the viewer's item (the message menu's "Mentve"); the first page of everything. */
export function useSavedIndex(enabled: boolean) {
  return useQuery({
    queryKey: savedKeys.index,
    enabled,
    staleTime: 30_000,
    queryFn: async () => {
      const page = unwrap(await api.GET("/me/saved", { params: { query: { filter: "all" } } }));
      return new Map(page.items.filter((i) => i.message_id).map((i) => [i.message_id!, i]));
    },
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => void qc.invalidateQueries({ queryKey: savedKeys.all });
}

/** Saves a message, sets a reminder, or both. */
export function useSave() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: async (body: Schemas["SaveItemRequest"]) => unwrap(await api.POST("/me/saved", { body })),
    onSettled: refresh,
  });
}

export function useUpdateSaved() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: async ({ id, ...body }: Schemas["UpdateSavedRequest"] & { id: string }) =>
      unwrap(await api.PATCH("/me/saved/{savedId}", { params: { path: { savedId: id } }, body })),
    onSettled: refresh,
  });
}

export function useDeleteSaved() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(await api.DELETE("/me/saved/{savedId}", { params: { path: { savedId: id } } }));
    },
    onSettled: refresh,
  });
}
