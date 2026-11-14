// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Note } from "./api";

export type ThreadGroup = { thread: string; notes: Note[]; last: string };
export type AgentGroup = { agentId: string; threads: ThreadGroup[] };

/** Notes by agent, then by thread; threads by their newest note, agents by name order given. */
export function groupNotes(notes: Note[], agentOrder: string[]): AgentGroup[] {
  const byAgent = new Map<string, Map<string, Note[]>>();
  for (const n of notes) {
    const threads = byAgent.get(n.agent_id) ?? new Map<string, Note[]>();
    threads.set(n.thread_root_id, [...(threads.get(n.thread_root_id) ?? []), n]);
    byAgent.set(n.agent_id, threads);
  }
  return agentOrder
    .filter((id) => byAgent.has(id))
    .map((agentId) => ({
      agentId,
      threads: [...byAgent.get(agentId)!.entries()]
        .map(([thread, list]) => {
          const sorted = [...list].sort((a, b) => b.created_at.localeCompare(a.created_at));
          return { thread, notes: sorted, last: sorted[0]!.created_at };
        })
        .sort((a, b) => b.last.localeCompare(a.last)),
    }));
}

/** The notes matching every word of the query (case- and accent-insensitive). */
export function searchNotes(notes: Note[], query: string): Note[] {
  const fold = (s: string) => s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();
  const words = fold(query).split(/\s+/).filter(Boolean);
  if (!words.length) return notes;
  return notes.filter((n) => words.every((w) => fold(n.text).includes(w)));
}

/** The authors of a thread that are agents (whose notebooks can hold notes on it). */
export function agentAuthors(messages: { author_id: string }[], isAgent: (id: string) => boolean): string[] {
  return [...new Set(messages.map((m) => m.author_id))].filter(isAgent);
}
