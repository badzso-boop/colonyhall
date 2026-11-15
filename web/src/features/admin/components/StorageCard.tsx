// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useSession } from "@/features/identity";
import { useSetQuota, useWorkspaceStorage, type StorageState } from "@/features/files";
import { localeOf } from "@/shared/i18n";
import { cn, formatBytes, fullTime } from "@/shared/lib";
import { Button, Input, Skeleton } from "@/shared/ui";
import { gbToBytes, storagePercent } from "../model";
import type { Tone } from "../model";
import { StatusCard, StatusRow } from "./StatusCard";

const tones: Record<StorageState, Tone> = { ok: "ok", warning_80: "warning", warning_95: "warning", full: "error" };

/**
 * Tárhely (round 54): the workspace's usage (attachments and its share of
 * the database), the quota and the state. Owner and admins see it, only the
 * owner sets the quota. Never per person.
 */
export function StorageCard() {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const role = useSession().data?.actor.role;
  const owner = role === "owner";
  const storage = useWorkspaceStorage(owner || role === "admin");
  const st = storage.data;
  const bytes = (n: number) => formatBytes(n, locale);
  if (!st?.state) {
    return (
      <StatusCard title={t("status.storage.title")} tone="ok">
        <Skeleton className="h-4 w-2/3" />
      </StatusCard>
    );
  }
  // Round 51b: the effective quota (the plan's, or the owner's when lower).
  const limit = st.limit_bytes;
  const pct = storagePercent(st.used_bytes, limit);
  return (
    <StatusCard title={t("status.storage.title")} tone={tones[st.state]}>
      <p className={cn("font-medium", st.state === "ok" ? "text-fg" : st.state === "full" ? "text-danger" : "text-warning")}>{t(`status.storage.state.${st.state}`)}</p>
      {pct !== null ? (
        <div
          role="meter"
          aria-label={t("status.storage.title")}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(pct)}
          className="h-2 overflow-hidden rounded-full bg-surface-sunken"
        >
          <div className={cn("h-full rounded-full", st.state === "full" ? "bg-danger" : st.state === "ok" ? "bg-accent" : "bg-warning")} style={{ width: `${Math.min(100, pct)}%` }} />
        </div>
      ) : null}
      <StatusRow
        label={t("status.storage.used")}
        value={limit ? t("status.storage.ofQuota", { used: bytes(st.used_bytes), quota: bytes(limit), pct: Math.round(pct ?? 0) }) : bytes(st.used_bytes)}
      />
      {st.plan ? (
        <StatusRow
          label={t("status.storage.plan")}
          value={t("status.storage.planValue", { name: st.plan.name, storage: bytes(st.plan.storage_bytes), cpu: st.plan.cpu_cores, memory: bytes(st.plan.memory_bytes) })}
        />
      ) : null}
      <StatusRow label={t("status.storage.attachments")} value={bytes(st.attachments_bytes)} />
      <StatusRow label={t("status.storage.chatSearch")} value={bytes(st.chat_search_bytes)} />
      <StatusRow label={t("status.storage.other")} value={bytes(st.other_bytes)} />
      <StatusRow label={t("status.storage.measured")} value={st.db_measured_at ? fullTime(st.db_measured_at, locale) : t("status.storage.notYet")} />
      {owner ? <QuotaForm quota={st.quota_bytes} withPlan={!!st.plan} /> : <StatusRow label={t("status.storage.quota")} value={st.quota_bytes ? bytes(st.quota_bytes) : t("status.storage.unlimited")} />}
    </StatusCard>
  );
}

function QuotaForm({ quota, withPlan }: { quota: number; withPlan: boolean }) {
  const { t } = useTranslation("admin");
  const set = useSetQuota();
  const [value, setValue] = useState(quota ? String(+(quota / 1024 ** 3).toFixed(2)) : "");
  const parsed = gbToBytes(value);
  const save = (e: FormEvent) => {
    e.preventDefault();
    if (parsed !== null) set.mutate(parsed);
  };
  return (
    <form onSubmit={save} className="mt-1 flex flex-wrap items-end gap-2">
      <label className="flex flex-col gap-1">
        <span className="text-meta font-semibold">{t("status.storage.quotaGb")}</span>
        <Input inputMode="decimal" value={value} onChange={(e) => setValue(e.target.value)} placeholder={t("status.storage.unlimited")} className="w-32" aria-invalid={parsed === null} />
      </label>
      <Button type="submit" size="sm" variant="secondary" disabled={parsed === null} loading={set.isPending}>
        {t("status.storage.save")}
      </Button>
      <span className="w-full text-meta text-fg-subtle">{t("status.storage.quotaHint")}</span>
      {withPlan ? <span className="w-full text-meta text-fg-subtle">{t("status.storage.belowPlan")}</span> : null}
      {set.error ? <span className="w-full text-meta text-danger">{set.error.message}</span> : null}
    </form>
  );
}
