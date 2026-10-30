import { useEffect, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Navigate, useLocation, useNavigate } from "react-router";
import { ApiError, WsClient, onUnauthorized, wsURL } from "@/shared/api";
import { registerAdminEvents } from "@/features/admin";
import { registerAgentEvents } from "@/features/agents";
import { registerConversationEvents } from "@/features/conversation";
import { registerIdentityEvents, useSession } from "@/features/identity";
import { registerQuestionEvents } from "@/features/questions";

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
      registerAgentEvents(),
      registerAdminEvents(qc),
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
  useLiveUpdates(session.isSuccess);

  if (session.isPending) return null;
  if (session.error) {
    if (session.error instanceof ApiError && session.error.status === 401) {
      return <Navigate to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`} replace />;
    }
    throw session.error;
  }
  return <>{children}</>;
}
