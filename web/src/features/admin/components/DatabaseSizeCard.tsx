// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { formatBytes, sizeGrowth, type SystemStatus } from "../model";
import { Sparkline } from "./charts";
import { StatusCard, StatusRow as Row } from "./StatusCard";

/**
 * Adatbázis mérete (round 50): the total, the ten largest tables and the
 * growth over the last 30 days. The memory store has no database.
 */
export function DatabaseSizeCard({ database }: { database: SystemStatus["database"] }) {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const bytes = (n: number) => formatBytes(n, locale);
  if (database.size_bytes === undefined) {
    return (
      <StatusCard title={t("status.size.title")} tone="ok">
        <p>{t("status.size.noDatabase")}</p>
      </StatusCard>
    );
  }
  const growth = sizeGrowth(database.growth ?? []);
  const tables = database.tables ?? [];
  return (
    <StatusCard title={t("status.size.title")} tone="ok">
      <Row label={t("status.size.total")} value={bytes(database.size_bytes)} />
      <div className="flex flex-wrap items-center justify-between gap-x-3">
        <span>{t("status.size.growth")}</span>
        {growth ? (
          <span className="flex items-center gap-2">
            <span className="tabular font-medium text-fg">{t("status.size.growthText", { delta: `${growth.delta >= 0 ? "+" : ""}${bytes(growth.delta)}`, count: growth.days })}</span>
            <Sparkline values={growth.recent.map((x) => x.size_bytes)} label={t("status.size.growth")} format={bytes} fromMin />
          </span>
        ) : (
          <span className="text-fg-subtle">{t("status.size.noGrowth")}</span>
        )}
      </div>
      {tables.length ? (
        <table className="mt-1 w-full table-fixed text-meta">
          <caption className="sr-only">{t("status.size.tables")}</caption>
          <thead>
            <tr className="text-left text-fg-subtle">
              <th scope="col" className="w-1/2 font-medium">{t("status.size.table")}</th>
              <th scope="col" className="text-right font-medium">{t("status.size.tableSize")}</th>
              <th scope="col" className="text-right font-medium">{t("status.size.rows")}</th>
            </tr>
          </thead>
          <tbody>
            {tables.map((x) => (
              <tr key={x.name}>
                <td className="truncate font-mono text-fg">{x.name}</td>
                <td className="tabular text-right text-fg">{bytes(x.total_bytes)}</td>
                <td className="tabular text-right">~{x.rows_estimate.toLocaleString(locale)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </StatusCard>
  );
}
