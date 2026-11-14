// Saved items and reminders (round 55, part E): the reminder times are
// chosen in the person's own time zone (their profile), sent as instants.

/** The UTC offset of a time zone at an instant, in milliseconds (east of UTC positive). */
export function zoneOffset(at: Date, timeZone: string): number {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    hourCycle: "h23",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).formatToParts(at);
  const get = (t: string) => Number(parts.find((p) => p.type === t)?.value ?? 0);
  const asUTC = Date.UTC(get("year"), get("month") - 1, get("day"), get("hour"), get("minute"), get("second"));
  return asUTC - Math.floor(at.getTime() / 1000) * 1000;
}

/** The instant of a wall-clock time in a time zone (month is 1-12). */
export function zonedTime(year: number, month: number, day: number, hour: number, minute: number, timeZone: string): Date {
  const guess = Date.UTC(year, month - 1, day, hour, minute);
  let t = guess - zoneOffset(new Date(guess), timeZone);
  // Around a DST change the offset at the result can differ: one more step.
  t = guess - zoneOffset(new Date(t), timeZone);
  return new Date(t);
}

/** The wall-clock date in a time zone: [year, month 1-12, day, weekday 0=Sunday]. */
export function zonedDate(at: Date, timeZone: string): [number, number, number, number] {
  const parts = new Intl.DateTimeFormat("en-US", { timeZone, year: "numeric", month: "numeric", day: "numeric", weekday: "short" }).formatToParts(at);
  const get = (t: string) => parts.find((p) => p.type === t)?.value ?? "";
  const weekdays = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  return [Number(get("year")), Number(get("month")), Number(get("day")), weekdays.indexOf(get("weekday"))];
}

/** A wall-clock time `days` after `at`'s date in the zone. */
export function dayAt(at: Date, days: number, hour: number, minute: number, timeZone: string): Date {
  const [y, m, d] = zonedDate(at, timeZone);
  const base = new Date(Date.UTC(y, m - 1, d + days));
  return zonedTime(base.getUTCFullYear(), base.getUTCMonth() + 1, base.getUTCDate(), hour, minute, timeZone);
}

export type ReminderChoice = "20m" | "1h" | "tomorrow" | "monday";
export const reminderChoices: ReminderChoice[] = ["20m", "1h", "tomorrow", "monday"];

/** The natural choices: in 20 minutes, in 1 hour, tomorrow 9:00, next Monday 9:00 (in the zone). */
export function reminderTime(choice: ReminderChoice, now: Date, timeZone: string): Date {
  switch (choice) {
    case "20m":
      return new Date(now.getTime() + 20 * 60_000);
    case "1h":
      return new Date(now.getTime() + 60 * 60_000);
    case "tomorrow":
      return dayAt(now, 1, 9, 0, timeZone);
    case "monday": {
      const weekday = zonedDate(now, timeZone)[3];
      return dayAt(now, ((8 - weekday) % 7) || 7, 9, 0, timeZone);
    }
  }
}

/** The person's time zone: their profile's, else the browser's. */
export function timeZoneOf(profile?: string | null): string {
  if (profile) {
    try {
      new Intl.DateTimeFormat("en-US", { timeZone: profile });
      return profile;
    } catch {
      // An unknown zone: fall back.
    }
  }
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/** "2026-11-15T09:00" (a datetime-local value) in the zone → the instant. */
export function fromLocalInput(value: string, timeZone: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value);
  if (!m) return null;
  return zonedTime(Number(m[1]), Number(m[2]), Number(m[3]), Number(m[4]), Number(m[5]), timeZone);
}
