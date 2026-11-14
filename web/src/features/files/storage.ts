import { useInfiniteQuery, useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { ApiError, api, onEvent, onResync, unwrap, type Schemas } from "@/shared/api";

// Round 54: "My files", deleting a file, the workspace storage and its quota.

export type MyFile = Schemas["MyFile"];
export type WorkspaceStorage = Schemas["WorkspaceStorage"];
export type StorageState = Schemas["StorageState"];
export type Uploader = "me" | "agents";
export type FileKind = "image" | "document" | "other";

export const storageKeys = {
  all: ["storage"] as const,
  mine: (uploader: Uploader, channel: string, kind: string) => ["storage", "mine", uploader, channel, kind] as const,
  workspace: ["storage", "workspace"] as const,
};

const PAGE = 50;

/** The person's files (or their agents'), newest first, page by page. */
export function useMyFiles(uploader: Uploader, channel: string, kind: FileKind | "") {
  return useInfiniteQuery({
    queryKey: storageKeys.mine(uploader, channel, kind),
    initialPageParam: "",
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/me/files", {
          params: {
            query: {
              uploader,
              limit: PAGE,
              ...(channel ? { channel } : {}),
              ...(kind ? { kind } : {}),
              ...(pageParam ? { cursor: pageParam } : {}),
            },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor,
  });
}

/** Deletes the files one by one; returns the IDs that failed (403, network). */
export function useDeleteFiles() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (ids: string[]) => {
      const failed: string[] = [];
      for (const fileId of ids) {
        const r = await api.DELETE("/files/{fileId}", { params: { path: { fileId } } });
        // 410: someone (another tab) deleted it already; that is done too.
        if (r.error && r.response.status !== 410) failed.push(fileId);
      }
      return failed;
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: storageKeys.all });
      void qc.invalidateQueries({ queryKey: ["conversation"] });
    },
  });
}

/** The workspace storage, for the owner and admins only (the server answers 403 otherwise). */
export function useWorkspaceStorage(enabled: boolean) {
  return useQuery({
    queryKey: storageKeys.workspace,
    enabled,
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
    queryFn: async () => unwrap(await api.GET("/workspace/storage")),
  });
}

/** Sets the quota in bytes (0: unlimited). Owner only. */
export function useSetQuota() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (quotaBytes: number) => unwrap(await api.PUT("/workspace/storage/quota", { body: { quota_bytes: quotaBytes } })),
    onSuccess: (st) => qc.setQueryData(storageKeys.workspace, st),
  });
}

/** The numbers of a 507 "storage_full" answer, or null for any other error. */
export function storageFull(e: unknown): { used: number; quota: number } | null {
  if (!(e instanceof ApiError) || e.status !== 507) return null;
  const p = e.problem as { used_bytes?: number; quota_bytes?: number } | undefined;
  return { used: p?.used_bytes ?? 0, quota: p?.quota_bytes ?? 0 };
}

/** A file deleted anywhere or a storage state change: refetch what shows them. */
export function registerFilesEvents(qc: QueryClient): () => void {
  const offs = [
    onEvent("file.deleted", () => void qc.invalidateQueries({ queryKey: [...storageKeys.all, "mine"] })),
    onEvent("workspace.storage_state_changed", () => void qc.invalidateQueries({ queryKey: storageKeys.workspace })),
    onResync(() => void qc.invalidateQueries({ queryKey: storageKeys.all })),
  ];
  return () => offs.forEach((off) => off());
}
