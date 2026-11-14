// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Drafts (round 55, part A): one unsent message per conversation and
// thread. The server keeps them per person; localStorage is only the
// offline fallback, and the newer of the two wins.

export type DraftFile = { id: string; name: string; mime: string; size: number };

/** What the composer holds for one place. */
export type DraftContent = { body: string; files: DraftFile[]; at: string };

/** The key of a place: the conversation, and the thread or the main stream. */
export function draftKey(channelId: string, threadRootId?: string | null): string {
  return `${channelId}:${threadRootId ?? ""}`;
}

/** Nothing worth keeping: no text and no files. */
export function isEmptyDraft(body: string, files: readonly unknown[]): boolean {
  return body.trim() === "" && files.length === 0;
}

/**
 * The copy to restore: the server's unless the local one is newer (typed
 * while offline). A missing copy loses to any present one.
 */
export function pickDraft(server: DraftContent | undefined, local: DraftContent | undefined): DraftContent | undefined {
  if (!server) return local;
  if (!local) return server;
  return Date.parse(local.at) > Date.parse(server.at) ? local : server;
}

const storagePrefix = "colony.draft.";

/** Reads the local copy; anything unreadable counts as none. */
export function parseLocalDraft(raw: string | null): DraftContent | undefined {
  if (!raw) return undefined;
  try {
    const v: unknown = JSON.parse(raw);
    if (typeof v !== "object" || v === null) return undefined;
    const { body, files, at } = v as Record<string, unknown>;
    if (typeof body !== "string" || typeof at !== "string" || Number.isNaN(Date.parse(at))) return undefined;
    const list = Array.isArray(files)
      ? files.filter(
          (f): f is DraftFile =>
            typeof f === "object" && f !== null && typeof f.id === "string" && typeof f.name === "string" && typeof f.mime === "string" && typeof f.size === "number",
        )
      : [];
    return { body, files: list, at };
  } catch {
    return undefined;
  }
}

export function readLocalDraft(key: string): DraftContent | undefined {
  try {
    return parseLocalDraft(localStorage.getItem(storagePrefix + key));
  } catch {
    return undefined;
  }
}

export function writeLocalDraft(key: string, d: DraftContent | undefined): void {
  try {
    if (!d || isEmptyDraft(d.body, d.files)) localStorage.removeItem(storagePrefix + key);
    else localStorage.setItem(storagePrefix + key, JSON.stringify(d));
  } catch {
    // Private mode or full storage: the server copy still works.
  }
}
