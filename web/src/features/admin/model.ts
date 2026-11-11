import type { Schemas } from "@/shared/api";

export type UsageGroup = Schemas["UsageGroup"];
export type SystemStatus = Schemas["SystemStatus"];
export type AdminStatus = Schemas["AdminStatus"];

// ---- Period

export type Period = { kind: "today" | "7" | "30" | "90" } | { kind: "custom"; from: string; to: string };

/** The calendar day in Budapest, where the server counts usage. */
export function budapestDay(d: Date): string {
  return d.toLocaleDateString("sv-SE", { timeZone: "Europe/Budapest" });
}

export function addDays(day: string, n: number): string {
  const d = new Date(`${day}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
}

export function daysBetween(from: string, to: string): number {
  return Math.round((new Date(`${to}T12:00:00Z`).getTime() - new Date(`${from}T12:00:00Z`).getTime()) / 86_400_000);
}

/** The inclusive days of a period; a custom one is cut to what /usage can return (90 days back). */
export function periodDays(p: Period, today: string): { from: string; to: string; fetchDays: number } {
  if (p.kind === "custom") {
    const earliest = addDays(today, -89);
    const from = p.from < earliest ? earliest : p.from;
    const to = p.to > today ? today : p.to;
    return { from, to: to < from ? from : to, fetchDays: Math.max(1, daysBetween(from, today) + 1) };
  }
  const n = p.kind === "today" ? 1 : Number(p.kind);
  return { from: addDays(today, -(n - 1)), to: today, fetchDays: n };
}

export function dayList(from: string, to: string): string[] {
  const out: string[] = [];
  for (let d = from; d <= to; d = addDays(d, 1)) out.push(d);
  return out;
}

// ---- Money

/** USD → HUF in the client, so a new rate never rewrites history. */
export function toHuf(usd: number, rate: number | undefined): number | null {
  return rate && rate > 0 ? usd * rate : null;
}

export function formatUsd(usd: number, locale: string): string {
  return `$${usd.toLocaleString(locale, { minimumFractionDigits: 2, maximumFractionDigits: usd < 1 ? 4 : 2 })}`;
}

export function formatHuf(huf: number, locale: string): string {
  return `${Math.round(huf).toLocaleString(locale)} Ft`;
}

export function shortTokens(n: number, locale: string): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toLocaleString(locale, { maximumFractionDigits: 1 })}M`;
  if (n >= 1000) return `${(n / 1000).toLocaleString(locale, { maximumFractionDigits: 1 })}k`;
  return n.toLocaleString(locale);
}

export function formatDuration(ms: number): string {
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}:${String(s % 60).padStart(2, "0")}`;
  return `${Math.floor(m / 60)} h ${m % 60} m`;
}

// ---- Charts

/** A categorical order with distant neighbours (validated in the report); an entity's own colour comes first. */
const fallbackHues = ["blue", "orange", "teal", "pink", "lime", "violet", "red", "cyan"] as const;

/**
 * A stable hue per key: the entity's own colour if it has one, otherwise from
 * the name, so a filter never repaints the survivors. A hash collision moves to
 * the next free hue, in sorted key order.
 */
export function assignHues(keys: string[], own: (key: string) => string | undefined): Map<string, string> {
  const out = new Map<string, string>();
  const taken = new Set<string>();
  const sorted = [...keys].sort();
  for (const k of sorted) {
    const hue = own(k);
    if (hue) {
      out.set(k, hue);
      taken.add(hue);
    }
  }
  for (const k of sorted) {
    if (out.has(k)) continue;
    let h = 0;
    for (const c of k) h = (h * 31 + (c.codePointAt(0) ?? 0)) >>> 0;
    let i = h % fallbackHues.length;
    for (let tries = 0; tries < fallbackHues.length && taken.has(fallbackHues[i]!); tries++) i = (i + 1) % fallbackHues.length;
    const hue = fallbackHues[i]!;
    taken.add(hue);
    out.set(k, hue);
  }
  return out;
}

export const OTHER_KEY = "__other__";
export const MAX_SERIES = 6;

export type StackDay = { day: string; segments: { key: string; value: number }[]; total: number };

/** The days of a stacked chart: the biggest series by total, the rest folded into "Egyéb". */
export function stackDays(groups: UsageGroup[], days: string[], value: (g: UsageGroup) => number, maxSeries = MAX_SERIES): { keys: string[]; days: StackDay[] } {
  const inRange = new Set(days);
  const totals = new Map<string, number>();
  for (const g of groups) if (inRange.has(g.day)) totals.set(g.key, (totals.get(g.key) ?? 0) + value(g));
  const ranked = [...totals.entries()].filter(([, v]) => v > 0).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const top = ranked.slice(0, maxSeries).map(([k]) => k);
  const keep = new Set(top);
  const hasOther = ranked.length > top.length;
  const keys = hasOther ? [...top, OTHER_KEY] : top;
  const byDay = new Map<string, Map<string, number>>();
  for (const g of groups) {
    if (!inRange.has(g.day)) continue;
    const key = keep.has(g.key) ? g.key : OTHER_KEY;
    const row = byDay.get(g.day) ?? new Map<string, number>();
    row.set(key, (row.get(key) ?? 0) + value(g));
    byDay.set(g.day, row);
  }
  return {
    keys,
    days: days.map((day) => {
      const row = byDay.get(day);
      const segments = keys.map((key) => ({ key, value: row?.get(key) ?? 0 })).filter((s) => s.value > 0);
      return { day, segments, total: segments.reduce((sum, s) => sum + s.value, 0) };
    }),
  };
}

export type Tiles = { tokens: number; runs: number; failures: number; usd: number; hasUsd: boolean };

/** The KPI tiles over the groups of the period. */
export function periodTiles(groups: UsageGroup[], from: string, to: string): Tiles {
  const out: Tiles = { tokens: 0, runs: 0, failures: 0, usd: 0, hasUsd: false };
  for (const g of groups) {
    if (g.day < from || g.day > to) continue;
    out.tokens += g.total_tokens;
    out.runs += g.run_count;
    out.failures += g.error_count;
    if (g.estimated_usd != null) {
      out.usd += g.estimated_usd;
      out.hasUsd = true;
    }
  }
  return out;
}

export type AgentRow = {
  id: string;
  runs: number;
  input: number;
  output: number;
  cache: number;
  usd: number | null;
  failures: number;
  lastDay: string | null;
  spark: number[];
};

/** One row per agent over the period, with a token sparkline per day. */
export function agentRows(groups: UsageGroup[], days: string[]): AgentRow[] {
  const inRange = new Map(days.map((d, i) => [d, i]));
  const rows = new Map<string, AgentRow>();
  for (const g of groups) {
    const i = inRange.get(g.day);
    if (i === undefined) continue;
    const row: AgentRow = rows.get(g.key) ?? { id: g.key, runs: 0, input: 0, output: 0, cache: 0, usd: null, failures: 0, lastDay: null, spark: days.map(() => 0) };
    row.runs += g.run_count;
    row.input += g.input_tokens;
    row.output += g.output_tokens;
    row.cache += g.cache_read_tokens + g.cache_write_tokens;
    row.failures += g.error_count;
    if (g.estimated_usd != null) row.usd = (row.usd ?? 0) + g.estimated_usd;
    if (g.run_count > 0 && (!row.lastDay || g.day > row.lastDay)) row.lastDay = g.day;
    row.spark[i] = (row.spark[i] ?? 0) + g.total_tokens;
    rows.set(g.key, row);
  }
  return [...rows.values()];
}

export type SortDir = "asc" | "desc";

/** A stable sort; empty values always go last. */
export function sortRows<T>(rows: T[], value: (r: T) => string | number | null, dir: SortDir): T[] {
  return [...rows].sort((a, b) => {
    const va = value(a);
    const vb = value(b);
    if (va === null || vb === null) return va === vb ? 0 : va === null ? 1 : -1;
    const c = typeof va === "number" && typeof vb === "number" ? va - vb : String(va).localeCompare(String(vb));
    return dir === "asc" ? c : -c;
  });
}

// ---- Status page

export type Tone = "ok" | "warning" | "error";
export const LAG_EVENTS = 100;
export const LAG_SECONDS = 60;

export type StatusFinding = { id: string; tone: Tone };

/** The thresholds of the status cards (round 35, part 8). */
export function statusFindings(st: AdminStatus): StatusFinding[] {
  const s = st.system;
  if (!s) return [];
  const lagging = s.consumers.some((c) => c.lag > LAG_EVENTS || c.lag_seconds > LAG_SECONDS);
  const offline = s.guards.some((g) => !g.online);
  return [
    { id: "server", tone: "ok" },
    { id: "database", tone: s.database.ok ? "ok" : "error" },
    { id: "events", tone: lagging ? "warning" : "ok" },
    { id: "connections", tone: "ok" },
    { id: "guards", tone: offline ? "warning" : "ok" },
    { id: "scheduler", tone: s.scheduler.failed_24h > 0 ? "warning" : "ok" },
    { id: "compaction", tone: s.compaction.failed_24h > 0 ? "warning" : "ok" },
    { id: "emergency", tone: st.emergency_stop.active ? "error" : "ok" },
    { id: "integrations", tone: "ok" },
  ];
}

export function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86_400);
  const h = Math.floor((seconds % 86_400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return d > 0 ? `${d} d ${h} h` : h > 0 ? `${h} h ${m} m` : `${m} m`;
}

/** The plain-text summary a bug report gets from "Másolás". No secret ever reaches it: the status holds none. */
export function statusText(st: AdminStatus, at: Date): string {
  const s = st.system;
  if (!s) return "";
  const lines = [
    `Colonyhall ${s.version}, ${at.toISOString()}`,
    `uptime: ${formatUptime(s.uptime_seconds)}`,
    `database: ${s.database.ok ? "ok" : "ERROR"} (${s.database.latency_ms.toFixed(1)} ms)`,
    `event head: ${s.event_head}`,
    ...s.consumers.map((c) => `consumer ${c.name}: offset ${c.offset}, lag ${c.lag} (${c.lag_seconds} s)`),
    `connections: human ${s.connections.human}, agent ${s.connections.agent}, host ${s.connections.host}`,
    ...s.guards.map((g) => `guard ${g.name}: ${g.online ? "online" : "offline"}, ${g.guard_version ?? "?"}, last seen ${g.last_seen_at ?? "never"}`),
    `scheduler: next ${s.scheduler.next_run_at ?? "none"}, failed in 24 h: ${s.scheduler.failed_24h}`,
    `compaction: running ${s.compaction.running}, failed in 24 h: ${s.compaction.failed_24h}`,
    `emergency stop: ${st.emergency_stop.active ? `ACTIVE since ${st.emergency_stop.since ?? "?"}` : "off"}`,
    `gitea configured: ${s.gitea_configured ? "yes" : "no"}, webhook secret: ${s.gitea_webhook_configured ? "yes" : "no"}`,
    `max upload: ${s.max_upload_mb} MB`,
  ];
  return lines.join("\n");
}

// ---- Users

export const roles = ["admin", "member", "observer"] as const;
const HANDLE = /^[a-z0-9-]{2,32}$/;

export function validHandle(h: string): boolean {
  return HANDLE.test(h);
}

export const MIN_PASSWORD = 12;

/** A password for "Generálás": letters and digits without look-alikes, from the browser's generator. */
export function generatePassword(length = 20, random: (a: Uint32Array<ArrayBuffer>) => Uint32Array = (a) => crypto.getRandomValues(a)): string {
  const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  const limit = Math.floor(0x1_0000_0000 / alphabet.length) * alphabet.length;
  let out = "";
  while (out.length < length) {
    for (const n of random(new Uint32Array(length))) {
      if (n < limit && out.length < length) out += alphabet[n % alphabet.length];
    }
  }
  return out;
}

// ---- Git

const REPO = /^[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+$/;

export function validRepo(repo: string): boolean {
  return REPO.test(repo);
}

// ---- Prices

const PRICE_FIELDS = ["input_per_mtok", "output_per_mtok", "cache_read_per_mtok", "cache_write_per_mtok"] as const;
export type PriceField = (typeof PRICE_FIELDS)[number];
export const priceFields = PRICE_FIELDS;

/** The price rows per model, newest first; the first of a model is the one in force. */
export function priceHistory<T extends { model: string; valid_from: string }>(prices: T[]): { model: string; rows: T[] }[] {
  const by = new Map<string, T[]>();
  for (const p of prices) by.set(p.model, [...(by.get(p.model) ?? []), p]);
  return [...by.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([model, rows]) => ({ model, rows: [...rows].sort((a, b) => b.valid_from.localeCompare(a.valid_from)) }));
}

// ---- The owner key wizard

export type WizardState = { step: 1 | 2 | 3; newTokenId: string | null; saved: boolean; places: string[]; done: boolean };
export const wizardStart: WizardState = { step: 1, newTokenId: null, saved: false, places: [], done: false };
export const wizardPlaces = ["scripts", "env", "machines", "other"] as const;

export type WizardAction =
  | { type: "created"; tokenId: string }
  | { type: "saved"; saved: boolean }
  | { type: "place"; place: string; on: boolean }
  | { type: "goto"; step: 1 | 2 | 3 }
  | { type: "finish" }
  | { type: "restart" };

/** The wizard's only memory (per viewer, localStorage). The secret itself never gets here. */
export function wizardReducer(s: WizardState, a: WizardAction): WizardState {
  switch (a.type) {
    case "created":
      return { ...s, newTokenId: a.tokenId, saved: false };
    case "saved":
      return { ...s, saved: a.saved };
    case "place":
      return { ...s, places: a.on ? [...new Set([...s.places, a.place])] : s.places.filter((p) => p !== a.place) };
    case "goto":
      if (a.step > 1 && !s.saved) return s;
      return { ...s, step: a.step };
    case "finish":
      return { ...wizardStart, done: true };
    case "restart":
      return wizardStart;
  }
}

// ---- Audit

export type AuditEntry = Schemas["AuditEntry"];

/** The payload as a flat key/value list, shown as text only. */
export function payloadRows(payload: Record<string, unknown> | null | undefined): { key: string; value: string }[] {
  return Object.entries(payload ?? {}).map(([key, value]) => ({ key, value: typeof value === "string" ? value : JSON.stringify(value) }));
}

/** The export link with the same filters as the table. */
export function auditExportQuery(f: { actor_id?: string; action?: string[]; target_type?: string; from?: string; to?: string }): string {
  const q = new URLSearchParams();
  if (f.actor_id) q.set("actor_id", f.actor_id);
  for (const a of f.action ?? []) q.append("action", a);
  if (f.target_type) q.set("target_type", f.target_type);
  if (f.from) q.set("from", f.from);
  if (f.to) q.set("to", f.to);
  return q.toString();
}
