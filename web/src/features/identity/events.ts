// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { QueryClient } from "@tanstack/react-query";
import { onEvent, onResync } from "@/shared/api";
import { identityKeys } from "./api";

/** Keeps the actor list fresh from the WebSocket. */
export function registerIdentityEvents(qc: QueryClient): () => void {
  const refetch = () => void qc.invalidateQueries({ queryKey: identityKeys.actors });
  const offs = [
    onEvent("actor.created", refetch),
    onEvent("actor.state_changed", refetch),
    onEvent("actor.profile_updated", refetch),
    onResync(refetch),
  ];
  return () => offs.forEach((off) => off());
}
