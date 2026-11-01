import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, onResync, onUntypedEvent, unwrap, type Schemas } from "@/shared/api";
import type { Task } from "./tasks";

export const plannerKeys = {
  tasks: ["planner", "tasks"] as const,
  calendars: ["planner", "calendars"] as const,
  events: (from: string, to: string) => ["planner", "events", from, to] as const,
  access: ["planner", "access"] as const,
};

export function useTasks() {
  return useQuery({ queryKey: plannerKeys.tasks, queryFn: async () => unwrap(await api.GET("/planner/tasks")).tasks });
}

export function useCreateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["CreatePlannerTaskRequest"]) => unwrap(await api.POST("/planner/tasks", { body })),
    onSettled: () => void qc.invalidateQueries({ queryKey: plannerKeys.tasks }),
  });
}

/** Title, description, status or a move; optimistic for status and moves. */
export function useUpdateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, ...body }: Schemas["UpdatePlannerTaskRequest"] & { id: string }) =>
      unwrap(await api.PATCH("/planner/tasks/{id}", { params: { path: { id } }, body })),
    onMutate: async ({ id, status }) => {
      if (!status) return;
      await qc.cancelQueries({ queryKey: plannerKeys.tasks });
      qc.setQueryData<Task[]>(plannerKeys.tasks, (list) => list?.map((t) => (t.id === id ? { ...t, status } : t)));
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: plannerKeys.tasks }),
  });
}

export function useDeleteTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(await api.DELETE("/planner/tasks/{id}", { params: { path: { id } } }));
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: plannerKeys.tasks }),
  });
}

export function useCalendars() {
  return useQuery({ queryKey: plannerKeys.calendars, queryFn: async () => unwrap(await api.GET("/planner/calendars")).calendars });
}

/** Which agents may read the viewer's tasks and calendars (round 34, part 8). */
export function usePlannerAccess(enabled: boolean) {
  return useQuery({ queryKey: plannerKeys.access, enabled, queryFn: async () => unwrap(await api.GET("/planner/access")) });
}

export function useSavePlannerAccess() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["PlannerAccess"]) => unwrap(await api.PUT("/planner/access", { body })),
    onSuccess: (a) => qc.setQueryData(plannerKeys.access, a),
  });
}

/** The occurrences in [from, to); the calendars are cached on the server, so a minute is fresh enough. */
export function useEvents(from: Date, to: Date, enabled = true) {
  const f = from.toISOString();
  const t = to.toISOString();
  return useQuery({
    queryKey: plannerKeys.events(f, t),
    queryFn: async () => unwrap(await api.GET("/planner/events", { params: { query: { from: f, to: t } } })).events,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
    enabled,
  });
}

export function useAddCalendar() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["AddPlannerCalendarRequest"]) => unwrap(await api.POST("/planner/calendars", { body })),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["planner"] }),
  });
}

export function useRefreshCalendar() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/planner/calendars/{id}/refresh", { params: { path: { id } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["planner"] }),
  });
}

export function useDeleteCalendar() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(await api.DELETE("/planner/calendars/{id}", { params: { path: { id } } }));
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ["planner"] }),
  });
}

/** Tasks and calendars change in another tab too: refetch on their events. */
export function registerPlannerEvents(qc: QueryClient): () => void {
  const tasks = () => void qc.invalidateQueries({ queryKey: plannerKeys.tasks });
  const cal = () => void qc.invalidateQueries({ queryKey: ["planner"] });
  const offs = [
    ...["planner.task_created", "planner.task_updated", "planner.task_status_changed", "planner.task_moved", "planner.task_deleted"].map((t) => onUntypedEvent(t, tasks)),
    onUntypedEvent("planner.calendars_changed", cal),
    onResync(cal),
  ];
  return () => offs.forEach((off) => off());
}
