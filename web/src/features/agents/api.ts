import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

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

export const agentKeys = {
  all: ["agents"] as const,
  actors: (archived: boolean) => ["agents", "actors", archived] as const,
  instructions: (id: string) => ["agents", "instructions", id] as const,
  rules: ["agents", "shared-rules"] as const,
};

/** Every actor, archived agents too when asked (the directory's archive filter). */
export function useAgentActors(archived: boolean) {
  return useQuery({
    queryKey: agentKeys.actors(archived),
    queryFn: async () => unwrap(await api.GET("/actors", { params: { query: archived ? { archived: true } : {} } })).actors,
  });
}

export function useAgentInstructions(agentId: string | null) {
  return useQuery({
    queryKey: agentKeys.instructions(agentId ?? ""),
    enabled: Boolean(agentId),
    staleTime: Infinity, // set once, at creation
    queryFn: async () => unwrap(await api.GET("/agents/{agentId}/instructions", { params: { path: { agentId: agentId! } } })),
  });
}

/** The shared rules text (for the creation dialog's effective preview). */
export function useSharedRules(enabled: boolean) {
  return useQuery({ queryKey: agentKeys.rules, enabled, queryFn: async () => unwrap(await api.GET("/agent-rules")) });
}

function useAgentMutation<V, R>(fn: (v: V) => Promise<R>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: agentKeys.all });
      void qc.invalidateQueries({ queryKey: ["hosts"] });
      void qc.invalidateQueries({ queryKey: ["identity"] });
    },
  });
}

export const useCreateAgent = () => useAgentMutation(async (body: Schemas["CreateAgentRequest"]) => unwrap(await api.POST("/agents", { body })));
export const useArchiveAgent = () =>
  useAgentMutation(async (agentId: string) => {
    unwrap(await api.DELETE("/agents/{agentId}", { params: { path: { agentId } } }));
  });
export const useSetLimits = () =>
  useAgentMutation(async ({ agentId, ...body }: Schemas["AgentLimitsRequest"] & { agentId: string }) =>
    unwrap(await api.PATCH("/agents/{agentId}/limits", { params: { path: { agentId } }, body })),
  );
export const useUpdateAgentProfile = () =>
  useAgentMutation(async ({ id, ...body }: Schemas["UpdateActorProfileRequest"] & { id: string }) =>
    unwrap(await api.PATCH("/actors/{actorId}/profile", { params: { path: { actorId: id } }, body })),
  );
export const useSetAgentState = () =>
  useAgentMutation(async ({ id, state }: { id: string; state: "active" | "paused" | "stopped" }) => {
    unwrap(await api.PUT("/actors/{actorId}/state", { params: { path: { actorId: id } }, body: { state } }));
  });

/** The channels an agent can join at creation (not DMs). */
export function useJoinableChannels(enabled: boolean) {
  return useQuery({
    queryKey: ["agents", "channels"],
    enabled,
    queryFn: async () => unwrap(await api.GET("/channels")).channels.filter((c) => c.kind !== "dm"),
  });
}
