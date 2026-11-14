// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useQueries, useQuery } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

export type Note = Schemas["AgentNote"];

// No event announces a new note: refetch every 30 s while visible, on focus
// and when the thread changes (round 32, part 5).
const Refetch = 30_000;

export const notesKeys = {
  agent: (agentId: string, thread?: string) => ["notes", agentId, thread ?? ""] as const,
};

async function fetchNotes(agentId: string, thread?: string): Promise<Note[]> {
  return unwrap(await api.GET("/agents/{id}/notes", { params: { path: { id: agentId }, query: thread ? { thread } : {} } })).notes;
}

/** Every note of these agents (the page). */
export function useAllNotes(agentIds: string[]) {
  return useQueries({
    queries: agentIds.map((id) => ({ queryKey: notesKeys.agent(id), queryFn: () => fetchNotes(id), refetchInterval: Refetch })),
    combine: (results) => ({ notes: results.flatMap((r) => r.data ?? []), pending: results.some((r) => r.isPending) }),
  });
}

/** The notes of one thread from the given agents, newest first. */
export function useThreadNotes(thread: string | null, agentIds: string[]) {
  return useQueries({
    queries: agentIds.map((id) => ({
      queryKey: notesKeys.agent(id, thread ?? ""),
      queryFn: () => fetchNotes(id, thread ?? undefined),
      enabled: thread !== null,
      refetchInterval: Refetch,
    })),
    combine: (results) => ({ notes: sortNotes(results.flatMap((r) => r.data ?? [])), pending: results.some((r) => r.isPending && r.fetchStatus !== "idle") }),
  });
}

export function sortNotes(notes: Note[]): Note[] {
  return [...notes].sort((a, b) => b.created_at.localeCompare(a.created_at) || a.id.localeCompare(b.id));
}

/** The first line of a thread's root message, for the list. */
export function useThreadRoot(rootId: string) {
  return useQuery({
    queryKey: ["notes", "root", rootId],
    queryFn: async () => unwrap(await api.GET("/messages/{messageId}/thread", { params: { path: { messageId: rootId } } })).root,
    staleTime: 5 * 60_000,
  });
}
