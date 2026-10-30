import { useTranslation } from "react-i18next";
import { useNow } from "@/shared/hooks";
import { useCalendars, useEvents } from "./api";
import { minutesToNext } from "./calendar";

/** The minimised calendar circle's badge: the minutes to the next event within the hour; news within ten. */
export function useCalendarBadge(enabled: boolean): { text: string | null; news: boolean } {
  const { t } = useTranslation("planner");
  const now = useNow();
  const calendars = useCalendars();
  // A stable query key: the window moves on the hour, a minute-based one would refetch every minute.
  const from = new Date(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours());
  const to = new Date(from.getTime() + 2 * 3_600_000);
  const events = useEvents(from, to, enabled && (calendars.data?.length ?? 0) > 0);
  const m = enabled ? minutesToNext(events.data ?? [], now) : null;
  return { text: m === null ? null : t("calendar.badge", { m }), news: m !== null && m <= 10 };
}
