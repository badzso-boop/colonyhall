// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

/**
 * Links in messages that the app opens itself instead of a new tab
 * (round 57: a preview's address opens the embedded preview). A feature
 * registers a handler; the message renderer asks before following a link.
 * A handler returns true when it took the link.
 */
type LinkHandler = (href: string) => boolean;

const handlers = new Set<LinkHandler>();

export function registerLinkHandler(h: LinkHandler): () => void {
  handlers.add(h);
  return () => {
    handlers.delete(h);
  };
}

/** True when a handler took the link (the caller then does not follow it). */
export function interceptLink(href: string): boolean {
  for (const h of handlers) if (h(href)) return true;
  return false;
}
