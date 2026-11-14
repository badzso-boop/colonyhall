// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { keepPreviousData, useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, onResync, onUntypedEvent, unwrap, type Schemas } from "@/shared/api";

export type Schedule = Schemas["Schedule"];

export const scheduleKeys = {
  list: ["schedules"] as const,
  preview: (body: Schemas["SchedulePreviewRequest"]) => ["schedules", "preview", body] as const,
};

export function useSchedules() {
  return useQuery({ queryKey: scheduleKeys.list, queryFn: async () => unwrap(await api.GET("/schedules")).schedules });
}

/** The server's next runs for the builder's current choice; null body = nothing to ask yet. */
export function useSchedulePreview(body: Schemas["SchedulePreviewRequest"] | null) {
  return useQuery({
    queryKey: scheduleKeys.preview(body ?? {}),
    enabled: body !== null,
    queryFn: async () => unwrap(await api.POST("/schedules/preview", { body: body! })),
    placeholderData: keepPreviousData,
    retry: false,
    staleTime: 30_000,
  });
}

export function useCreateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["CreateScheduleRequest"]) => unwrap(await api.POST("/schedules", { body })),
    onSettled: () => void qc.invalidateQueries({ queryKey: scheduleKeys.list, exact: true }),
  });
}

export function useCancelSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(await api.DELETE("/schedules/{scheduleId}", { params: { path: { scheduleId: id } } }));
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: scheduleKeys.list, exact: true }),
  });
}

export function registerScheduleEvents(qc: QueryClient): () => void {
  const refetch = () => void qc.invalidateQueries({ queryKey: scheduleKeys.list, exact: true });
  const offs = [...["schedule.created", "schedule.canceled", "schedule.fired"].map((t) => onUntypedEvent(t, refetch)), onResync(refetch)];
  return () => offs.forEach((off) => off());
}
