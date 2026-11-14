// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { Navigate } from "react-router";
import { EmptyState } from "@/shared/ui";
import { useChannels } from "@/features/conversation";

/**
 * "/": on a phone the conversation list (the sidebar) is the home screen;
 * on wider screens the first channel opens.
 */
export function Home() {
  const { t } = useTranslation("conversation");
  const channels = useChannels();
  const wide = typeof window !== "undefined" && window.matchMedia?.("(min-width: 768px)").matches;
  const first = channels.data?.find((c) => c.kind !== "dm" && c.is_member) ?? channels.data?.[0];
  if (wide && first) return <Navigate to={`/c/${first.id}`} replace />;
  return <EmptyState title={t("empty.title")} body={t("empty.body")} />;
}
