import { useQuery } from "@tanstack/react-query";
import { ApiError, api, unwrap, type Schemas } from "@/shared/api";

export type ClaudeLimits = Schemas["ClaudeLimits"];

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
    queryKey: ["hosts", "claude-limits"],
    enabled,
    refetchInterval: 60_000,
    queryFn: async () => pickClaudeLimits(unwrap(await api.GET("/hosts")).hosts),
  });
}

export type Host = Schemas["Host"];
export type HostDirEntry = Schemas["HostDirEntry"];

export const hostKeys = {
  list: ["hosts", "list"] as const,
  dirs: (hostId: string, path: string) => ["hosts", hostId, "dirs", path] as const,
};

/** The machines with their report and agents (human admins). */
export function useHosts(enabled = true) {
  return useQuery({ queryKey: hostKeys.list, enabled, queryFn: async () => unwrap(await api.GET("/hosts")).hosts });
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
