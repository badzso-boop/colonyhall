// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo } from "react";
import { useHosts } from "@/features/hosts";
import { useSession } from "@/features/identity";
import type { Viewer } from "./components/MessageItem";
import { claudePausedAgents } from "./messageParts";

/**
 * The viewer of a conversation, once per feed (MessageItem is memoised):
 * human or not, admin, write access here, and (admins, who can read the
 * machines) the agents waiting for the Claude limits.
 */
export function useViewer(canWrite: boolean): Viewer {
  const session = useSession();
  const me = session.data?.actor;
  const admin = me?.role === "owner" || me?.role === "admin";
  const hosts = useHosts(admin, 60_000);
  return useMemo(
    () => ({
      id: me?.id ?? "",
      human: me?.kind === "human",
      admin,
      canWrite,
      pausedAgents: claudePausedAgents(hosts.data ?? []),
    }),
    [me?.id, me?.kind, admin, canWrite, hosts.data],
  );
}
