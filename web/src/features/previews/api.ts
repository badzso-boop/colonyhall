// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, onResync, onUntypedEvent, unwrap } from "@/shared/api";

export const previewKeys = { all: ["previews"] as const };

/** Every preview the viewer may see (round 13; proxy_url since round 57). */
export function usePreviews(enabled = true) {
  return useQuery({ queryKey: previewKeys.all, enabled, queryFn: async () => unwrap(await api.GET("/previews")).previews });
}

export function useStopPreview() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (previewId: string) => unwrap(await api.POST("/previews/{previewId}/stop", { params: { path: { previewId } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: previewKeys.all }),
  });
}

const previewEvents = ["preview.requested", "preview.running", "preview.stopped", "preview.failed"];

/** Live updates: every preview change re-reads the list. */
export function registerPreviewEvents(qc: QueryClient): () => void {
  const reload = () => void qc.invalidateQueries({ queryKey: previewKeys.all });
  const offs = [...previewEvents.map((t) => onUntypedEvent(t, reload)), onResync(reload)];
  return () => offs.forEach((off) => off());
}
