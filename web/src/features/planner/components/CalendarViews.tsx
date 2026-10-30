import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { cn, dayKey, shortTime } from "@/shared/lib";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui";
import { layoutDay, type CalEvent } from "../calendar";
import { Dot, EventButton, EventLine, type Calendar } from "./CalendarParts";

type ViewProps = {
  days: Date[];
  byDay: Map<string, CalEvent[]>;
  calendars: Map<string, Calendar>;
  now: Date;
};

const MaxInCell = 3;

/** Monday-first weekday names in the UI language, from the first week of the grid. */
function weekdays(days: Date[], locale: string, style: "short" | "narrow") {
  return days.slice(0, 7).map((d) => d.toLocaleDateString(locale, { weekday: style }));
}

/**
 * The 6×7 month grid. On a phone (`dots`) the cells show coloured dots and a
 * tap selects the day; otherwise up to three events, "+N további" and a click
 * on the date opens the day view.
 */
export function MonthView({ days, byDay, calendars, now, month, onDay, dots, selected }: ViewProps & { month: number; onDay: (d: Date) => void; dots?: boolean; selected?: string }) {
  const { t, i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  const today = dayKey(now);
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="grid grid-cols-7 border-b border-border text-center text-meta font-semibold text-fg-subtle uppercase">
        {weekdays(days, locale, dots ? "narrow" : "short").map((w, i) => (
          <div key={i} className="py-1">
            {w}
          </div>
        ))}
      </div>
      <div className="grid flex-1 auto-rows-fr grid-cols-7">
        {days.map((d) => {
          const key = dayKey(d);
          const list = byDay.get(key) ?? [];
          const other = d.getMonth() !== month;
          const label = d.toLocaleDateString(locale, { month: "long", day: "numeric", weekday: "long" });
          const date = (
            <span className={cn("grid size-6 place-items-center rounded-full text-meta tabular-nums", key === today && "bg-accent font-semibold text-accent-fg", other && key !== today && "text-fg-subtle")}>
              {d.getDate()}
            </span>
          );
          if (dots)
            return (
              <button
                key={key}
                type="button"
                aria-label={label}
                aria-pressed={selected === key}
                onClick={() => onDay(d)}
                className={cn("flex min-h-12 flex-col items-center gap-1 border-r border-b border-border py-1", selected === key && "bg-accent-soft")}
              >
                {date}
                <span className="flex gap-0.5">
                  {list.slice(0, 3).map((e, i) => (
                    <Dot key={i} colour={calendars.get(e.calendar_id)?.colour} className="size-1.5" />
                  ))}
                </span>
              </button>
            );
          const shown = list.slice(0, list.length > MaxInCell ? MaxInCell - 1 : MaxInCell);
          return (
            <div key={key} className={cn("flex min-h-24 min-w-0 flex-col gap-0.5 border-r border-b border-border p-1", other && "bg-surface-sunken/50")}>
              <button type="button" aria-label={t("calendar.openDay", { day: label })} onClick={() => onDay(d)} className="self-start rounded-full hover:bg-surface-sunken">
                {date}
              </button>
              {shown.map((e, i) => (
                <EventLine key={i} event={e} calendar={calendars.get(e.calendar_id)} />
              ))}
              {list.length > shown.length ? (
                <Popover>
                  <PopoverTrigger asChild>
                    <button type="button" className="self-start px-1 text-meta font-medium text-fg-muted hover:text-fg">
                      {t("calendar.more", { count: list.length - shown.length })}
                    </button>
                  </PopoverTrigger>
                  <PopoverContent label={label} className="w-64">
                    <p className="mb-2 font-semibold">{label}</p>
                    <div className="flex flex-col gap-0.5">
                      {list.map((e, i) => (
                        <EventLine key={i} event={e} calendar={calendars.get(e.calendar_id)} />
                      ))}
                    </div>
                  </PopoverContent>
                </Popover>
              ) : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}

const HourPx = 48;

/** Week, three-day and day views: an all-day strip and the hour grid with a red now line. */
export function TimeGrid({ days, byDay, calendars, now, wide }: ViewProps & { wide?: boolean }) {
  const { i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  const scroller = useRef<HTMLDivElement>(null);
  useEffect(() => {
    // 07:00 at the top; 0–24 stays scrollable.
    if (scroller.current) scroller.current.scrollTop = 7 * HourPx;
  }, []);
  const today = dayKey(now);
  const cols = { gridTemplateColumns: `3.5rem repeat(${days.length}, minmax(0, 1fr))` };
  const nowTop = ((now.getHours() * 60 + now.getMinutes()) / 60) * HourPx;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="grid border-b border-border" style={cols}>
        <div />
        {days.map((d) => (
          <div key={dayKey(d)} className={cn("border-l border-border px-1 py-1 text-center text-meta", dayKey(d) === today ? "font-semibold text-accent" : "text-fg-muted")}>
            {d.toLocaleDateString(locale, { weekday: "short", day: "numeric" })}
          </div>
        ))}
        <div />
        {days.map((d) => (
          <div key={dayKey(d)} className="flex min-w-0 flex-col gap-0.5 border-l border-border p-0.5">
            {(byDay.get(dayKey(d)) ?? [])
              .filter((e) => e.all_day)
              .map((e, i) => (
                <EventLine key={i} event={e} calendar={calendars.get(e.calendar_id)} />
              ))}
          </div>
        ))}
      </div>
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto">
        <div className="relative grid" style={{ ...cols, height: 24 * HourPx }}>
          <div className="relative">
            {Array.from({ length: 23 }, (_, h) => (
              <span key={h} className="absolute right-1.5 -translate-y-1/2 text-meta text-fg-subtle tabular-nums" style={{ top: (h + 1) * HourPx }}>
                {String(h + 1).padStart(2, "0")}:00
              </span>
            ))}
          </div>
          {days.map((d) => {
            const key = dayKey(d);
            return (
              <div key={key} className="relative border-l border-border bg-[repeating-linear-gradient(to_bottom,transparent_0,transparent_47px,var(--color-border)_47px,var(--color-border)_48px)]">
                {layoutDay(byDay.get(key) ?? [], d).map((p, i) => {
                  const cal = calendars.get(p.event.calendar_id);
                  return (
                    <EventButton
                      key={i}
                      event={p.event}
                      calendar={cal}
                      className="absolute overflow-hidden rounded-sm border-l-[3px] bg-surface-raised px-1 py-0.5 text-meta shadow-raised hover:z-10"
                      style={{
                        top: (p.top / 60) * HourPx,
                        height: Math.max(18, (p.height / 60) * HourPx - 2),
                        left: `calc(${(p.col / p.cols) * 100}% + 2px)`,
                        width: `calc(${100 / p.cols}% - 4px)`,
                        borderLeftColor: cal?.colour || "var(--color-fg-subtle)",
                      }}
                    >
                      <span className="block truncate font-medium">{p.event.title}</span>
                      <span className="block truncate text-fg-subtle tabular-nums">
                        {shortTime(p.event.start, locale)}
                        {wide && p.event.location ? ` · ${p.event.location}` : ""}
                      </span>
                    </EventButton>
                  );
                })}
                {key === today ? (
                  <div aria-hidden className="pointer-events-none absolute inset-x-0 z-20 h-0.5 bg-accent" style={{ top: nowTop }}>
                    <span className="absolute -top-1 -left-1 size-2.5 rounded-full bg-accent" />
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
