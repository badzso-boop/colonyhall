// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useActiveThread, useNotesOfThread, useSeen } from "./active";

/** The notes circle's badge: a dot for notes in the active thread newer than the last ones shown. */
export function useNotesBadge(): { text: string | null; news: boolean } {
  const { thread } = useActiveThread();
  const { notes } = useNotesOfThread(thread);
  const seen = useSeen((s) => (thread ? s.seen[thread] : undefined));
  const newest = notes[0]?.created_at;
  const news = Boolean(thread && newest && (!seen || newest > seen));
  return { text: news ? "•" : null, news };
}
