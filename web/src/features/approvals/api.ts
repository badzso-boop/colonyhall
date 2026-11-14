// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

export const approvalKeys = {
  all: ["approvals"] as const,
  approval: (id: string) => ["approvals", "approval", id] as const,
  delegations: ["approvals", "delegations"] as const,
  grants: ["approvals", "grants"] as const,
};

/** A tool permission request referenced as [approval:<id>] (humans only). */
export function useApproval(id: string, enabled: boolean) {
  return useQuery({
    queryKey: approvalKeys.approval(id),
    enabled,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/approvals/{approvalId}", { params: { path: { approvalId: id } } })),
  });
}

export function useDecideApproval(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (decision: Schemas["DecideApprovalRequest"]["decision"]) =>
      unwrap(await api.POST("/approvals/{approvalId}", { params: { path: { approvalId: id } }, body: { decision } })),
    onSuccess: (a) => qc.setQueryData(approvalKeys.approval(id), a),
  });
}

/** The workspace's delegation requests (humans): matched to messages by message id. */
export function useDelegations(enabled: boolean) {
  return useQuery({
    queryKey: approvalKeys.delegations,
    enabled,
    retry: false,
    staleTime: 30_000,
    queryFn: async () => unwrap(await api.GET("/delegations")).delegations,
  });
}

/** The active agent–agent grants (round 25). */
export function useActiveGrants(enabled: boolean) {
  return useQuery({
    queryKey: approvalKeys.grants,
    enabled,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/delegation-grants", { params: { query: { active: true } } })).grants,
  });
}

export function useRevokeGrant() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/delegation-grants/{id}/revoke", { params: { path: { id } } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: approvalKeys.grants }),
  });
}
