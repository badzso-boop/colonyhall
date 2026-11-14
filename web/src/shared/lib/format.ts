// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Dates, day separators and the session context label. Ported from the dev
// page's format.js (round 30), with its tests.

const asDate = (d: Date | string): Date => (d instanceof Date ? d : new Date(d));

/** The viewer's local calendar day of d, e.g. "2026-10-29"; "" for an invalid date. */
export function dayKey(d: Date | string): string {
  const date = asDate(d);
  if (Number.isNaN(date.getTime())) return "";
  const m = date.getMonth() + 1;
  const day = date.getDate();
  return `${date.getFullYear()}-${m < 10 ? "0" : ""}${m}-${day < 10 ? "0" : ""}${day}`;
}

function addDays(d: Date, n: number): Date {
  const out = new Date(d.getTime());
  out.setDate(out.getDate() + n);
  return out;
}

/** The date and time next to a message: always the day, the year only when it is not this year. */
export function messageTime(d: Date | string, now: Date, locale: string): string {
  const date = asDate(d);
  if (Number.isNaN(date.getTime())) return "";
  const opts: Intl.DateTimeFormatOptions = { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" };
  if (date.getFullYear() !== now.getFullYear()) opts.year = "numeric";
  return date.toLocaleString(locale, opts);
}

/** Only the time, for messages of today. */
export function shortTime(d: Date | string, locale: string): string {
  const date = asDate(d);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit" });
}

/** The tooltip: the complete date with weekday and time. */
export function fullTime(d: Date | string, locale: string): string {
  const date = asDate(d);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString(locale, {
    year: "numeric", month: "long", day: "numeric", weekday: "long", hour: "2-digit", minute: "2-digit", second: "2-digit",
  });
}

/** A day separator's label: "today"/"yesterday" by word, other days by date and weekday. */
export function dayLabel(d: Date | string, now: Date, locale: string, words: { today: string; yesterday: string }): string {
  const date = asDate(d);
  if (Number.isNaN(date.getTime())) return "";
  const key = dayKey(date);
  if (key === dayKey(now)) return words.today;
  if (key === dayKey(addDays(now, -1))) return words.yesterday;
  const opts: Intl.DateTimeFormatOptions = { month: "long", day: "numeric", weekday: "long" };
  if (date.getFullYear() !== now.getFullYear()) opts.year = "numeric";
  return date.toLocaleDateString(locale, opts);
}

export type FeedItem<M> = { separator: true; day: string; label: string } | { separator: false; message: M };

/** Day separators interleaved with the messages, one before the first message of each local day. */
export function feedItems<M extends { created_at: string }>(
  messages: M[],
  now: Date,
  locale: string,
  words: { today: string; yesterday: string },
): FeedItem<M>[] {
  const out: FeedItem<M>[] = [];
  let last: string | null = null;
  for (const m of messages) {
    const key = dayKey(m.created_at);
    if (key && key !== last) {
      out.push({ separator: true, day: key, label: dayLabel(m.created_at, now, locale, words) });
      last = key;
    }
    out.push({ separator: false, message: m });
  }
  return out;
}

/** A relative time for summaries: "2 perce", "tegnap 18:20" via Intl. */
export function relativeTime(d: Date | string, now: Date, locale: string): string {
  const date = asDate(d);
  if (Number.isNaN(date.getTime())) return "";
  const secs = Math.round((date.getTime() - now.getTime()) / 1000);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const abs = Math.abs(secs);
  if (abs < 60) return rtf.format(secs, "second");
  if (abs < 3600) return rtf.format(Math.round(secs / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(secs / 3600), "hour");
  return messageTime(date, now, locale);
}

/** The session's measured context against its limit (the agent overview). */
export function sessionContext(t: { context_tokens?: number; max_session_tokens?: number; last_run_tokens?: number }) {
  const k = (n: number) => `${Math.round(n / 1000)}k`;
  const ctx = (t.context_tokens ?? 0) > 0 ? t.context_tokens! : 0;
  const max = (t.max_session_tokens ?? 0) > 0 ? t.max_session_tokens! : 0;
  return {
    known: ctx > 0,
    near: ctx > 0 && max > 0 && ctx >= max * 0.8,
    text: `${ctx > 0 ? k(ctx) : "–"} / ${max > 0 ? k(max) : "∞"}`,
    runTokens: (t.last_run_tokens ?? 0) > 0 ? k(t.last_run_tokens!) : "0k",
  };
}

const BYTE_UNITS = ["B", "kB", "MB", "GB", "TB"] as const;

/** A byte count the way pg_size_pretty shows it (1 kB = 1024 B): "21.4 MB". */
export function formatBytes(bytes: number, locale: string): string {
  const sign = bytes < 0 ? "-" : "";
  let n = Math.abs(bytes);
  let unit = 0;
  while (n >= 1024 && unit < BYTE_UNITS.length - 1) {
    n /= 1024;
    unit++;
  }
  return `${sign}${n.toLocaleString(locale, { maximumFractionDigits: unit === 0 ? 0 : 1 })} ${BYTE_UNITS[unit]}`;
}
