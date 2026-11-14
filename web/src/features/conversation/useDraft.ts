// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useCallback, useEffect, useRef } from "react";
import { useDrafts, useDraftWriter } from "./api";
import type { AttachmentTray } from "./attachments";
import { draftKey, isEmptyDraft, pickDraft, readLocalDraft, writeLocalDraft, type DraftContent, type DraftFile } from "./drafts";

/** How long after the last keystroke the draft goes to the server. */
export const DRAFT_SAVE_DELAY = 1000;

type Options = {
  channelId: string;
  threadRootId?: string;
  /** Humans keep drafts; agents and observers do not. */
  enabled: boolean;
  text: string;
  setText: (s: string) => void;
  tray: AttachmentTray;
};

/**
 * Keeps the composer's text and uploads as the place's draft (round 55,
 * part A): restored when the place opens, saved ~1 s after typing stops,
 * when leaving the place and when the page hides; the local copy is written
 * at once as the offline fallback. `sent()` clears it after a send.
 */
export function useDraft({ channelId, threadRootId, enabled, text, setText, tray }: Options) {
  const drafts = useDrafts(enabled && channelId !== "");
  const writer = useDraftWriter();
  const key = draftKey(channelId, threadRootId);
  const loaded = useRef<string | null>(null);
  const dirty = useRef(false);
  const timer = useRef<number | undefined>(undefined);
  // Saves run one after another, so a delete after a send comes last.
  const chain = useRef<Promise<void>>(Promise.resolve());
  const latest = useRef<{ key: string; channelId: string; threadRootId?: string; body: string; files: DraftFile[] }>({ key, channelId, threadRootId, body: "", files: [] });

  const files: DraftFile[] = tray.items
    .filter((i) => i.status === "done" && i.fileId)
    .map((i) => ({ id: i.fileId!, name: i.file.name, mime: i.file.type, size: i.size ?? i.file.size }));
  const filesSig = files.map((f) => f.id).join(",");

  const enqueue = useCallback((job: () => Promise<void>) => {
    chain.current = chain.current.then(job).catch(() => {
      // The local copy stays; the next save tries again.
    });
    return chain.current;
  }, []);

  const flush = useCallback(() => {
    window.clearTimeout(timer.current);
    timer.current = undefined;
    if (!enabled || !dirty.current) return;
    dirty.current = false;
    const d = latest.current;
    const body = { channel_id: d.channelId, body: d.body, ...(d.threadRootId ? { thread_root_id: d.threadRootId } : {}), file_ids: d.files.map((f) => f.id) };
    void enqueue(() => writer.save(body));
  }, [enabled, enqueue, writer]);

  // Restore once per place, when the server list is there (or failed).
  useEffect(() => {
    if (!enabled || !channelId || loaded.current === key) return;
    if (drafts.isPending && !drafts.isError) return;
    loaded.current = key;
    dirty.current = false;
    const server = drafts.data?.find((d) => d.channel_id === channelId && (d.thread_root_id ?? "") === (threadRootId ?? ""));
    const pick = pickDraft(server ? { body: server.body, files: server.attachments, at: server.updated_at } : undefined, readLocalDraft(key));
    if (!pick || isEmptyDraft(pick.body, pick.files)) return;
    if (text === "" && !tray.items.length) {
      setText(pick.body);
      tray.restore(pick.files);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once per place; text and tray are read, not watched
  }, [enabled, key, channelId, threadRootId, drafts.isPending, drafts.isError, drafts.data]);

  // Every change: the local copy at once, the server after a pause.
  useEffect(() => {
    latest.current = { key, channelId, threadRootId, body: text, files };
    if (!enabled || loaded.current !== key) return;
    const content: DraftContent = { body: text, files, at: new Date().toISOString() };
    const prev = readLocalDraft(key);
    if (prev && prev.body === text && prev.files.map((f) => f.id).join(",") === filesSig) return;
    if (!prev && isEmptyDraft(text, files) && !dirty.current) return;
    writeLocalDraft(key, content);
    dirty.current = true;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(flush, DRAFT_SAVE_DELAY);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- files is rebuilt each render; filesSig is its identity
  }, [enabled, key, text, filesSig]);

  // Leaving the place (switching conversations or threads, closing the pane) and hiding the page save at once.
  useEffect(() => {
    const onHide = () => flush();
    window.addEventListener("pagehide", onHide);
    const onVisibility = () => document.visibilityState === "hidden" && flush();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("pagehide", onHide);
      document.removeEventListener("visibilitychange", onVisibility);
      flush();
    };
  }, [key, flush]);

  /** After a send: nothing to keep here any more. */
  const sent = useCallback(() => {
    window.clearTimeout(timer.current);
    timer.current = undefined;
    dirty.current = false;
    writeLocalDraft(key, undefined);
    if (!enabled) return;
    const d = latest.current;
    void enqueue(() => writer.remove(d.channelId, d.threadRootId));
  }, [enabled, enqueue, key, writer]);

  return { sent };
}
