// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useInfiniteQuery, useMutation, useQuery, useQueryClient, type InfiniteData, type QueryClient, type QueryKey } from "@tanstack/react-query";
import { api, onEvent, onResync, unwrap, type Schemas } from "@/shared/api";

export const adminKeys = {
  status: ["admin", "status"] as const,
  system: ["admin", "system"] as const,
  usage: (days: number, groupBy: string, ownerId = "") => ["admin", "usage", days, groupBy, ownerId] as const,
  runs: (f: RunFilters) => ["admin", "runs", f] as const,
  prices: ["admin", "prices"] as const,
  currency: ["admin", "currency"] as const,
  compaction: ["admin", "compaction"] as const,
  fileFetch: ["admin", "file-fetch"] as const,
  delegation: ["admin", "delegation"] as const,
  grants: ["admin", "grants"] as const,
  mappings: ["admin", "mappings"] as const,
  users: ["admin", "users"] as const,
  tokens: (actorId: string) => ["admin", "tokens", actorId] as const,
  audit: (f: AuditFilters) => ["admin", "audit", f] as const,
  auditActions: ["admin", "audit-actions"] as const,
};

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

/** Releasing it asks for the password again; the server answers lockouts and rate limits. */
export function useEmergencyClear() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (password: string) => unwrap(await api.POST("/admin/emergency-clear", { body: { password } })).emergency_stop,
    onSuccess: (s) => {
      qc.setQueryData(adminKeys.status, s);
      void qc.invalidateQueries({ queryKey: adminKeys.system });
    },
  });
}

/** The whole status page, re-read every 10 s while the tab is visible. */
export function useSystemStatus(enabled = true, refetchInterval: number | false = 10_000) {
  return useQuery({
    queryKey: adminKeys.system,
    enabled,
    refetchInterval,
    queryFn: async () => unwrap(await api.GET("/admin/status")),
  });
}

/** ownerId narrows to one person's agents (round 38); "" = everyone the viewer may see. */
export function useUsage(days: number, groupBy: "agent" | "host" | "model", ownerId = "") {
  return useQuery({
    queryKey: adminKeys.usage(days, groupBy, ownerId),
    placeholderData: (prev) => prev,
    queryFn: async () => unwrap(await api.GET("/usage", { params: { query: { days, group_by: groupBy, ...(ownerId ? { owner_id: ownerId } : {}) } } })),
  });
}

export type RunFilters = { owner_id?: string; agent_id?: string; host_id?: string; channel_id?: string; status?: "ok" | "error" | "halted"; from?: string; to?: string };

export function useRunPages(filters: RunFilters) {
  return useInfiniteQuery<Schemas["RunList"], Error, InfiniteData<Schemas["RunList"], string | undefined>, QueryKey, string | undefined>({
    queryKey: adminKeys.runs(filters),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor || undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.GET("/runs", { params: { query: { ...filters, cursor: pageParam, limit: 50 } } })),
  });
}

export function usePrices() {
  return useQuery({ queryKey: adminKeys.prices, queryFn: async () => unwrap(await api.GET("/settings/prices")).prices });
}

export function useAddPrice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["AddPriceRequest"]) => unwrap(await api.POST("/settings/prices", { body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: adminKeys.prices });
      void qc.invalidateQueries({ queryKey: ["admin", "usage"] });
      void qc.invalidateQueries({ queryKey: ["admin", "runs"] });
    },
  });
}

export function useCurrency() {
  return useQuery({ queryKey: adminKeys.currency, queryFn: async () => unwrap(await api.GET("/settings/currency")) });
}

export function useSetCurrency() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (usd_huf: number) => unwrap(await api.PUT("/settings/currency", { body: { usd_huf } })),
    onSuccess: (c) => qc.setQueryData(adminKeys.currency, c),
  });
}

export function useCompactionSettings() {
  return useQuery({ queryKey: adminKeys.compaction, queryFn: async () => unwrap(await api.GET("/settings/compaction")) });
}

export function useSetCompactionSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["UpdateCompactionSettingsRequest"]) => unwrap(await api.PUT("/settings/compaction", { body })),
    onSuccess: (s) => {
      qc.setQueryData(adminKeys.compaction, s);
      void qc.invalidateQueries({ queryKey: ["compaction"] });
    },
  });
}

export function useFileFetchSettings() {
  return useQuery({ queryKey: adminKeys.fileFetch, queryFn: async () => unwrap(await api.GET("/settings/file-fetch")) });
}

export function useSetFileFetchSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["FileFetchSettings"]) => unwrap(await api.PUT("/settings/file-fetch", { body })),
    onSuccess: (s) => qc.setQueryData(adminKeys.fileFetch, s),
  });
}

export function useDelegationSettings() {
  return useQuery({ queryKey: adminKeys.delegation, queryFn: async () => unwrap(await api.GET("/delegation-settings")) });
}

export function useSetDelegationSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["UpdateDelegationSettingsRequest"]) => unwrap(await api.PUT("/delegation-settings", { body })),
    onSuccess: (s) => qc.setQueryData(adminKeys.delegation, s),
  });
}

export function useActiveGrants() {
  return useQuery({
    queryKey: adminKeys.grants,
    queryFn: async () => unwrap(await api.GET("/delegation-grants", { params: { query: { active: true } } })).grants,
  });
}

export function useRevokeGrant() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/delegation-grants/{id}/revoke", { params: { path: { id } } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: adminKeys.grants }),
  });
}

export function useGitMappings() {
  return useQuery({ queryKey: adminKeys.mappings, queryFn: async () => unwrap(await api.GET("/git/mappings")).mappings });
}

export function useAddGitMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["SetGitRepoMappingRequest"]) => unwrap(await api.POST("/git/mappings", { body })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: adminKeys.mappings }),
  });
}

export function useRemoveGitMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (repo: string) => {
      const [owner = "", name = ""] = repo.split("/");
      unwrap(await api.DELETE("/git/mappings/{owner}/{repo}", { params: { path: { owner, repo: name } } }));
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: adminKeys.mappings }),
  });
}

export function useUsers(enabled: boolean) {
  return useQuery({ queryKey: adminKeys.users, enabled, queryFn: async () => unwrap(await api.GET("/admin/users")).users });
}

function useUsersRefresh() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: adminKeys.users });
    void qc.invalidateQueries({ queryKey: ["identity"] });
  };
}

export function useCreateUser() {
  const refresh = useUsersRefresh();
  return useMutation({
    mutationFn: async (body: { handle: string; display_name: string; role: Schemas["Role"]; password: string }) =>
      unwrap(await api.POST("/actors", { body: { kind: "human", ...body } })),
    onSuccess: refresh,
  });
}

export function useSetRole() {
  const refresh = useUsersRefresh();
  return useMutation({
    mutationFn: async ({ id, role }: { id: string; role: Schemas["Role"] }) =>
      unwrap(await api.PUT("/actors/{actorId}/role", { params: { path: { actorId: id } }, body: { role } })),
    onSuccess: refresh,
  });
}

export function useSetUserState() {
  const refresh = useUsersRefresh();
  return useMutation({
    mutationFn: async ({ id, state }: { id: string; state: "active" | "stopped" }) =>
      unwrap(await api.PUT("/actors/{actorId}/state", { params: { path: { actorId: id } }, body: { state } })),
    onSuccess: refresh,
  });
}

export function useResetPassword() {
  const refresh = useUsersRefresh();
  return useMutation({
    mutationFn: async ({ id, password }: { id: string; password: string }) =>
      unwrap(await api.PUT("/actors/{actorId}/password", { params: { path: { actorId: id } }, body: { new_password: password } })),
    onSuccess: refresh,
  });
}

export function useTokens(actorId: string | undefined) {
  return useQuery({
    queryKey: adminKeys.tokens(actorId ?? ""),
    enabled: Boolean(actorId),
    queryFn: async () => unwrap(await api.GET("/actors/{actorId}/tokens", { params: { path: { actorId: actorId ?? "" } } })).tokens,
  });
}

export function useIssueToken(actorId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (label: string) => unwrap(await api.POST("/actors/{actorId}/tokens", { params: { path: { actorId: actorId ?? "" } }, body: { label } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: adminKeys.tokens(actorId ?? "") }),
  });
}

export function useRevokeToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (tokenId: string) => unwrap(await api.DELETE("/tokens/{tokenId}", { params: { path: { tokenId } } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "tokens"] }),
  });
}

export type AuditFilters = { actor_id?: string; action?: string[]; target_type?: string; from?: string; to?: string };

export function useAuditPages(filters: AuditFilters) {
  return useInfiniteQuery<Schemas["AuditPage"], Error, InfiniteData<Schemas["AuditPage"], number>, QueryKey, number>({
    queryKey: adminKeys.audit(filters),
    initialPageParam: 0,
    getNextPageParam: (last) => (last.entries.length === 50 ? last.entries[last.entries.length - 1]?.seq : undefined),
    queryFn: async ({ pageParam }) =>
      unwrap(await api.GET("/admin/audit", { params: { query: { ...filters, limit: 50, ...(pageParam ? { before: pageParam } : {}) } } })),
  });
}

export function useAuditActions() {
  return useQuery({ queryKey: adminKeys.auditActions, queryFn: async () => unwrap(await api.GET("/admin/audit/actions")).actions });
}

export function useVerifyAudit() {
  return useMutation({ mutationFn: async () => unwrap(await api.GET("/admin/audit/verify")) });
}

export function registerAdminEvents(qc: QueryClient): () => void {
  const refetch = () => {
    void qc.invalidateQueries({ queryKey: adminKeys.status });
    void qc.invalidateQueries({ queryKey: adminKeys.system });
  };
  const offs = [onEvent("workspace.emergency_activated", refetch), onEvent("workspace.emergency_cleared", refetch), onResync(refetch)];
  return () => offs.forEach((off) => off());
}
