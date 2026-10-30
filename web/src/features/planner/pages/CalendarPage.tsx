import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { CalendarDays, ChevronLeft, ChevronRight } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { useLocalStorage, useNow, useWidth } from "@/shared/hooks";
import { cn, dayKey, useShortcut } from "@/shared/lib";
import { Button, EmptyState, IconButton, PageHeader } from "@/shared/ui";
import { openWindow } from "@/shared/windows";
import { useCalendars, useEvents } from "../api";
import { eventsByDay, rangeTitle, shift, startOfDay, viewDays, viewRange, type CalView } from "../calendar";
import { Agenda, useHiddenCalendars } from "../components/CalendarParts";
import { CalendarsPopover } from "../components/CalendarsPopover";
import { MonthView, TimeGrid } from "../components/CalendarViews";

type Saved = "month" | "week" | "day";

/** Naptár (docs/handoff/32, part 4): month, week and day over the subscribed calendars; read-only. */
export default function CalendarPage() {
  const { t, i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  const narrow = useWidth() === "narrow";
  const now = useNow();
  const [saved, setSaved] = useLocalStorage<Saved>("colony.calendar.view", "month");
  // A phone shows three days instead of a week.
  const view: CalView = saved === "week" && narrow ? "three" : saved;
  const [anchor, setAnchor] = useState(() => startOfDay(new Date()));
  const [selected, setSelected] = useState(() => dayKey(new Date()));
  const days = useMemo(() => viewDays(view, anchor), [view, anchor]);
  const range = useMemo(() => viewRange(view, anchor), [view, anchor]);
  const calendars = useCalendars();
  const events = useEvents(range.from, range.to, (calendars.data?.length ?? 0) > 0);
  const { hidden, toggle } = useHiddenCalendars();
  const calMap = useMemo(() => new Map((calendars.data ?? []).map((c) => [c.id, c])), [calendars.data]);
  const byDay = useMemo(() => eventsByDay((events.data ?? []).filter((e) => !hidden.has(e.calendar_id))), [events.data, hidden]);

  const go = (dir: -1 | 1) => setAnchor((a) => shift(view, a, dir));
  const today = () => {
    setAnchor(startOfDay(new Date()));
    setSelected(dayKey(new Date()));
  };
  const openDay = (d: Date) => {
    setAnchor(startOfDay(d));
    setSaved("day");
  };
  useShortcut("m", "page", "calMonth", () => setSaved("month"));
  useShortcut("w", "page", "calWeek", () => setSaved("week"));
  useShortcut("d", "page", "calDay", () => setSaved("day"));
  useShortcut("arrowleft", "page", "calPrev", () => go(-1));
  useShortcut("arrowright", "page", "calNext", () => go(1));
  useShortcut("t", "page", "calToday", today);

  const selectedDay = days.find((d) => dayKey(d) === selected);
  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={<CalendarDays className="size-5" />}
        title={t("calendar.title")}
        onPopOut={() => openWindow("calendar")}
        popOutLabel={t("calendar.popOut")}
        actions={
          <>
            <Button size="sm" onClick={today}>
              {t("calendar.today")}
            </Button>
            <IconButton size="sm" label={t("calendar.prev")} icon={<ChevronLeft className="size-4" />} onClick={() => go(-1)} />
            <IconButton size="sm" label={t("calendar.next")} icon={<ChevronRight className="size-4" />} onClick={() => go(1)} />
            <div role="radiogroup" aria-label={t("calendar.view")} className="flex rounded-md border border-border p-0.5">
              {(["month", "week", "day"] as const).map((v) => (
                <button
                  key={v}
                  type="button"
                  role="radio"
                  aria-checked={saved === v}
                  onClick={() => setSaved(v)}
                  className={cn("rounded-sm px-2.5 py-1 text-ui", saved === v ? "bg-accent-soft font-semibold text-fg" : "text-fg-muted")}
                >
                  {t(`calendar.${v === "week" && narrow ? "three" : v}`)}
                </button>
              ))}
            </div>
            {calendars.data?.length ? <CalendarsPopover calendars={calendars.data} hidden={hidden} onToggle={toggle} /> : null}
          </>
        }
      />
      <h2 aria-live="polite" className="px-4 pt-3 text-title font-semibold first-letter:uppercase">
        {rangeTitle(view, anchor, locale)}
      </h2>
      {calendars.data && calendars.data.length === 0 ? (
        <div className="mx-auto flex w-full max-w-md flex-col gap-4 p-6">
          <EmptyState icon={<CalendarDays className="size-8" />} title={t("calendar.emptyTitle")} body={t("calendar.readOnly")} />
          <div className="self-center">
            <CalendarsPopover calendars={[]} hidden={hidden} onToggle={toggle} />
          </div>
        </div>
      ) : view === "month" ? (
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-2 md:p-4">
          <MonthView
            days={days}
            byDay={byDay}
            calendars={calMap}
            now={now}
            month={anchor.getMonth()}
            dots={narrow}
            selected={selected}
            onDay={narrow ? (d) => setSelected(dayKey(d)) : openDay}
          />
          {narrow && selectedDay ? (
            <div className="pt-3">
              <Agenda days={[selectedDay]} byDay={byDay} calendars={calMap} now={now} />
            </div>
          ) : null}
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col p-2 md:p-4">
          <TimeGrid days={days} byDay={byDay} calendars={calMap} now={now} wide={view === "day"} />
        </div>
      )}
      {calendars.data?.length ? <p className="px-4 pb-2 text-meta text-fg-subtle">{t("calendar.readOnly")}</p> : null}
    </div>
  );
}
