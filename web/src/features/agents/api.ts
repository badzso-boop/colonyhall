import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/shared/api";

export function useAgentOverview(agentId: string) {
  return useQuery({
    queryKey: ["agents", "overview", agentId],
    refetchInterval: 15_000,
    queryFn: async () => unwrap(await api.GET("/agents/{id}/overview", { params: { path: { id: agentId } } })),
  });
}

/**
 * A channel the viewer shares with the agent: the agent relay operations
 * (terminals, files) are authorised through one.
 */
export function useAgentChannel(agentId: string): { channelId: string | null; pending: boolean } {
  const overview = useAgentOverview(agentId);
  return { channelId: overview.data?.common_channels?.[0]?.id ?? null, pending: overview.isPending };
}
