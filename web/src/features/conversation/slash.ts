// The composer's slash commands (round 55, part B): which message "this"
// is, and when a /remind fires. Pure; useSlash runs them.
import type { RemindWhen } from "@/shared/lib";
import { dayAt, zonedDate } from "@/features/saved";
import type { MessagePages } from "./api";

/** The newest message in the cached pages (newest page first, each page oldest first) that matches. */
export function latestMessage(pages: MessagePages | undefined, match: (m: MessagePages["pages"][number][number]) => boolean) {
  for (const page of pages?.pages ?? []) {
    for (let i = page.length - 1; i >= 0; i--) {
      const m = page[i]!;
      if (!m.deleted_at && m.kind !== "system" && match(m)) return m;
    }
  }
  return undefined;
}

/** The instant of a /remind time in the person's zone; a time of day already past today means tomorrow. */
export function remindAt(when: RemindWhen, now: Date, timeZone: string): Date {
  if (when.kind === "in") return new Date(now.getTime() + when.minutes * 60_000);
  if (when.days === "monday") {
    const weekday = zonedDate(now, timeZone)[3];
    return dayAt(now, (8 - weekday) % 7 || 7, when.hour, when.minute, timeZone);
  }
  const at = dayAt(now, when.days, when.hour, when.minute, timeZone);
  return when.days === 0 && at.getTime() <= now.getTime() ? dayAt(now, 1, when.hour, when.minute, timeZone) : at;
}
