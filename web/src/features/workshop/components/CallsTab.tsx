// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useActors } from "@/features/identity";
import { formatBytes, fullTime } from "@/shared/lib";
import { Badge, EmptyState, Select, Skeleton } from "@/shared/ui";
import { useConnectors, useToolCalls } from "../api";

const outcomeTone = {
  ok: "success",
  error: "danger",
  denied: "warning",
  timeout: "danger",
  limit: "warning",
} as const;

/** The call log: who called what, when, how it ended; never the content. */
export function CallsTab() {
  const { t, i18n } = useTranslation("workshop");
  const [target, setTarget] = useState("");
  const calls = useToolCalls(target);
  const connectors = useConnectors();
  const actors = useActors();
  const names = new Map(
    (connectors.data?.connectors ?? []).map((c) => [c.id, c.slug]),
  );
  return (
    <div className="flex flex-col gap-3">
      <p className="text-ui text-fg-muted">{t("calls.intro")}</p>
      <Select
        aria-label={t("grants.target")}
        value={target}
        onChange={(e) => setTarget(e.target.value)}
        className="max-w-xs"
      >
        <option value="">{t("calls.all")}</option>
        {(connectors.data?.connectors ?? []).map((c) => (
          <option key={c.id} value={c.id}>
            {c.title}
          </option>
        ))}
      </Select>
      {calls.isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : calls.isError ? (
        <p className="text-ui text-danger">{calls.error.message}</p>
      ) : calls.data.length === 0 ? (
        <EmptyState title={t("calls.empty")} />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border bg-surface">
          <table className="w-full text-left text-ui">
            <thead className="text-meta text-fg-muted">
              <tr className="border-b border-border">
                <th className="px-3 py-2 font-medium">{t("calls.when")}</th>
                <th className="px-3 py-2 font-medium">{t("calls.who")}</th>
                <th className="px-3 py-2 font-medium">{t("calls.tool")}</th>
                <th className="px-3 py-2 font-medium">{t("calls.outcome")}</th>
                <th className="px-3 py-2 text-right font-medium">
                  {t("calls.duration")}
                </th>
                <th className="px-3 py-2 text-right font-medium">
                  {t("calls.size")}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {calls.data.map((c) => (
                <tr key={c.id}>
                  <td className="px-3 py-1.5 whitespace-nowrap text-meta">
                    {fullTime(c.started_at, i18n.language)}
                  </td>
                  <td className="px-3 py-1.5">
                    @{actors.data?.get(c.actor_id)?.handle ?? "?"}
                  </td>
                  <td className="px-3 py-1.5">
                    <code className="font-mono text-meta">
                      {names.get(c.target_id) ?? "?"}__{c.tool}
                    </code>
                    <span className="ml-2 text-meta text-fg-subtle">
                      {t(`connectors.risk.${c.risk}`)}
                    </span>
                  </td>
                  <td className="px-3 py-1.5">
                    <Badge tone={outcomeTone[c.outcome]}>
                      {t(`calls.outcomes.${c.outcome}`)}
                    </Badge>
                    {c.approval_id ? (
                      <span className="ml-2 text-meta text-fg-subtle">
                        {t("calls.approved")}
                      </span>
                    ) : null}
                    {c.error ? (
                      <span className="block max-w-72 truncate text-meta text-fg-muted">
                        {c.error}
                      </span>
                    ) : null}
                  </td>
                  <td className="px-3 py-1.5 text-right text-meta">
                    {c.duration_ms} ms
                  </td>
                  <td className="px-3 py-1.5 text-right text-meta">
                    {formatBytes(c.out_bytes, i18n.language)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
