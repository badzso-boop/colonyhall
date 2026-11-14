// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { CircleAlert, CircleCheck, TriangleAlert } from "lucide-react";
import { cn } from "@/shared/lib";
import type { Tone } from "../model";

const toneIcon = { ok: CircleCheck, warning: TriangleAlert, error: CircleAlert };
const toneClass = { ok: "text-success", warning: "text-warning", error: "text-danger" };

/** One card of the system status page, its border and badge by tone. */
export function StatusCard({ title, tone, children }: { title: string; tone: Tone; children: ReactNode }) {
  const { t } = useTranslation("admin");
  const Icon = toneIcon[tone];
  return (
    <section className={cn("flex min-w-0 flex-col gap-2 rounded-lg border bg-surface p-3", tone === "ok" ? "border-border" : tone === "warning" ? "border-warning" : "border-danger")}>
      <h2 className="flex items-center gap-2 font-semibold">
        <Icon aria-hidden className={cn("size-4 shrink-0", toneClass[tone])} />
        <span className="min-w-0 flex-1 truncate">{title}</span>
        <span className={cn("text-meta font-medium", toneClass[tone])}>{t(`status.tone.${tone}`)}</span>
      </h2>
      <div className="flex flex-col gap-1 text-ui text-fg-muted">{children}</div>
    </section>
  );
}

/** A label and its value in a status card. */
export function StatusRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <p className="flex flex-wrap justify-between gap-x-3">
      <span>{label}</span>
      <span className="tabular min-w-0 font-medium break-all text-fg">{value}</span>
    </p>
  );
}
