import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

export const agentKeys = {
  all: ["agents"] as const,
  overview: (id: string) => ["agents", "overview", id] as const,
  overviews: ["agents", "overview"] as const,
  runs: (id: string) => ["agents", "runs", id] as const,
  usage: ["agents", "usage"] as const,
  hosts: ["agents", "hosts"] as const,
  grants: ["agents", "grants"] as const,
  instructions: (id: string) => ["agents", "instructions", id] as const,
  error: (id: string) => ["agents", "error", id] as const,
};

/** The overview; live events patch it, otherwise it is refetched at most every 15 s. */
export function useAgentOverview(agentId: string) {
  return useQuery({
    queryKey: agentKeys.overview(agentId),
    refetchInterval: 15_000,
    staleTime: 15_000,
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

/** The agent's latest runs with their place and duration (humans only). */
export function useAgentRuns(agentId: string, enabled: boolean) {
  return useQuery({
    queryKey: agentKeys.runs(agentId),
    enabled,
    retry: false,
    staleTime: 15_000,
    queryFn: async () => unwrap(await api.GET("/runs", { params: { query: { agent_id: agentId, limit: 5 } } })).runs,
  });
}

/** Tokens and runs per agent and day for the last seven days (humans only). */
export function useUsageWeek(enabled: boolean) {
  return useQuery({
    queryKey: agentKeys.usage,
    enabled,
    retry: false,
    staleTime: 15_000,
    queryFn: async () => unwrap(await api.GET("/usage", { params: { query: { days: 7 } } })).usage,
  });
}

/** The machines (human admins): the host chip's colour and when its guard last reported. */
export function useHosts(enabled: boolean) {
  return useQuery({
    queryKey: agentKeys.hosts,
    enabled,
    retry: false,
    staleTime: 60_000,
    queryFn: async () => unwrap(await api.GET("/hosts")).hosts,
  });
}

/** The active delegation grants of the workspace. */
export function useDelegationGrants(enabled: boolean) {
  return useQuery({
    queryKey: agentKeys.grants,
    enabled,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/delegation-grants", { params: { query: { active: true } } })).grants,
  });
}

/** The agent's own instructions, set at creation (humans only). */
export function useAgentInstructions(agentId: string, enabled: boolean) {
  return useQuery({
    queryKey: agentKeys.instructions(agentId),
    enabled,
    retry: false,
    staleTime: Infinity,
    queryFn: async () => unwrap(await api.GET("/agents/{agentId}/instructions", { params: { path: { agentId } } })),
  });
}

/** The details of a failed run (humans only); fetched when asked for. */
export function useRunError(errorId: string | null) {
  return useQuery({
    queryKey: agentKeys.error(errorId ?? ""),
    enabled: Boolean(errorId),
    retry: false,
    staleTime: Infinity,
    queryFn: async () => unwrap(await api.GET("/errors/{errorId}", { params: { path: { errorId: errorId! } } })),
  });
}

/** Pause, resume or stop an agent (admins). */
export function useSetAgentState(agentId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (state: Schemas["SetStateRequest"]["state"]) => {
      unwrap(await api.PUT("/actors/{actorId}/state", { params: { path: { actorId: agentId } }, body: { state } }));
      return state;
    },
    onSuccess: (state) => {
      qc.setQueryData<Schemas["AgentOverview"]>(agentKeys.overview(agentId), (o) => (o ? { ...o, state } : o));
      void qc.invalidateQueries({ queryKey: agentKeys.overview(agentId) });
    },
  });
}

export function useRevokeGrant() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/delegation-grants/{id}/revoke", { params: { path: { id } } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: agentKeys.grants }),
  });
}

export function useReleaseLease(agentId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/leases/{id}/release", { params: { path: { id } } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: agentKeys.overview(agentId) }),
  });
}

/** The DM channel with this actor, created on first use. */
export function useOpenDm() {
  return useMutation({
    mutationFn: async (actorId: string) => unwrap(await api.POST("/dms", { body: { actor_ids: [actorId] } })),
  });
}
