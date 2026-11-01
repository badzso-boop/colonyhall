import { useQuery } from "@tanstack/react-query";
import { ApiError, api, unwrap, type Schemas } from "@/shared/api";

export type Entry = Schemas["FsEntry"];
export type FetchRequest = Schemas["FileFetchRequest"];

async function relay(agentId: string, body: Schemas["AgentFsRequest"]) {
  const r = unwrap(await api.POST("/agents/{agentId}/fs", { params: { path: { agentId } }, body }));
  if (r.error_code) throw Object.assign(new Error(r.error_code), { code: r.error_code });
  return r;
}

/** One folder of the agent's tree ("" = the top). Entries: folders first, by name. */
export function useDirectory(agentId: string, channelId: string, path: string, enabled = true) {
  return useQuery({
    queryKey: ["files", agentId, "dir", path],
    enabled,
    staleTime: 30_000,
    queryFn: async () => {
      const r = await relay(agentId, { channel_id: channelId, op: "list", path });
      const entries = [...(r.entries ?? [])].sort((a, b) => (a.kind === "dir") === (b.kind === "dir") ? a.name.localeCompare(b.name) : a.kind === "dir" ? -1 : 1);
      return { roots: r.roots ?? [], entries, truncated: r.truncated ?? false };
    },
  });
}

export function useGitStatus(agentId: string, channelId: string) {
  return useQuery({
    queryKey: ["files", agentId, "git"],
    staleTime: 30_000,
    queryFn: async () => (await relay(agentId, { channel_id: channelId, op: "git_status" })).git_status ?? [],
  });
}

export function useFileSearch(agentId: string, channelId: string, query: string) {
  return useQuery({
    queryKey: ["files", agentId, "search", query],
    enabled: query.trim().length >= 2,
    queryFn: async () => {
      const r = await relay(agentId, { channel_id: channelId, op: "search", query: query.trim() });
      return { results: r.results ?? [], truncated: r.truncated ?? false };
    },
  });
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Asks the guard for the file and waits until it is stored (or fails): at most 2 minutes. */
export async function fetchFile(agentId: string, channelId: string, path: string, refresh = false): Promise<FetchRequest> {
  let req = unwrap(await api.POST("/agents/{agentId}/file-fetch", { params: { path: { agentId } }, body: { channel_id: channelId, path, refresh } }));
  const until = Date.now() + 120_000;
  while (req.state === "pending" && Date.now() < until) {
    await sleep(800);
    req = unwrap(await api.GET("/file-fetch/{id}", { params: { path: { id: req.id } } }));
  }
  return req;
}

/**
 * A local file linked in an agent's message (round 16, humans only): the
 * server asks the guard of the message's author, and the result is polled
 * until it is stored or fails, at most 2 minutes.
 */
export async function fetchMessageFile(messageId: string, path: string, refresh = false): Promise<FetchRequest> {
  let req = unwrap(await api.POST("/messages/{messageId}/file-fetch", { params: { path: { messageId } }, body: { path, refresh } }));
  const until = Date.now() + 120_000;
  while (req.state === "pending" && Date.now() < until) {
    await sleep(800);
    req = unwrap(await api.GET("/file-fetch/{id}", { params: { path: { id: req.id } } }));
  }
  return req.state === "pending" ? { ...req, state: "failed", error_code: "timeout" } : req;
}

export function useMessageFile(messageId: string, path: string | null, nonce: number) {
  return useQuery({
    queryKey: ["files", "message", messageId, path, nonce],
    enabled: path !== null,
    staleTime: Infinity,
    retry: false,
    queryFn: () => fetchMessageFile(messageId, path!, nonce > 0),
  });
}

export function useFetchedFile(agentId: string, channelId: string, path: string | null, nonce: number) {
  return useQuery({
    queryKey: ["files", agentId, "fetch", path, nonce],
    enabled: path !== null,
    staleTime: Infinity,
    retry: false,
    queryFn: () => fetchFile(agentId, channelId, path!, nonce > 0),
  });
}

/** The stored file's text; with a Range header for the head or tail of a large file. */
export function useFileText(fileId: string | null, range: string | null) {
  return useQuery({
    queryKey: ["files", "content", fileId, range],
    enabled: fileId !== null,
    staleTime: Infinity,
    queryFn: async () => {
      const res = await fetch(`/api/v1/files/${fileId}/content`, { credentials: "same-origin", headers: range ? { Range: range } : {} });
      if (!res.ok && res.status !== 206) throw new ApiError(res.status, undefined);
      return res.text();
    },
  });
}
