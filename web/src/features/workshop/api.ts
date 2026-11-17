// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

export type Connector = Schemas["Connector"];
export type ToolGrant = Schemas["ToolGrant"];
export type ToolCall = Schemas["ToolCallEntry"];
export type ToolRisk = Schemas["ToolRisk"];

export const workshopKeys = {
  connectors: ["workshop", "connectors"] as const,
  grants: ["workshop", "grants"] as const,
  calls: (targetId: string) => ["workshop", "calls", targetId] as const,
};

/** The workspace's connectors (owner and admins; round 56a). */
export function useConnectors(enabled = true) {
  return useQuery({
    queryKey: workshopKeys.connectors,
    enabled,
    queryFn: async () => unwrap(await api.GET("/workshop/connectors")),
  });
}

function useInvalidate() {
  const qc = useQueryClient();
  return () => void qc.invalidateQueries({ queryKey: ["workshop"] });
}

export function useCreateConnector() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (body: Schemas["CreateConnectorRequest"]) =>
      unwrap(await api.POST("/workshop/connectors", { body })),
    onSuccess: done,
  });
}

export function useUpdateConnector(id: string) {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (body: Schemas["UpdateConnectorRequest"]) =>
      unwrap(
        await api.PATCH("/workshop/connectors/{connectorId}", {
          params: { path: { connectorId: id } },
          body,
        }),
      ),
    onSuccess: done,
  });
}

export function useDeleteConnector() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.DELETE("/workshop/connectors/{connectorId}", {
          params: { path: { connectorId: id } },
        }),
      ),
    onSuccess: done,
  });
}

export function useRefreshConnector() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/workshop/connectors/{connectorId}/refresh", {
          params: { path: { connectorId: id } },
        }),
      ),
    onSuccess: done,
  });
}

export function useSetApprovalOff() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async ({ id, off }: { id: string; off: boolean }) =>
      unwrap(
        await api.PUT("/workshop/connectors/{connectorId}/approval", {
          params: { path: { connectorId: id } },
          body: { approval_off: off },
        }),
      ),
    onSuccess: done,
  });
}

export function useSetToolRisk() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async ({
      id,
      tool,
      risk,
    }: {
      id: string;
      tool: string;
      risk: Schemas["ToolRiskRequest"]["risk"];
    }) =>
      unwrap(
        await api.PUT("/workshop/connectors/{connectorId}/tools/{tool}/risk", {
          params: { path: { connectorId: id, tool } },
          body: { risk },
        }),
      ),
    onSuccess: done,
  });
}

export function useGrants(enabled = true) {
  return useQuery({
    queryKey: workshopKeys.grants,
    enabled,
    queryFn: async () => unwrap(await api.GET("/workshop/grants")).grants,
  });
}

export function useAddGrant() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (body: Schemas["AddToolGrantRequest"]) =>
      unwrap(await api.POST("/workshop/grants", { body })),
    onSuccess: done,
  });
}

export function useRevokeGrant() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.DELETE("/workshop/grants/{grantId}", {
          params: { path: { grantId: id } },
        }),
      ),
    onSuccess: done,
  });
}

export function useToolCalls(targetId: string, enabled = true) {
  return useQuery({
    queryKey: workshopKeys.calls(targetId),
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/workshop/calls", {
          params: {
            query: { limit: 100, ...(targetId ? { target_id: targetId } : {}) },
          },
        }),
      ).calls,
  });
}
