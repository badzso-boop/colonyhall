// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { useNow } from "@/shared/hooks";
import { cn, dayKey } from "@/shared/lib";
import { IconButton } from "@/shared/ui";
import { useCalendars, useEvents } from "../api";
import { addDays, eventsByDay, monthGrid, rangeTitle, shift, startOfDay, viewRange } from "../calendar";
import { Agenda, useHiddenCalendars } from "./CalendarParts";

const AgendaDays = 8;

/** The floating window: a mini month on top, today and the next seven days below. */
export function CalendarWindow() {
  const { t, i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  const now = useNow();
  const today = startOfDay(now);
  const [month, setMonth] = useState(() => new Date(today.getFullYear(), today.getMonth(), 1));
  const grid = useMemo(() => monthGrid(month), [month]);
  const monthRange = useMemo(() => viewRange("month", month), [month]);
  const todayKey = dayKey(today);
  const agendaDays = useMemo(() => Array.from({ length: AgendaDays }, (_, i) => addDays(new Date(`${todayKey}T00:00`), i)), [todayKey]);
  const calendars = useCalendars();
  const has = (calendars.data?.length ?? 0) > 0;
  const monthEvents = useEvents(monthRange.from, monthRange.to, has);
  const agendaEvents = useEvents(agendaDays[0]!, addDays(agendaDays[0]!, AgendaDays), has);
  const { hidden } = useHiddenCalendars();
  const calMap = useMemo(() => new Map((calendars.data ?? []).map((c) => [c.id, c])), [calendars.data]);
  const visible = (list: typeof monthEvents.data) => (list ?? []).filter((e) => !hidden.has(e.calendar_id));
  const monthByDay = useMemo(() => eventsByDay(visible(monthEvents.data)), [monthEvents.data, hidden]); // eslint-disable-line react-hooks/exhaustive-deps
  const agendaByDay = useMemo(() => eventsByDay(visible(agendaEvents.data)), [agendaEvents.data, hidden]); // eslint-disable-line react-hooks/exhaustive-deps

  const jump = (d: Date) => document.getElementById(`agenda-${dayKey(d)}`)?.scrollIntoView({ block: "start", behavior: "smooth" });
  return (
    <div className="flex flex-col gap-3 p-2.5 text-ui">
      <div>
        <div className="mb-1 flex items-center gap-1">
          <p className="flex-1 font-semibold first-letter:uppercase">{rangeTitle("month", month, locale)}</p>
          <IconButton size="sm" label={t("calendar.prev")} icon={<ChevronLeft className="size-3.5" />} onClick={() => setMonth((m) => shift("month", m, -1))} />
          <IconButton size="sm" label={t("calendar.next")} icon={<ChevronRight className="size-3.5" />} onClick={() => setMonth((m) => shift("month", m, 1))} />
        </div>
        <div className="grid grid-cols-7 text-center text-meta">
          {grid.slice(0, 7).map((d) => (
            <span key={d.getDay()} className="text-fg-subtle uppercase">
              {d.toLocaleDateString(locale, { weekday: "narrow" })}
            </span>
          ))}
          {grid.map((d) => {
            const key = dayKey(d);
            return (
              <button
                key={key}
                type="button"
                aria-label={d.toLocaleDateString(locale, { month: "long", day: "numeric" })}
                onClick={() => jump(d)}
                className={cn(
                  "relative mx-auto grid size-6 place-items-center rounded-full tabular-nums hover:bg-surface-sunken",
                  d.getMonth() !== month.getMonth() && "text-fg-subtle",
                  key === todayKey && "bg-accent font-semibold text-accent-fg hover:bg-accent",
                )}
              >
                {d.getDate()}
                {monthByDay.has(key) && key !== todayKey ? <span aria-hidden className="absolute bottom-0 size-1 rounded-full bg-accent" /> : null}
              </button>
            );
          })}
        </div>
      </div>
      <div className="border-t border-border pt-2">
        {has ? <Agenda days={agendaDays} byDay={agendaByDay} calendars={calMap} now={now} /> : <p className="text-meta text-fg-subtle">{t("calendar.readOnly")}</p>}
      </div>
    </div>
  );
}
