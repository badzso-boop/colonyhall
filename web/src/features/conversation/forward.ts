// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Channel } from "./model";

/**
 * Where a message can be forwarded (round 55, part F): conversations the
 * viewer is in and that are not archived, the source's own included (as a
 * new message). The server checks write access again.
 */
export function forwardTargets(channels: Channel[]): Channel[] {
  return channels.filter((c) => c.is_member && !c.archived_at);
}

/** Forwarding out of a private channel (or a DM) into another conversation needs a confirmation. */
export function needsLeakConfirm(source: Channel | undefined, targetId: string): boolean {
  return Boolean(source && source.kind !== "public" && source.id !== targetId);
}
