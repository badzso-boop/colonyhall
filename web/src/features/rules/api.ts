import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, onResync, onUntypedEvent, unwrap } from "@/shared/api";

export const rulesKeys = { all: ["agent-rules"] as const, current: ["agent-rules", "current"] as const, version: (v: number) => ["agent-rules", "v", v] as const };

export function useRules() {
  return useQuery({ queryKey: rulesKeys.current, queryFn: async () => unwrap(await api.GET("/agent-rules")) });
}

/** One version's text; versions never change, so they are cached for good. */
export function useRulesVersion(version: number | null) {
  return useQuery({
    queryKey: rulesKeys.version(version ?? 0),
    enabled: version !== null && version > 0,
    staleTime: Infinity,
    queryFn: async () => unwrap(await api.GET("/agent-rules/{version}", { params: { path: { version: version! } } })),
  });
}

export function useSaveRules() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (text: string) => unwrap(await api.PUT("/agent-rules", { body: { text } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: rulesKeys.current }),
  });
}

export function registerRulesEvents(qc: QueryClient): () => void {
  const refetch = () => void qc.invalidateQueries({ queryKey: rulesKeys.current });
  const offs = [onUntypedEvent("agent_rules.updated", refetch), onResync(refetch)];
  return () => offs.forEach((off) => off());
}
