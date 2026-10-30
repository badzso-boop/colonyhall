import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, api, unwrap, type Schemas } from "@/shared/api";

export type Terminal = Schemas["TerminalSummary"];

/** The relay error as an i18n key under "errors.". */
export function relayError(e: unknown, code?: string): string {
  if (code) return code;
  if (e instanceof ApiError) {
    if (e.status === 504) return "timeout";
    if (e.status === 403) return "denied";
    if (e.status === 404) return "not_found";
  }
  return "generic";
}

async function relay(agentId: string, body: Schemas["AgentFsRequest"]) {
  const r = unwrap(await api.POST("/agents/{agentId}/fs", { params: { path: { agentId } }, body }));
  if (r.error_code) throw Object.assign(new Error(r.error_code), { code: r.error_code });
  return r;
}

export function useTerminals(agentId: string, channelId: string | null) {
  return useQuery({
    queryKey: ["terminals", agentId],
    enabled: channelId !== null,
    refetchInterval: 5000,
    queryFn: async () => (await relay(agentId, { channel_id: channelId!, op: "term_list" })).terminals ?? [],
  });
}

export function useTerminalOutput(agentId: string, channelId: string | null, name: string, running: boolean) {
  return useQuery({
    queryKey: ["terminals", agentId, name, "output"],
    enabled: channelId !== null,
    refetchInterval: running ? 2000 : false,
    queryFn: async () => {
      const r = await relay(agentId, { channel_id: channelId!, op: "term_output", name, lines: 1000 });
      return { output: r.output ?? "", truncated: r.truncated ?? false };
    },
  });
}

export function useKillTerminal(agentId: string, channelId: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) => relay(agentId, { channel_id: channelId!, op: "term_kill", name }),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["terminals", agentId] }),
  });
}
