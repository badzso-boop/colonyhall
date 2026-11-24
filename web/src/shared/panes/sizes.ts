// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Width, WorkKind } from "./panes";

/** What a resizable side slot holds; each remembers its own width. */
export type SlotContent = "thread" | WorkKind | "left";

/** The narrowest a side slot may be dragged. */
export const MIN_SLOT = 320;
/** The room the main column (the chat) always keeps. */
export const MIN_MAIN = 360;

/**
 * The width a slot opens with: a thread or an agent overview is a column,
 * a file browser is wide enough for the tree beside the file (its two-column
 * layout starts at 720px), a terminal for 80+ columns.
 */
export function defaultSlotWidth(content: SlotContent, width: Width): number {
  if (content === "left") return 560;
  if (width === "medium") return content === "files" || content === "terminal" ? 560 : 380;
  switch (content) {
    case "files":
      return 760;
    case "terminal":
      return 640;
    default:
      return 440;
  }
}

/** Keeps a slot between MIN_SLOT and what leaves the main column MIN_MAIN of the free room. */
export function clampSlot(px: number, free: number): number {
  const max = Math.max(MIN_SLOT, free - MIN_MAIN);
  return Math.round(Math.min(Math.max(px, MIN_SLOT), max));
}
