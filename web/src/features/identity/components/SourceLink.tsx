// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { cn } from "@/shared/lib";
import { useVersion } from "../api";

/**
 * The version, the licence and a link to this server's source code: AGPL
 * section 13 asks a network service to offer its users the source (round 48).
 * Shown on the sign-in page and in the user menu.
 */
export function SourceLink({ className }: { className?: string }) {
  const { t } = useTranslation("identity");
  const v = useVersion();
  if (!v.data) return null;
  return (
    <p className={cn("text-meta text-fg-subtle", className)}>
      {t("about.line", { version: v.data.version, license: v.data.license })} ·{" "}
      <a href={v.data.source_url} target="_blank" rel="noopener noreferrer" title={t("about.sourceHint")} className="underline hover:text-fg">
        {t("about.source")}
      </a>
    </p>
  );
}
