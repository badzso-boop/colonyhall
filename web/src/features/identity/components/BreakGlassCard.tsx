// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { fullTime } from "@/shared/lib";
import { Badge, Button, EmptyState, Skeleton, toast } from "@/shared/ui";
import { useBreakGlassList, useCloseBreakGlass, useSession, type BreakGlass } from "../api";

/**
 * The operator's break-glass accesses to the workspace (round 51b): who,
 * why, until when; the owner may close an open one at once. Every read of
 * an access is in the audit log.
 */
export function BreakGlassCard() {
  const { t } = useTranslation("identity");
  const role = useSession().data?.actor.role;
  const list = useBreakGlassList(role === "owner" || role === "admin");
  if (list.isPending) return <Skeleton className="h-20" />;
  if (list.isError) return <p className="text-ui text-danger">{list.error.message}</p>;
  if (!list.data.length) return <EmptyState title={t("breakGlass.none")} body={t("breakGlass.noneBody")} />;
  return (
    <ul className="flex flex-col gap-2">
      {list.data.map((b) => (
        <Entry key={b.id} access={b} owner={role === "owner"} />
      ))}
    </ul>
  );
}

function Entry({ access: b, owner }: { access: BreakGlass; owner: boolean }) {
  const { t, i18n } = useTranslation("identity");
  const locale = localeOf(i18n.language);
  const close = useCloseBreakGlass();
  return (
    <li className="flex flex-col gap-1 rounded-lg border border-border bg-surface p-3 text-ui">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold">{b.operator_handle}</span>
        {b.open ? <Badge tone="warning">{t("breakGlass.open")}</Badge> : <Badge>{t(`breakGlass.closedBy.${b.closed_by ?? "expired"}`)}</Badge>}
        {b.dev_mode ? <Badge tone="danger">{t("breakGlass.devMode")}</Badge> : null}
        {b.open && owner ? (
          <Button
            size="sm"
            variant="danger"
            className="ml-auto"
            loading={close.isPending}
            onClick={() => close.mutate(b.id, { onError: (e) => toast(e.message, { tone: "danger" }) })}
          >
            {t("breakGlass.close")}
          </Button>
        ) : null}
      </div>
      <p className="break-words text-fg">{b.reason || t("breakGlass.noReason")}</p>
      <p className="tabular text-meta text-fg-muted">
        {t("breakGlass.when", { opened: fullTime(b.opened_at, locale), until: fullTime(b.closed_at ?? b.expires_at, locale) })}
      </p>
    </li>
  );
}
