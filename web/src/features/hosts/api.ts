import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { ApiError, api, onResync, onUntypedEvent, unwrap, type Schemas } from "@/shared/api";

export type ClaudeLimits = Schemas["ClaudeLimits"];
export type Host = Schemas["Host"];
export type HostDirEntry = Schemas["HostDirEntry"];
export type Preview = Schemas["Preview"];

export const hostKeys = {
  list: ["hosts", "list"] as const,
  limits: ["hosts", "claude-limits"] as const,
  previews: ["hosts", "previews"] as const,
  dirs: (hostId: string, path: string) => ["hosts", hostId, "dirs", path] as const,
};

/** The most used Claude 5-hour window across the machines (admins), or null. */
export function pickClaudeLimits(hosts: Schemas["Host"][]): ClaudeLimits | null {
  let best: ClaudeLimits | null = null;
  for (const h of hosts) {
    const l = h.report.claude_limits;
    if (!l?.five_hour) continue;
    if (!best?.five_hour || l.five_hour.utilization > best.five_hour.utilization) best = l;
  }
  return best;
}

export function useClaudeLimits(enabled: boolean) {
  return useQuery({
    queryKey: hostKeys.limits,
    enabled,
    refetchInterval: 60_000,
    queryFn: async () => pickClaudeLimits(unwrap(await api.GET("/hosts")).hosts),
  });
}

/**
 * The machines with their report and agents (human admins). Reports and
 * last_seen_at change without an event, so an open page re-reads them.
 */
export function useHosts(enabled = true, refetchInterval?: number) {
  return useQuery({ queryKey: hostKeys.list, enabled, refetchInterval, queryFn: async () => unwrap(await api.GET("/hosts")).hosts });
}

/** Why a folder listing failed: the guard is not connected, it took too long, or something else. */
export type DirsFailure = "offline" | "timeout" | "error";

export function dirsFailure(err: unknown): DirsFailure {
  if (err instanceof ApiError) {
    if (err.status === 409 && err.problem?.detail === "host_offline") return "offline";
    if (err.status === 504) return "timeout";
  }
  return "error";
}

/** One folder of a machine (path "" = its roots), asked of its guard over the host relay. */
export function useHostDirs(hostId: string, path: string, enabled = true) {
  return useQuery({
    queryKey: hostKeys.dirs(hostId, path),
    enabled: enabled && Boolean(hostId),
    retry: false,
    staleTime: 30_000,
    queryFn: async () => unwrap(await api.POST("/hosts/{hostId}/fs", { params: { path: { hostId } }, body: { op: "list_dirs", ...(path ? { path } : {}) } })),
  });
}

/** Re-reads the machine list and the header's Claude meter, which comes from it. */
function invalidateHosts(qc: QueryClient) {
  void qc.invalidateQueries({ queryKey: hostKeys.list });
  void qc.invalidateQueries({ queryKey: hostKeys.limits });
}

/** Rename or recolour a machine (admins). */
export function useUpdateHost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ hostId, body }: { hostId: string; body: Schemas["UpdateHostRequest"] }) =>
      unwrap(await api.PATCH("/hosts/{hostId}", { params: { path: { hostId } }, body })),
    onSuccess: (host) => {
      qc.setQueryData<Host[]>(hostKeys.list, (old) => old?.map((h) => (h.id === host.id ? host : h)));
      invalidateHosts(qc);
    },
  });
}

/** Revoke a machine (every agent token it was given ends) or approve it again. */
export function useSetHostState() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ hostId, state }: { hostId: string; state: Schemas["SetHostStateRequest"]["state"] }) =>
      unwrap(await api.PUT("/hosts/{hostId}/state", { params: { path: { hostId } }, body: { state } })),
    onSettled: () => invalidateHosts(qc),
  });
}

/** A one-time pairing code (10 minutes) for `colony guard enroll`. */
export function useCreatePairingCode() {
  return useMutation({ mutationFn: async () => unwrap(await api.POST("/hosts/pairing-codes")) });
}

/** The workspace's previews; the caller filters them to a machine. */
export function usePreviews(enabled = true) {
  return useQuery({ queryKey: hostKeys.previews, enabled, queryFn: async () => unwrap(await api.GET("/previews")).previews });
}

export function useStopPreview() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (previewId: string) => unwrap(await api.POST("/previews/{previewId}/stop", { params: { path: { previewId } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: hostKeys.previews }),
  });
}

const hostEvents = [
  "host.enrolled",
  "host.updated",
  "host.state_changed",
  "host.agent_assigned",
  "host.agent_unassigned",
  "host.agent_limits_changed",
  "host.grant_added",
  "host.grant_revoked",
  "agent.created",
  "agent.archived",
];
const previewEvents = ["preview.requested", "preview.running", "preview.stopped", "preview.failed"];

/** Live updates of the machine pages: every host or agent change re-reads the list. */
export function registerHostEvents(qc: QueryClient): () => void {
  const hosts = () => invalidateHosts(qc);
  const previews = () => void qc.invalidateQueries({ queryKey: hostKeys.previews });
  const offs = [
    ...hostEvents.map((t) => onUntypedEvent(t, hosts)),
    ...previewEvents.map((t) => onUntypedEvent(t, previews)),
    onResync(hosts),
    onResync(previews),
  ];
  return () => offs.forEach((off) => off());
}
