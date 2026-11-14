// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo } from "react";
import { create } from "zustand";
import { useThread } from "@/features/conversation";
import { useActors } from "@/features/identity";
import { useLocalStorage } from "@/shared/hooks";
import { usePanes } from "@/shared/panes";
import { useThreadNotes } from "./api";
import { agentAuthors } from "./model";

/** The thread the window shows: the pinned one, else the thread open in the thread pane. */
export function useActiveThread() {
  const [panes] = usePanes();
  const [pinned, setPinned] = useLocalStorage<string | null>("colony.notes.pin", null);
  return { thread: pinned ?? panes.thread, pinned, pin: (id: string | null) => setPinned(id) };
}

/** The notes of every agent that has written in the thread. */
export function useNotesOfThread(thread: string | null) {
  const actors = useActors();
  const t = useThread(thread);
  const agents = useMemo(() => {
    if (!t.data) return [];
    return agentAuthors([t.data.root, ...t.data.replies], (id) => actors.data?.get(id)?.kind === "agent");
  }, [t.data, actors.data]);
  return { agents, ...useThreadNotes(thread, agents), root: t.data?.root };
}

// The newest note seen per thread, for the minimised circle's dot (this tab only).
type Seen = { seen: Record<string, string>; mark: (thread: string, at: string) => void };
export const useSeen = create<Seen>((set) => ({
  seen: {},
  mark: (thread, at) => set((s) => (s.seen[thread] && s.seen[thread] >= at ? s : { seen: { ...s.seen, [thread]: at } })),
}));
