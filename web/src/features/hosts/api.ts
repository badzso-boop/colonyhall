import { useQuery } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

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
