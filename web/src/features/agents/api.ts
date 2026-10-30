import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/shared/api";

export function useAgentOverview(agentId: string) {
  return useQuery({
    queryKey: ["agents", "overview", agentId],
    refetchInterval: 15_000,
    queryFn: async () => unwrap(await api.GET("/agents/{id}/overview", { params: { path: { id: agentId } } })),
  });
}
