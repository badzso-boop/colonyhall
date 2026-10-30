import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, onEvent, onResync, unwrap } from "@/shared/api";

export const adminKeys = { status: ["admin", "status"] as const };

/** The emergency stop state (admins). */
export function useEmergency(enabled: boolean) {
  return useQuery({
    queryKey: adminKeys.status,
    enabled,
    queryFn: async () => unwrap(await api.GET("/admin/status")).emergency_stop,
  });
}

export function useEmergencyStop() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/admin/emergency-stop")).emergency_stop,
    onSuccess: (s) => qc.setQueryData(adminKeys.status, s),
  });
}

export function registerAdminEvents(qc: QueryClient): () => void {
  const refetch = () => void qc.invalidateQueries({ queryKey: adminKeys.status });
  const offs = [onEvent("workspace.emergency_activated", refetch), onEvent("workspace.emergency_cleared", refetch), onResync(refetch)];
  return () => offs.forEach((off) => off());
}
