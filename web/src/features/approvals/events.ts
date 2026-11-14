// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { QueryClient } from "@tanstack/react-query";
import { onResync, onUntypedEvent } from "@/shared/api";
import { approvalKeys } from "./api";

/** approval.* refetch the cards; delegation.* (and an answered question) the delegations and grants. */
export function registerApprovalEvents(qc: QueryClient): () => void {
  const approvals = () => void qc.invalidateQueries({ queryKey: [...approvalKeys.all, "approval"] });
  const delegations = () => {
    void qc.invalidateQueries({ queryKey: approvalKeys.delegations });
    void qc.invalidateQueries({ queryKey: approvalKeys.grants });
  };
  const offs = [
    ...["approval.requested", "approval.decided"].map((t) => onUntypedEvent(t, approvals)),
    ...[
      "delegation.created",
      "delegation.approved",
      "delegation.denied",
      "delegation.expired",
      "delegation.grant_created",
      "delegation.grant_revoked",
      "question.answered",
    ].map((t) => onUntypedEvent(t, delegations)),
    onResync(() => void qc.invalidateQueries({ queryKey: approvalKeys.all })),
  ];
  return () => offs.forEach((off) => off());
}
