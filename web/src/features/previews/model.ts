// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Schemas } from "@/shared/api";

export type Preview = Schemas["Preview"];
export type PreviewState = Preview["state"];

/** The split view's width (round 57), kept in localStorage. */
export const WIDTH_KEY = "colonyhall.preview.width";
export const MIN_WIDTH = 320;
export const DEFAULT_WIDTH = 500;
export const MAX_RATIO = 0.8;
/** Below this the preview is a full-screen sheet, not a column. */
export const NARROW_BELOW = 768;

/** The width kept between MIN_WIDTH and 80% of the viewport. */
export function clampWidth(width: number, viewport: number): number {
  const max = Math.max(MIN_WIDTH, Math.floor(viewport * MAX_RATIO));
  if (!Number.isFinite(width)) return Math.min(DEFAULT_WIDTH, max);
  return Math.round(Math.min(max, Math.max(MIN_WIDTH, width)));
}

/** The width after a drag: the divider is on the column's left edge. */
export function dragWidth(startWidth: number, startX: number, x: number, viewport: number): number {
  return clampWidth(startWidth + (startX - x), viewport);
}

export const isLive = (p: Pick<Preview, "state">) => p.state === "running" || p.state === "requested";

/**
 * The conversation's preview: the newest live one announced in this
 * channel (and thread, when given), or null.
 */
export function previewFor(previews: Preview[], channelId: string, threadRootId?: string | null): Preview | null {
  return here(previews, channelId, threadRootId).find(isLive) ?? null;
}

/**
 * The newest preview of this conversation in any state, or null: a
 * stopped one can be restarted with its last compose file.
 */
export function lastPreviewFor(previews: Preview[], channelId: string, threadRootId?: string | null): Preview | null {
  return here(previews, channelId, threadRootId)[0] ?? null;
}

function here(previews: Preview[], channelId: string, threadRootId?: string | null): Preview[] {
  return previews
    .filter((p) => p.channel_id === channelId && (!threadRootId || p.thread_root_id === threadRootId))
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
}

/**
 * The preview a link in a message points at: its proxy address or the
 * address the guard reported (the one the chat announcement carries).
 */
export function previewForLink(previews: Preview[], href: string): Preview | null {
  let u: URL;
  try {
    u = new URL(href);
  } catch {
    return null;
  }
  for (const p of previews) {
    for (const base of [p.proxy_url, p.url]) {
      if (!base) continue;
      try {
        const b = new URL(base);
        const path = b.pathname.replace(/\/$/, "");
        if (u.origin === b.origin && (u.pathname === path || u.pathname.startsWith(`${path}/`))) return p;
      } catch {
        // an address that is not a URL never matches
      }
    }
  }
  return null;
}

/** What the pane shows instead of the frame, or null to show the frame. */
export function blocker(p: Preview): "noProxy" | "offline" | null {
  if (p.state !== "running") return "offline";
  if (!p.proxy_url) return "noProxy";
  return null;
}
