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
