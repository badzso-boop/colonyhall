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

export type Extension = Schemas["Extension"];
export type ExtensionVersion = Schemas["ExtensionVersion"];
export type ExtensionDiff = Schemas["ExtensionDiff"];

export const extensionKeys = {
  list: ["workshop", "extensions"] as const,
  version: (id: string, v: number) =>
    ["workshop", "extensions", id, v] as const,
};

export function useExtensions(enabled = true) {
  return useQuery({
    queryKey: extensionKeys.list,
    enabled,
    queryFn: async () => unwrap(await api.GET("/workshop/extensions")),
  });
}

export function useExtensionVersion(
  id: string,
  version: number,
  enabled = true,
) {
  return useQuery({
    queryKey: extensionKeys.version(id, version),
    enabled: enabled && version > 0,
    queryFn: async () =>
      unwrap(
        await api.GET("/workshop/extensions/{extensionId}/versions/{version}", {
          params: { path: { extensionId: id, version } },
        }),
      ),
  });
}

export function useCreateExtension() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (body: Schemas["CreateExtensionRequest"]) =>
      unwrap(await api.POST("/workshop/extensions", { body })),
    onSuccess: done,
  });
}

export function useAddVersion(id: string) {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (body: Schemas["AddExtensionVersionRequest"]) =>
      unwrap(
        await api.POST("/workshop/extensions/{extensionId}/versions", {
          params: { path: { extensionId: id } },
          body,
        }),
      ),
    onSuccess: done,
  });
}

export function useApproveVersion(id: string) {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (version: number) =>
      unwrap(
        await api.POST(
          "/workshop/extensions/{extensionId}/versions/{version}/approve",
          { params: { path: { extensionId: id, version } } },
        ),
      ),
    onSuccess: done,
  });
}

export function useTestTool(id: string) {
  return useMutation({
    mutationFn: async ({
      version,
      tool,
      input,
    }: {
      version: number;
      tool: string;
      input: Record<string, unknown>;
    }) =>
      unwrap(
        await api.POST(
          "/workshop/extensions/{extensionId}/versions/{version}/test",
          {
            params: { path: { extensionId: id, version } },
            body: { tool, input },
          },
        ),
      ),
  });
}

export function useRollback(id: string) {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (version: number) =>
      unwrap(
        await api.POST("/workshop/extensions/{extensionId}/rollback", {
          params: { path: { extensionId: id } },
          body: { version },
        }),
      ),
    onSuccess: done,
  });
}

export function useSetExtensionEnabled(id: string) {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (enabled: boolean) =>
      unwrap(
        await api.PUT("/workshop/extensions/{extensionId}/enabled", {
          params: { path: { extensionId: id } },
          body: { enabled },
        }),
      ),
    onSuccess: done,
  });
}

export function useSetExtensionApprovalOff(id: string) {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (off: boolean) =>
      unwrap(
        await api.PUT("/workshop/extensions/{extensionId}/approval", {
          params: { path: { extensionId: id } },
          body: { approval_off: off },
        }),
      ),
    onSuccess: done,
  });
}

export function useTakeOver(id: string) {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/workshop/extensions/{extensionId}/take-over", {
          params: { path: { extensionId: id } },
        }),
      ),
    onSuccess: done,
  });
}

export function useDeleteExtension() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.DELETE("/workshop/extensions/{extensionId}", {
          params: { path: { extensionId: id } },
        }),
      ),
    onSuccess: done,
  });
}

export function useRoutines(id: string, enabled = true) {
  return useQuery({
    queryKey: ["workshop", "extensions", id, "routines"] as const,
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/workshop/extensions/{extensionId}/routines", {
          params: { path: { extensionId: id } },
        }),
      ),
  });
}

export function useCreateWebhook(id: string) {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (routine: string) =>
      unwrap(
        await api.POST(
          "/workshop/extensions/{extensionId}/routines/{routine}/webhook",
          { params: { path: { extensionId: id, routine } } },
        ),
      ),
    onSuccess: done,
  });
}

export type Bundle = Schemas["ExtensionBundle"];

export async function exportExtension(id: string): Promise<Bundle> {
  return unwrap(
    await api.GET("/workshop/extensions/{extensionId}/export", {
      params: { path: { extensionId: id } },
    }),
  );
}

export function useImportExtension() {
  const done = useInvalidate();
  return useMutation({
    mutationFn: async (body: Schemas["ImportExtensionRequest"]) =>
      unwrap(await api.POST("/workshop/extensions/import", { body })),
    onSuccess: done,
  });
}
