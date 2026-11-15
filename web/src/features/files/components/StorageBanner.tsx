// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { localeOf } from "@/shared/i18n";
import { cn, formatBytes } from "@/shared/lib";
import { useWorkspaceStorage } from "../storage";

/**
 * The storage warning under the header (round 54), for the owner and admins
 * only: at 80%, 95% and full. It names the numbers, never people or files.
 */
export function StorageBanner({ admin }: { admin: boolean }) {
  const { t, i18n } = useTranslation("files");
  const storage = useWorkspaceStorage(admin);
  const st = storage.data;
  if (!admin || !st || st.state === "ok" || !st.limit_bytes) return null;
  const locale = localeOf(i18n.language);
  const full = st.state === "full";
  return (
    <div role="status" className={cn("px-4 py-1 text-center text-meta font-medium", full ? "bg-danger-soft text-danger" : "bg-warning-soft text-warning")}>
      {t(`banner.${st.state}`, { used: formatBytes(st.used_bytes, locale), quota: formatBytes(st.limit_bytes, locale) })}{" "}
      <Link to="/admin/status" className="underline">
        {t("banner.details")}
      </Link>
    </div>
  );
}
