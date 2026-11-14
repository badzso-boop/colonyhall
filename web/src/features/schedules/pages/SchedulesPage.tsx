// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { CalendarClock, Copy, Plus, XCircle } from "lucide-react";
import { conversationTitle, useChannels } from "@/features/conversation";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { useLocalStorage } from "@/shared/hooks";
import { cn, fullTime, openScheduleBuilder, relativeTime } from "@/shared/lib";
import { Badge, Button, Dialog, EmptyState, IconButton, PageHeader, Skeleton } from "@/shared/ui";
import { useCancelSchedule, useSchedules, type Schedule } from "../api";
import { describeRule, parseCron } from "../rule";

type Filter = "active" | "done" | "canceled";
const tone = { active: "success", done: "neutral", canceled: "danger" } as const;

/** Ütemezések (round 32, part 6): the table, filters, cancel and copy-into-new. */
export default function SchedulesPage() {
  const { t, i18n } = useTranslation("schedules");
  const locale = localeOf(i18n.language);
  const list = useSchedules();
  const channels = useChannels();
  const actors = useActors();
  const session = useSession();
  const cancel = useCancelSchedule();
  const [filter, setFilter] = useLocalStorage<Filter>("colony.schedules.filter", "active");
  const [everyone, setEveryone] = useState(false);
  const [confirm, setConfirm] = useState<Schedule | null>(null);
  const me = session.data?.actor;
  const admin = me?.role === "owner" || me?.role === "admin";
  const now = new Date();

  const rows = useMemo(
    () => (list.data ?? []).filter((s) => s.state === filter && (everyone || s.created_by === me?.id)),
    [list.data, filter, everyone, me?.id],
  );
  const channelTitle = (id: string) => {
    const ch = channels.data?.find((c) => c.id === id);
    return ch && actors.data ? conversationTitle(ch, actors.data, me?.id ?? "") : "…";
  };
  const repeat = (s: Schedule) => {
    // A one-off has no cron.
    if (!s.cron) return t("modes.once");
    const rule = parseCron(s.cron);
    return rule.mode === "advanced" ? <span className="font-mono">{s.cron}</span> : describeRule(rule, t, locale);
  };
  const copy = (s: Schedule) =>
    openScheduleBuilder({ body: s.body, channelId: s.channel_id, threadRootId: s.thread_root_id, cron: s.cron || undefined, timezone: s.timezone, catchUp: s.catch_up });
  const when = (iso?: string) =>
    iso ? (
      <time dateTime={iso} title={`${fullTime(iso, locale)} (${relativeTime(iso, now, locale)})`}>
        {new Date(iso).toLocaleString(locale, { month: "short", day: "numeric", weekday: "short", hour: "2-digit", minute: "2-digit" })}
      </time>
    ) : (
      t("never")
    );

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={<CalendarClock className="size-5" />}
        title={t("title")}
        actions={
          <Button variant="primary" size="sm" icon={<Plus className="size-4" />} onClick={() => openScheduleBuilder({ body: "" })}>
            {t("new")}
          </Button>
        }
      />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <div className="flex flex-wrap items-center gap-2">
          <div role="radiogroup" aria-label={t("filters.label")} className="flex rounded-md border border-border p-0.5">
            {(["active", "done", "canceled"] as const).map((f) => (
              <button
                key={f}
                type="button"
                role="radio"
                aria-checked={filter === f}
                onClick={() => setFilter(f)}
                className={cn("rounded-sm px-2.5 py-1 text-ui", filter === f ? "bg-accent-soft font-semibold text-fg" : "text-fg-muted")}
              >
                {t(`filters.${f}`)}
              </button>
            ))}
          </div>
          {admin ? (
            <button
              type="button"
              aria-pressed={everyone}
              onClick={() => setEveryone(!everyone)}
              className={cn("rounded-md border px-2.5 py-1 text-ui", everyone ? "border-accent bg-accent-soft font-semibold" : "border-border text-fg-muted")}
            >
              {t("filters.everyone")}
            </button>
          ) : null}
          <p className="ml-auto text-meta text-fg-subtle">{t("editHint")}</p>
        </div>
        {list.isPending ? <Skeleton className="h-6 w-2/3" /> : null}
        {list.data && rows.length === 0 ? <EmptyState icon={<CalendarClock className="size-8" />} title={t("empty")} body={t("emptyBody")} /> : null}
        {rows.length ? (
          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full min-w-[56rem] text-left text-ui">
              <thead className="bg-surface-sunken text-meta text-fg-muted">
                <tr>
                  {(["message", "where", "repeat", "next", "last", "runs", "state", "error"] as const).map((c) => (
                    <th key={c} scope="col" className="px-3 py-2 font-semibold">
                      {t(`cols.${c}`)}
                    </th>
                  ))}
                  <th scope="col" className="px-3 py-2">
                    <span className="sr-only">{t("cols.actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {rows.map((s) => {
                  const author = actors.data?.get(s.created_by);
                  return (
                    <tr key={s.id} className="align-top">
                      <td className="max-w-64 px-3 py-2">
                        <span className="block truncate font-medium">{s.body.split("\n")[0]}</span>
                        {everyone && author ? <span className="text-meta text-fg-subtle">{t("by", { name: author.display_name || author.handle })}</span> : null}
                      </td>
                      <td className="px-3 py-2">
                        <Link to={`/c/${s.channel_id}${s.thread_root_id ? `?thread=${s.thread_root_id}` : ""}`} className="hover:underline">
                          {channelTitle(s.channel_id)}
                        </Link>
                      </td>
                      <td className="px-3 py-2">
                        {repeat(s)}
                        <span className="block text-meta text-fg-subtle">{s.timezone}</span>
                      </td>
                      <td className="px-3 py-2 whitespace-nowrap">{s.state === "active" ? when(s.next_run_at) : t("never")}</td>
                      <td className="px-3 py-2 whitespace-nowrap">
                        {s.last_run_at && s.last_message_id ? (
                          <Link to={`/c/${s.channel_id}${s.thread_root_id ? `?thread=${s.thread_root_id}` : ""}`} title={t("openLast")} className="hover:underline">
                            {when(s.last_run_at)}
                          </Link>
                        ) : (
                          when(s.last_run_at)
                        )}
                      </td>
                      <td className="px-3 py-2 tabular-nums">{s.run_count}</td>
                      <td className="px-3 py-2">
                        <Badge tone={tone[s.state]}>{t(`state.${s.state}`)}</Badge>
                      </td>
                      <td className="max-w-48 px-3 py-2">
                        {s.last_error ? (
                          <span className="line-clamp-2 text-meta text-danger" title={s.last_error}>
                            {s.last_error}
                          </span>
                        ) : null}
                      </td>
                      <td className="px-2 py-1.5 whitespace-nowrap">
                        <IconButton size="sm" label={t("copy")} icon={<Copy className="size-3.5" />} onClick={() => copy(s)} />
                        {s.state === "active" ? <IconButton size="sm" label={t("cancelSchedule")} icon={<XCircle className="size-3.5" />} onClick={() => setConfirm(s)} /> : null}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : null}
      </div>
      <Dialog
        open={confirm !== null}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={t("cancelTitle")}
        description={confirm ? t("cancelBody", { body: confirm.body.split("\n")[0] }) : undefined}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirm(null)}>{t("keep")}</Button>
            <Button variant="danger" loading={cancel.isPending} onClick={() => confirm && cancel.mutate(confirm.id, { onSuccess: () => setConfirm(null) })}>
              {t("cancelSchedule")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </div>
  );
}
