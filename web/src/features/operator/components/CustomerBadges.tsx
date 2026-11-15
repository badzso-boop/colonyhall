// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { formatBytes } from "@/shared/lib";
import { Badge } from "@/shared/ui";
import { inviteStatus, storagePercent, type Customer } from "../model";

const inviteTone = { owner: "success", none: "neutral", open: "info", expired: "warning", used: "neutral", revoked: "neutral" } as const;
const storageTone = { ok: "neutral", warning_80: "warning", warning_95: "warning", full: "danger" } as const;

/** Active or suspended. */
export function StateBadge({ state }: { state: Customer["state"] }) {
  const { t } = useTranslation("operator");
  return <Badge tone={state === "active" ? "success" : "danger"} className="whitespace-nowrap">{t(`state.${state}`)}</Badge>;
}

/** The owner, or where the owner's invite stands. */
export function OwnerBadge({ customer, now }: { customer: Customer; now: Date }) {
  const { t } = useTranslation("operator");
  const s = inviteStatus(customer, now);
  return <Badge tone={inviteTone[s]} className="whitespace-nowrap">{t(`invite.${s}`)}</Badge>;
}

/** Used of the quota (round 54), with the warning level as a word, not only a colour. */
export function StorageText({ storage }: { storage: Customer["storage"] }) {
  const { t, i18n } = useTranslation("operator");
  const locale = localeOf(i18n.language);
  const pct = storagePercent(storage);
  const used = formatBytes(storage.used_bytes, locale);
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      <span className="tabular">
        {pct === null ? t("storage.unlimited", { used }) : t("storage.ofQuota", { used, quota: formatBytes(storage.quota_bytes, locale), pct })}
      </span>
      {storage.state !== "ok" ? <Badge tone={storageTone[storage.state]}>{t(`storage.${storage.state}`)}</Badge> : null}
    </span>
  );
}
