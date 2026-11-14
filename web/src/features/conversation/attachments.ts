import { useCallback, useEffect, useReducer, useRef } from "react";
import { uploadFile, ApiError } from "@/shared/api";

// The composer's attachment tray (round 31, part 5): uploads start at
// once, sending waits for running uploads, failures can be retried.

export type Attachment = {
  key: number;
  file: File;
  status: "uploading" | "done" | "error";
  progress: number;
  fileId?: string;
  /** Set for an upload restored from a draft: the file itself is not here, only its metadata. */
  size?: number;
  /** "too_large" for a 413, otherwise a generic failure. */
  error?: "too_large" | "failed";
};

export type TrayAction =
  | { type: "add"; items: Array<{ key: number; file: File }> }
  | { type: "restore"; items: Array<{ key: number; file: File; fileId: string; size: number }> }
  | { type: "progress"; key: number; progress: number }
  | { type: "done"; key: number; fileId: string }
  | { type: "error"; key: number; error: "too_large" | "failed" }
  | { type: "retry"; key: number }
  | { type: "remove"; key: number }
  | { type: "clear" };

export function trayReducer(items: Attachment[], a: TrayAction): Attachment[] {
  const patch = (key: number, p: Partial<Attachment>) => items.map((i) => (i.key === key ? { ...i, ...p } : i));
  switch (a.type) {
    case "add":
      return [...items, ...a.items.map((x) => ({ ...x, status: "uploading" as const, progress: 0 }))];
    case "restore":
      return [...items, ...a.items.map((x) => ({ ...x, status: "done" as const, progress: 1 }))];
    case "progress":
      return patch(a.key, { progress: a.progress });
    case "done":
      return patch(a.key, { status: "done", progress: 1, fileId: a.fileId });
    case "error":
      return patch(a.key, { status: "error", error: a.error });
    case "retry":
      return patch(a.key, { status: "uploading", progress: 0, error: undefined });
    case "remove":
      return items.filter((i) => i.key !== a.key);
    case "clear":
      return [];
  }
}

/** Ready to send: nothing uploading; the ids of the uploaded files. */
export function trayState(items: Attachment[]): { busy: boolean; fileIds: string[]; failed: boolean } {
  return {
    busy: items.some((i) => i.status === "uploading"),
    fileIds: items.filter((i) => i.status === "done" && i.fileId).map((i) => i.fileId!),
    failed: items.some((i) => i.status === "error"),
  };
}

let nextKey = 1;

/** The tray of one composer: add files (picker, drop, paste), upload, retry, remove. */
export function useAttachments(channelId: string) {
  const [items, dispatch] = useReducer(trayReducer, []);
  const controllers = useRef(new Map<number, AbortController>());

  const start = useCallback(
    (key: number, file: File) => {
      const ctrl = new AbortController();
      controllers.current.set(key, ctrl);
      uploadFile(channelId, file, (p) => dispatch({ type: "progress", key, progress: p }), ctrl.signal)
        .then((f) => dispatch({ type: "done", key, fileId: f.id }))
        .catch((e: unknown) => {
          if (e instanceof DOMException && e.name === "AbortError") return;
          dispatch({ type: "error", key, error: e instanceof ApiError && e.status === 413 ? "too_large" : "failed" });
        })
        .finally(() => controllers.current.delete(key));
    },
    [channelId],
  );

  const add = useCallback(
    (files: Iterable<File>) => {
      const list = [...files].map((file) => ({ key: nextKey++, file }));
      if (!list.length) return;
      dispatch({ type: "add", items: list });
      list.forEach((x) => start(x.key, x.file));
    },
    [start],
  );

  const remove = useCallback((key: number) => {
    controllers.current.get(key)?.abort();
    dispatch({ type: "remove", key });
  }, []);

  const retry = useCallback(
    (a: Attachment) => {
      dispatch({ type: "retry", key: a.key });
      start(a.key, a.file);
    },
    [start],
  );

  const clear = useCallback(() => dispatch({ type: "clear" }), []);

  /** Puts a draft's earlier uploads back (already on the server). */
  const restore = useCallback((files: Array<{ id: string; name: string; mime: string; size: number }>) => {
    if (!files.length) return;
    dispatch({
      type: "restore",
      items: files.map((f) => ({ key: nextKey++, file: new File([], f.name, { type: f.mime }), fileId: f.id, size: f.size })),
    });
  }, []);

  useEffect(() => {
    const map = controllers.current;
    return () => map.forEach((c) => c.abort());
  }, []);

  return { items, add, remove, retry, clear, restore, ...trayState(items) };
}

export type AttachmentTray = ReturnType<typeof useAttachments>;
