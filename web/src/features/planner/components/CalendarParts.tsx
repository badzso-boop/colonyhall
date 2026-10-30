import { useState, type CSSProperties, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { MapPin } from "lucide-react";
import type { Schemas } from "@/shared/api";
import { useLocalStorage, useWidth } from "@/shared/hooks";
import { cn, dayKey, shortTime } from "@/shared/lib";
import { Markdown, Popover, PopoverContent, PopoverTrigger, Sheet } from "@/shared/ui";
import { addDays, type CalEvent } from "../calendar";

export type Calendar = Schemas["PlannerCalendar"];

/** The calendars hidden in this browser (the visibility toggles of the calendars popover). */
export function useHiddenCalendars() {
  const [hidden, setHidden] = useLocalStorage<string[]>("colony.calendar.hidden", []);
  const toggle = (id: string) => setHidden(hidden.includes(id) ? hidden.filter((x) => x !== id) : [...hidden, id]);
  return { hidden: new Set(hidden), toggle };
}

export function Dot({ colour, className }: { colour?: string; className?: string }) {
  return <span aria-hidden className={cn("inline-block size-2 shrink-0 rounded-full", className)} style={{ background: colour || "var(--color-fg-subtle)" }} />;
}

/** "09:30–10:30", "egész nap", or the days of a multi-day all-day event. */
export function timeLabel(e: CalEvent, locale: string, allDay: string): string {
  if (e.all_day) {
    const start = new Date(e.start);
    const last = addDays(new Date(e.end), -1);
    if (dayKey(last) <= dayKey(start)) return allDay;
    const f = (d: Date) => d.toLocaleDateString(locale, { month: "short", day: "numeric" });
    return `${f(start)} – ${f(last)}`;
  }
  return `${shortTime(e.start, locale)}–${shortTime(e.end, locale)}`;
}

function EventDetail({ event, calendar }: { event: CalEvent; calendar?: Calendar }) {
  const { t, i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  const date = new Date(event.start).toLocaleDateString(locale, { weekday: "long", month: "long", day: "numeric" });
  return (
    <div className="flex flex-col gap-2">
      <p className="text-meta text-fg-muted">
        {date} · {timeLabel(event, locale, t("calendar.allDay"))}
      </p>
      {calendar ? (
        <p className="flex items-center gap-1.5 text-meta text-fg-muted">
          <Dot colour={calendar.colour} />
          {calendar.name}
        </p>
      ) : null}
      {event.location ? (
        <p className="flex items-start gap-1.5 text-ui">
          <MapPin aria-hidden className="mt-0.5 size-3.5 shrink-0 text-fg-subtle" />
          <span className="sr-only">{t("calendar.location")}: </span>
          {event.location}
        </p>
      ) : null}
      {event.description ? <Markdown text={event.description} className="max-h-64 overflow-y-auto text-ui" copyLabel={t("tasks.copy")} copiedLabel={t("tasks.copied")} /> : null}
    </div>
  );
}

/** An event that opens its detail: a popover on desktop, a sheet on a phone. */
export function EventButton({ event, calendar, className, style, children }: { event: CalEvent; calendar?: Calendar; className?: string; style?: CSSProperties; children: ReactNode }) {
  const { t } = useTranslation("planner");
  const narrow = useWidth() === "narrow";
  const [open, setOpen] = useState(false);
  const button = (
    <button type="button" title={event.title} className={cn("text-left", className)} style={style} onClick={narrow ? () => setOpen(true) : undefined}>
      {children}
    </button>
  );
  if (narrow)
    return (
      <>
        {button}
        <Sheet open={open} onOpenChange={setOpen} title={event.title} closeLabel={t("calendar.close")}>
          <EventDetail event={event} calendar={calendar} />
        </Sheet>
      </>
    );
  return (
    <Popover>
      <PopoverTrigger asChild>{button}</PopoverTrigger>
      <PopoverContent label={event.title}>
        <p className="mb-1 font-semibold">{event.title}</p>
        <EventDetail event={event} calendar={calendar} />
      </PopoverContent>
    </Popover>
  );
}

/** One line of an event: the colour dot, the time (or nothing for all-day) and the title. */
export function EventLine({ event, calendar, compact }: { event: CalEvent; calendar?: Calendar; compact?: boolean }) {
  const { i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  if (event.all_day)
    return (
      <EventButton
        event={event}
        calendar={calendar}
        className="block w-full truncate rounded-sm px-1.5 text-meta font-medium text-white"
        style={{ background: calendar?.colour || "var(--color-fg-subtle)" }}
      >
        {event.title}
      </EventButton>
    );
  return (
    <EventButton event={event} calendar={calendar} className="flex w-full items-center gap-1 truncate rounded-sm px-1 text-meta hover:bg-surface-sunken">
      <Dot colour={calendar?.colour} />
      {compact ? null : <span className="shrink-0 text-fg-subtle tabular-nums">{shortTime(event.start, locale)}</span>}
      <span className="truncate">{event.title}</span>
    </EventButton>
  );
}

/** Days with their events as a list (the floating window, the phone's month view). */
export function Agenda({ days, byDay, calendars, now }: { days: Date[]; byDay: Map<string, CalEvent[]>; calendars: Map<string, Calendar>; now: Date }) {
  const { t, i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  return (
    <div className="flex flex-col gap-3">
      {days.map((d) => {
        const key = dayKey(d);
        const list = byDay.get(key) ?? [];
        return (
          <section key={key} id={`agenda-${key}`} aria-label={d.toLocaleDateString(locale, { weekday: "long", month: "long", day: "numeric" })}>
            <h3 className={cn("mb-1 text-meta font-semibold text-fg-subtle", key === dayKey(now) && "text-accent")}>
              {d.toLocaleDateString(locale, { weekday: "short", month: "short", day: "numeric" })}
            </h3>
            {list.length === 0 ? <p className="px-1 text-meta text-fg-subtle">{t("calendar.noEvents")}</p> : null}
            <ul className="flex flex-col gap-0.5">
              {list.map((e, i) => (
                <li key={`${e.calendar_id}-${e.start}-${i}`}>
                  <EventLine event={e} calendar={calendars.get(e.calendar_id)} />
                </li>
              ))}
            </ul>
          </section>
        );
      })}
    </div>
  );
}
