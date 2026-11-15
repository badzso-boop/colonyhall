// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { Eye } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { useSession } from "../api";

/**
 * Shown on every page of the operator's break-glass session (round 51b):
 * read-only, until when. Never dismissable.
 */
export function BreakGlassBanner() {
  const { t, i18n } = useTranslation("identity");
  const bg = useSession().data?.break_glass;
  if (!bg) return null;
  const until = new Date(bg.expires_at).toLocaleTimeString(localeOf(i18n.language), { hour: "2-digit", minute: "2-digit" });
  return (
    <div role="status" className="flex items-center justify-center gap-2 border-b border-warning/40 bg-warning-soft px-4 py-1.5 text-center text-ui font-medium text-warning">
      <Eye aria-hidden className="size-4 shrink-0" />
      {t("breakGlass.banner", { operator: bg.operator_handle, until })}
    </div>
  );
}
