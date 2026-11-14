// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { fullTime } from "@/shared/lib";
import { EmptyState, Sheet, Skeleton } from "@/shared/ui";
import { useRunError } from "../api";

/**
 * "[error:<id>]" in a message: errors never go into the chat, only their
 * id (CLAUDE.md). Humans open the details from /errors/{id} in a Sheet;
 * for anyone else the chip has no action and shows no details.
 */
export function ErrorChip({ errorId, human }: { errorId: string; human: boolean }) {
  const { t, i18n } = useTranslation("agents");
  const [open, setOpen] = useState(false);
  const err = useRunError(open ? errorId : null);
  const chip = "mx-0.5 inline-flex items-center gap-1 rounded-full border border-danger/40 bg-danger-soft px-1.5 align-baseline text-meta font-medium text-danger";
  if (!human) {
    return (
      <span className={chip}>
        <TriangleAlert aria-hidden className="size-3" />
        {t("error.chip")}
      </span>
    );
  }
  return (
    <>
      <button type="button" onClick={() => setOpen(true)} title={t("error.open")} className={`${chip} hover:border-danger`}>
        <TriangleAlert aria-hidden className="size-3" />
        {t("error.chip")}
      </button>
      <Sheet open={open} onOpenChange={setOpen} title={t("error.title")} description={errorId} closeLabel={t("common:close")}>
        {err.isPending ? (
          <Skeleton className="h-16" />
        ) : err.isError || !err.data ? (
          <EmptyState title={t("error.unavailable")} />
        ) : (
          <div className="flex flex-col gap-2">
            <p className="text-body font-medium">{err.data.summary}</p>
            <p className="text-meta text-fg-muted">{fullTime(err.data.created_at, localeOf(i18n.language))}</p>
            {/* The detail is server-side text for humans; shown as text, never interpreted. */}
            <pre className="max-h-[60vh] overflow-auto rounded-md bg-surface-sunken p-2.5 font-mono text-meta break-all whitespace-pre-wrap">{err.data.detail}</pre>
          </div>
        )}
      </Sheet>
    </>
  );
}
