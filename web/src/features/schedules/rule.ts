// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The schedule builder's choices and the cron they make (round 32,
// part 6). Cron weekdays: 0 = Sunday … 6 = Saturday, as robfig/cron reads them.
// The server's parser has no "L" (last day of the month), so Havonta offers 1–31.

export type Rule =
  | { mode: "once"; date: string; time: string }
  | { mode: "daily"; time: string; workdays: boolean }
  | { mode: "weekly"; days: number[]; time: string }
  | { mode: "monthly"; day: number; time: string }
  | { mode: "advanced"; cron: string };

export type Mode = Rule["mode"];

/** Monday first, the way the chips are shown. */
export const weekOrder = [1, 2, 3, 4, 5, 6, 0];

const hm = (time: string): [number, number] | null => {
  const m = /^(\d{1,2}):(\d{2})$/.exec(time.trim());
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  return h < 24 && min < 60 ? [h, min] : null;
};

/** The weekday set as cron writes it: ranges where they are consecutive ("1-5", "1,3,5"). */
function dowList(days: number[]): string {
  const sorted = [...new Set(days)].sort((a, b) => a - b);
  const parts: string[] = [];
  for (let i = 0; i < sorted.length; ) {
    let j = i;
    while (j + 1 < sorted.length && sorted[j + 1] === sorted[j]! + 1) j++;
    parts.push(j - i >= 2 ? `${sorted[i]}-${sorted[j]}` : sorted.slice(i, j + 1).join(","));
    i = j + 1;
  }
  return parts.join(",");
}

/** What the create (and preview) request needs: `at` for a one-off, `cron` otherwise; null when incomplete. */
export function buildCron(rule: Rule): { at: string } | { cron: string } | null {
  if (rule.mode === "advanced") return rule.cron.trim() ? { cron: rule.cron.trim().replace(/\s+/g, " ") } : null;
  const t = hm(rule.time);
  if (!t) return null;
  const [h, m] = t;
  switch (rule.mode) {
    case "once":
      return /^\d{4}-\d{2}-\d{2}$/.test(rule.date) ? { at: `${rule.date} ${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}` } : null;
    case "daily":
      return { cron: `${m} ${h} * * ${rule.workdays ? "1-5" : "*"}` };
    case "weekly":
      return rule.days.length ? { cron: `${m} ${h} * * ${dowList(rule.days)}` } : null;
    case "monthly":
      return rule.day >= 1 && rule.day <= 31 ? { cron: `${m} ${h} ${rule.day} * *` } : null;
  }
}

function parseDow(field: string): number[] | null {
  const out: number[] = [];
  for (const part of field.split(",")) {
    const r = /^(\d)(?:-(\d))?$/.exec(part);
    if (!r) return null;
    const a = Number(r[1]);
    const b = r[2] === undefined ? a : Number(r[2]);
    if (a > b || b > 7) return null;
    for (let d = a; d <= b; d++) out.push(d % 7);
  }
  return [...new Set(out)];
}

const time = (h: string, m: string) => `${h.padStart(2, "0")}:${m.padStart(2, "0")}`;

/** The builder's choice for a cron it could have made; any other cron is "advanced". */
export function parseCron(cron: string): Rule {
  const f = cron.trim().split(/\s+/);
  const plain = (s: string | undefined) => s !== undefined && /^\d{1,2}$/.test(s);
  if (f.length === 5 && plain(f[0]) && plain(f[1]) && f[3] === "*") {
    const t = time(f[1]!, f[0]!);
    if (hm(t)) {
      if (f[2] === "*" && f[4] === "*") return { mode: "daily", time: t, workdays: false };
      if (f[2] === "*") {
        const days = parseDow(f[4]!);
        if (days && days.length === 5 && [1, 2, 3, 4, 5].every((d) => days.includes(d))) return { mode: "daily", time: t, workdays: true };
        if (days) return { mode: "weekly", days: weekOrder.filter((d) => days.includes(d)), time: t };
      }
      if (plain(f[2]) && f[4] === "*" && Number(f[2]) >= 1 && Number(f[2]) <= 31) return { mode: "monthly", day: Number(f[2]), time: t };
    }
  }
  return { mode: "advanced", cron: cron.trim() };
}

type T = (key: string, opts?: Record<string, unknown>) => string;

/** The rule in words: "Minden hétköznap 09:00-kor", "Hetente hétfőn és szerdán 18:30-kor". */
export function describeRule(rule: Rule, t: T, locale: string): string {
  const dayName = (d: number) => new Date(2026, 9, 25 + d).toLocaleDateString(locale, { weekday: "long" }); // 2026-10-25 is a Sunday
  switch (rule.mode) {
    case "once": {
      const d = new Date(`${rule.date}T${rule.time.padStart(5, "0")}`);
      return t("rule.once", { when: Number.isNaN(d.getTime()) ? `${rule.date} ${rule.time}` : d.toLocaleDateString(locale, { year: "numeric", month: "long", day: "numeric", weekday: "long" }), time: rule.time });
    }
    case "daily":
      return t(rule.workdays ? "rule.workdays" : "rule.daily", { time: rule.time });
    case "weekly": {
      const names = weekOrder.filter((d) => rule.days.includes(d)).map(dayName);
      const list = names.length > 1 ? new Intl.ListFormat(locale, { type: "conjunction" }).format(names) : (names[0] ?? "");
      return t("rule.weekly", { days: list, time: rule.time });
    }
    case "monthly":
      return t("rule.monthly", { day: rule.day, time: rule.time });
    case "advanced":
      return t("rule.advanced", { cron: rule.cron });
  }
}

const pad = (n: number) => String(n).padStart(2, "0");
const ymd = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

/** The one-off quick chips: Ma 18:00, Holnap 09:00, Hétfő 09:00, 1 óra múlva (rounded up to 15 minutes). */
export function quickChips(now: Date): { id: "today" | "tomorrow" | "monday" | "hour"; date: string; time: string }[] {
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  const monday = new Date(now.getFullYear(), now.getMonth(), now.getDate() + (((8 - now.getDay()) % 7) || 7));
  const hour = new Date(now.getTime() + 60 * 60_000);
  const q = Math.ceil((hour.getHours() * 60 + hour.getMinutes()) / 15) * 15;
  const inHour = new Date(hour.getFullYear(), hour.getMonth(), hour.getDate(), 0, q);
  const chips = [
    { id: "today" as const, date: ymd(now), time: "18:00" },
    { id: "tomorrow" as const, date: ymd(tomorrow), time: "09:00" },
    { id: "monday" as const, date: ymd(monday), time: "09:00" },
    { id: "hour" as const, date: ymd(inHour), time: `${pad(inHour.getHours())}:${pad(inHour.getMinutes())}` },
  ];
  return now.getHours() >= 18 ? chips.filter((c) => c.id !== "today") : chips;
}
