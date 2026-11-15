// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, setCsrfToken, unwrap, type Schemas } from "@/shared/api";

export const operatorKeys = {
  session: ["operator", "session"] as const,
  customers: ["operator", "customers"] as const,
  customer: (id: string) => ["operator", "customers", id] as const,
  audit: ["operator", "audit"] as const,
  database: ["operator", "database"] as const,
};

/** The operator's session and its CSRF token; 401 without one. */
export function useOperatorSession() {
  return useQuery({
    queryKey: operatorKeys.session,
    queryFn: async () => {
      const s = unwrap(await api.GET("/operator/auth/session"));
      setCsrfToken(s.csrf_token);
      return s;
    },
    retry: false,
    staleTime: 60_000,
  });
}

export function useOperatorLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["OperatorLoginRequest"]) => unwrap(await api.POST("/operator/auth/login", { body })),
    onSuccess: (s) => {
      setCsrfToken(s.csrf_token);
      qc.setQueryData(operatorKeys.session, s);
    },
  });
}

export function useOperatorLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      await api.POST("/operator/auth/logout");
    },
    onSettled: () => {
      setCsrfToken("");
      qc.clear();
    },
  });
}

export function useRenewBackupCodes() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["OperatorReauthRequest"]) => unwrap(await api.POST("/operator/me/backup-codes", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: operatorKeys.session }),
  });
}

export function useCustomers() {
  return useQuery({
    queryKey: operatorKeys.customers,
    queryFn: async () => unwrap(await api.GET("/operator/workspaces")).customers,
  });
}

export function useCustomer(id: string) {
  return useQuery({
    queryKey: operatorKeys.customer(id),
    queryFn: async () => unwrap(await api.GET("/operator/workspaces/{workspaceId}", { params: { path: { workspaceId: id } } })),
  });
}

export function useCreateCustomer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["CreateCustomerRequest"]) => unwrap(await api.POST("/operator/workspaces", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: operatorKeys.customers }),
  });
}

export function useInviteOwner(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/operator/workspaces/{workspaceId}/invite", { params: { path: { workspaceId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: operatorKeys.customers }),
  });
}

export function useSuspendCustomer(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (reason: string) =>
      unwrap(await api.POST("/operator/workspaces/{workspaceId}/suspend", { params: { path: { workspaceId: id } }, body: { reason } })),
    onSuccess: (c) => {
      qc.setQueryData(operatorKeys.customer(id), c);
      void qc.invalidateQueries({ queryKey: operatorKeys.customers });
    },
  });
}

export function useRestoreCustomer(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/operator/workspaces/{workspaceId}/restore", { params: { path: { workspaceId: id } } })),
    onSuccess: (c) => {
      qc.setQueryData(operatorKeys.customer(id), c);
      void qc.invalidateQueries({ queryKey: operatorKeys.customers });
    },
  });
}

const auditPage = 50;

/** The operator audit, newest first, 50 at a time. */
export function useOperatorAudit() {
  return useInfiniteQuery({
    queryKey: operatorKeys.audit,
    initialPageParam: "",
    queryFn: async ({ pageParam }) =>
      unwrap(await api.GET("/operator/audit", { params: { query: { limit: auditPage, ...(pageParam ? { before: pageParam } : {}) } } })).entries,
    getNextPageParam: (last) => (last.length === auditPage ? last[last.length - 1]?.created_at : undefined),
  });
}

export function useOperatorDatabase() {
  return useQuery({
    queryKey: operatorKeys.database,
    queryFn: async () => unwrap(await api.GET("/operator/system/database")),
  });
}

// Round 51b: plans and break-glass access.
export type Plan = Schemas["Plan"];
export type BreakGlass = Schemas["BreakGlass"];

const planKeys = {
  all: ["operator", "plans"] as const,
  check: (ws: string, plan: string) => ["operator", "plan-check", ws, plan] as const,
  breakGlass: (ws: string) => ["operator", "break-glass", ws] as const,
};

export function usePlans() {
  return useQuery({ queryKey: planKeys.all, queryFn: async () => unwrap(await api.GET("/operator/plans")).plans });
}

export function useCreatePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["PlanRequest"]) => unwrap(await api.POST("/operator/plans", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: planKeys.all }),
  });
}

export function useUpdatePlan(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["PlanPatchRequest"]) => unwrap(await api.PATCH("/operator/plans/{planId}", { params: { path: { planId: id } }, body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: planKeys.all });
      void qc.invalidateQueries({ queryKey: operatorKeys.customers });
    },
  });
}

/** What a plan would mean for a workspace's storage (before assigning it). */
export function usePlanCheck(workspaceId: string, planId: string) {
  return useQuery({
    queryKey: planKeys.check(workspaceId, planId),
    queryFn: async () =>
      unwrap(await api.GET("/operator/workspaces/{workspaceId}/plan-check", { params: { path: { workspaceId }, query: { plan_id: planId } } })),
    enabled: planId !== "",
  });
}

export function useAssignPlan(workspaceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["AssignPlanRequest"]) =>
      unwrap(await api.PUT("/operator/workspaces/{workspaceId}/plan", { params: { path: { workspaceId } }, body })),
    onSuccess: (c) => {
      qc.setQueryData(operatorKeys.customer(workspaceId), c);
      void qc.invalidateQueries({ queryKey: operatorKeys.customers });
      void qc.invalidateQueries({ queryKey: planKeys.all });
    },
  });
}

export function useBreakGlassList(workspaceId: string) {
  return useQuery({
    queryKey: planKeys.breakGlass(workspaceId),
    queryFn: async () => unwrap(await api.GET("/operator/workspaces/{workspaceId}/break-glass", { params: { path: { workspaceId } } })).break_glass,
  });
}

export function useOpenBreakGlass(workspaceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["BreakGlassRequest"]) =>
      unwrap(await api.POST("/operator/workspaces/{workspaceId}/break-glass", { params: { path: { workspaceId } }, body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: planKeys.breakGlass(workspaceId) });
      void qc.invalidateQueries({ queryKey: operatorKeys.customer(workspaceId) });
    },
  });
}

export function useCloseOperatorBreakGlass(workspaceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.DELETE("/operator/workspaces/{workspaceId}/break-glass/{breakGlassId}", { params: { path: { workspaceId, breakGlassId: id } } }),
      );
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: planKeys.breakGlass(workspaceId) });
      void qc.invalidateQueries({ queryKey: operatorKeys.customer(workspaceId) });
    },
  });
}
