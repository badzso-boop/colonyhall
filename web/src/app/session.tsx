// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Navigate, useLocation, useNavigate } from "react-router";
import { ApiError, WsClient, onUnauthorized, wsURL } from "@/shared/api";
import { registerActivityEvents } from "@/features/activity";
import { registerAdminEvents } from "@/features/admin";
import { registerAgentEvents } from "@/features/agents";
import { registerApprovalEvents } from "@/features/approvals";
import { registerCompactionEvents } from "@/features/compaction";
import { registerConversationEvents } from "@/features/conversation";
import { registerFilesEvents } from "@/features/files";
import { registerHostEvents } from "@/features/hosts";
import { registerIdentityEvents, useSession } from "@/features/identity";
import { registerLeaseEvents } from "@/features/leases";
import { registerPlannerEvents } from "@/features/planner";
import { registerPreviewEvents } from "@/features/previews";
import { registerQuestionEvents } from "@/features/questions";
import { registerRulesEvents } from "@/features/rules";
import { registerSavedEvents } from "@/features/saved";
import { registerScheduleEvents } from "@/features/schedules";
import { registerWikiEvents } from "@/features/wiki";

/** Sends the user to /login (and back afterwards) when the session is gone. */
function useUnauthorizedRedirect() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  useEffect(() => {
    onUnauthorized(() => {
      const here = window.location.pathname.replace(/^\/app/, "") + window.location.search;
      qc.clear();
      navigate(`/login?next=${encodeURIComponent(here)}`, { replace: true });
    });
    return () => onUnauthorized(() => {});
  }, [navigate, qc]);
}

/** One WebSocket for the signed-in app; features subscribe to its events. */
function useLiveUpdates(enabled: boolean) {
  const qc = useQueryClient();
  useEffect(() => {
    if (!enabled) return;
    const offs = [
      registerIdentityEvents(qc),
      registerConversationEvents(qc),
      registerQuestionEvents(qc),
      registerApprovalEvents(qc),
      registerCompactionEvents(qc),
      registerAgentEvents(qc),
      registerAdminEvents(qc),
      registerPlannerEvents(qc),
      registerScheduleEvents(qc),
      registerWikiEvents(qc),
      registerLeaseEvents(qc),
      registerRulesEvents(qc),
      registerHostEvents(qc),
      registerPreviewEvents(qc),
      registerFilesEvents(qc),
      registerSavedEvents(qc),
      registerActivityEvents(qc),
    ];
    const ws = new WsClient(wsURL());
    ws.start();
    return () => {
      ws.stop();
      offs.forEach((off) => off());
    };
  }, [enabled, qc]);
}

/** Renders the app only for a signed-in user; otherwise goes to /login. */
export function RequireSession({ children }: { children: ReactNode }) {
  const session = useSession();
  const location = useLocation();
  useUnauthorizedRedirect();
  // Round 51b: the operator's read-only break-glass session has no socket
  // (the server refuses it); the app shows what it loads, without live updates.
  useLiveUpdates(session.isSuccess && !session.data.break_glass);

  if (session.isPending) return null;
  if (session.error) {
    if (session.error instanceof ApiError && session.error.status === 401) {
      return <Navigate to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`} replace />;
    }
    throw session.error;
  }
  return <>{children}</>;
}
