import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, onResync, onUntypedEvent, unwrap } from "@/shared/api";

const History = 7 * 86_400_000;

export const leaseKeys = { all: ["leases"] as const, list: (history: boolean) => ["leases", history ? "all" : "active"] as const };

/** The active leases, or every lease of the last 7 days. */
export function useLeases(history: boolean) {
  return useQuery({
    queryKey: leaseKeys.list(history),
    queryFn: async () =>
      unwrap(await api.GET("/leases", { params: { query: history ? { state: "all", since: new Date(Date.now() - History + 60_000).toISOString() } : {} } })).leases,
  });
}

export function useReleaseLease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/leases/{id}/release", { params: { path: { id } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: leaseKeys.all }),
  });
}

export function registerLeaseEvents(qc: QueryClient): () => void {
  const refetch = () => void qc.invalidateQueries({ queryKey: leaseKeys.all });
  const offs = [...["lease.acquired", "lease.extended", "lease.released"].map((t) => onUntypedEvent(t, refetch)), onResync(refetch)];
  return () => offs.forEach((off) => off());
}
