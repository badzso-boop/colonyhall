// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { useNow } from "@/shared/hooks";
import { localeOf } from "@/shared/i18n";
import { cn } from "@/shared/lib";
import { useUsageWeek } from "../api";
import { budapestDay, lastSevenDays, shortTokens, type AgentOverview } from "../model";
import { Section } from "./Section";

type Day = { day: string; tokens: number; runs: number };

/** A seven-day column sparkline: one series, so no legend; each bar has its value as a tooltip. */
function WeekBars({ days, label, dayLabel }: { days: Day[]; label: string; dayLabel: (d: Day) => string }) {
  const max = Math.max(1, ...days.map((d) => d.tokens));
  return (
    <figure className="flex flex-col gap-1">
      <div role="img" aria-label={label} className="flex h-10 items-end gap-0.5">
        {days.map((d) => (
          <span key={d.day} title={dayLabel(d)} className="flex h-full flex-1 items-end">
            <span className="w-full rounded-t-sm bg-info" style={{ height: `${d.tokens > 0 ? Math.max(6, (d.tokens / max) * 100) : 0}%` }} />
          </span>
        ))}
      </div>
      <ul className="sr-only">
        {days.map((d) => (
          <li key={d.day}>{dayLabel(d)}</li>
        ))}
      </ul>
      <div aria-hidden className="h-px bg-border" />
    </figure>
  );
}

/** 3. Felhasználás: today and the last seven days, the daily limit bar and the week's bars. */
export function UsageSection({ overview, human }: { overview: AgentOverview; human: boolean }) {
  const { t, i18n } = useTranslation("agents");
  const locale = localeOf(i18n.language);
  const now = useNow();
  const week = useUsageWeek(human);
  const usage = overview.usage;
  const days = week.data ? lastSevenDays(week.data, overview.actor.id, budapestDay(now)) : null;
  const today = days?.[days.length - 1];
  const todayTokens = usage?.today_tokens ?? today?.tokens ?? 0;
  const limit = usage?.daily_token_limit ?? 0;
  const pct = limit > 0 ? Math.min(100, (todayTokens / limit) * 100) : 0;
  const num = (n: number) => n.toLocaleString(locale);
  const weekday = (day: string) => new Date(`${day}T12:00:00Z`).toLocaleDateString(locale, { weekday: "short", month: "numeric", day: "numeric" });
  return (
    <Section id="usage" title={t("overview.usage.title")}>
      <dl className="grid grid-cols-2 gap-3">
        <div>
          <dt className="text-meta text-fg-subtle">{t("overview.usage.today")}</dt>
          <dd className="tabular font-semibold">{t("overview.usage.tokens", { tokens: num(todayTokens) })}</dd>
          {today ? <dd className="tabular text-meta text-fg-muted">{t("overview.usage.runs", { count: today.runs })}</dd> : null}
        </div>
        <div>
          <dt className="text-meta text-fg-subtle">{t("overview.usage.week")}</dt>
          {days ? (
            <>
              <dd className="tabular font-semibold">{t("overview.usage.tokens", { tokens: num(days.reduce((s, d) => s + d.tokens, 0)) })}</dd>
              <dd className="tabular text-meta text-fg-muted">{t("overview.usage.runs", { count: days.reduce((s, d) => s + d.runs, 0) })}</dd>
            </>
          ) : (
            <dd className="text-meta text-fg-muted">{t("overview.usage.noWeek")}</dd>
          )}
        </div>
      </dl>
      {limit > 0 ? (
        <div className="mt-3">
          <p className={cn("tabular text-meta", pct >= 80 ? "text-warning" : "text-fg-muted")}>{t("overview.usage.limit", { used: shortTokens(todayTokens), limit: shortTokens(limit) })}</p>
          <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-surface-sunken" role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} aria-label={t("overview.usage.today")}>
            <div className={cn("h-full rounded-full", pct >= 80 ? "bg-warning" : "bg-fg-muted")} style={{ width: `${pct}%` }} />
          </div>
        </div>
      ) : (
        <p className="mt-3 text-meta text-fg-subtle">{t("overview.usage.noLimit")}</p>
      )}
      {days ? (
        <div className="mt-3">
          <WeekBars days={days} label={t("overview.usage.chart")} dayLabel={(d) => t("overview.usage.day", { day: weekday(d.day), tokens: num(d.tokens), runs: d.runs })} />
        </div>
      ) : null}
    </Section>
  );
}
