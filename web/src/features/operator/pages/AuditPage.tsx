// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { ScrollText } from "lucide-react";
import { fullTime } from "@/shared/lib";
import { Button, EmptyState, PageHeader, Skeleton } from "@/shared/ui";
import { useCustomers, useOperatorAudit } from "../api";
import { slugsById } from "../model";

/** Napló: every operator action, newest first; never a password, code or token. */
export default function AuditPage() {
  const { t, i18n } = useTranslation("operator");
  const audit = useOperatorAudit();
  const customers = useCustomers();
  const slugs = slugsById(customers.data ?? []);
  const entries = audit.data?.pages.flat() ?? [];
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader icon={<ScrollText className="size-5" />} title={t("audit.title")} />
      {audit.isPending ? (
        <Skeleton className="m-4 h-32" />
      ) : audit.isError ? (
        <p className="p-4 text-danger">{audit.error.message}</p>
      ) : !entries.length ? (
        <EmptyState title={t("audit.empty")} />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto p-3 sm:p-4">
          <ol className="flex flex-col divide-y divide-border/60 rounded-lg border border-border bg-surface">
            {entries.map((e) => (
              <li key={e.id} className="flex flex-col gap-0.5 px-3 py-2 text-ui">
                <span className="flex flex-wrap items-baseline gap-x-2">
                  <span className="font-mono font-medium">{e.action}</span>
                  {e.workspace_id ? <span className="font-mono text-meta text-fg-muted">{slugs.get(e.workspace_id) ?? e.workspace_id}</span> : null}
                </span>
                <span className="text-meta text-fg-subtle">
                  {fullTime(e.created_at, i18n.language)}
                  {e.remote_addr ? ` · ${e.remote_addr}` : ""}
                  {e.operator_id ? "" : ` · ${t("audit.cli")}`}
                </span>
                {Object.keys(e.payload).length ? (
                  <code className="font-mono text-meta break-all text-fg-muted">{JSON.stringify(e.payload)}</code>
                ) : null}
              </li>
            ))}
          </ol>
          {audit.hasNextPage ? (
            <div className="mt-3 flex justify-center">
              <Button size="sm" loading={audit.isFetchingNextPage} onClick={() => void audit.fetchNextPage()}>
                {t("audit.more")}
              </Button>
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}
