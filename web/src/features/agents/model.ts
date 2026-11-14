// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Schemas } from "@/shared/api";

export type AgentOverview = Schemas["AgentOverview"];
export type OverviewThread = Schemas["OverviewThread"];
export type Run = Schemas["Run"];
export type DayUsage = Schemas["DayUsage"];
export type DelegationGrant = Schemas["DelegationGrant"];
export type Host = Schemas["Host"];

/** The overview header's state chip (round 33, part 3). */
export type DisplayState = "working" | "idle" | "offline" | "paused" | "stopped" | "archived";

/** The settings dialog's tabs, as they appear in the URL (?agent=<handle>&settings=<tab>). */
export type SettingsTab = "profil" | "keretek" | "konfiguracio" | "utasitas" | "veszelyes";

/** A machine counts as offline when its guard has not reported for this long. */
export const OFFLINE_AFTER_MS = 5 * 60_000;

/**
 * Pure: what the header shows. Archived and stopped win over everything,
 * then paused, then live work; an agent whose machine has gone quiet is
 * offline, otherwise idle.
 */
export function displayState(a: { archived: boolean; state: string; working: boolean; hostLastSeen?: string | undefined }, now: Date): DisplayState {
  if (a.archived) return "archived";
  if (a.state === "stopped") return "stopped";
  if (a.state === "paused") return "paused";
  if (a.working) return "working";
  if (a.hostLastSeen !== undefined) {
    const seen = new Date(a.hostLastSeen).getTime();
    if (Number.isNaN(seen) || now.getTime() - seen > OFFLINE_AFTER_MS) return "offline";
  }
  return "idle";
}

/** Pure: the session's context fill in percent (0 when unmeasured or unlimited). */
export function contextPercent(t: { context_tokens?: number; max_session_tokens: number }): number {
  const ctx = t.context_tokens ?? 0;
  if (ctx <= 0 || t.max_session_tokens <= 0) return 0;
  return Math.min(100, (ctx / t.max_session_tokens) * 100);
}

/** Pure: a session is close to rotation from 90 % of its limit (amber starts at 80 %). */
export function nearRotation(t: { context_tokens?: number; max_session_tokens: number }): boolean {
  return contextPercent(t) >= 90;
}

/**
 * Pure: the last seven days of usage for one agent, oldest first, with the
 * missing days filled with zeros. `today` is the Europe/Budapest day key
 * (YYYY-MM-DD) the server counts in.
 */
export function lastSevenDays(rows: DayUsage[], agentId: string, today: string): Array<{ day: string; tokens: number; runs: number }> {
  const byDay = new Map<string, { tokens: number; runs: number }>();
  for (const r of rows) {
    if (r.agent_id !== agentId) continue;
    const cur = byDay.get(r.day) ?? { tokens: 0, runs: 0 };
    byDay.set(r.day, { tokens: cur.tokens + r.total_tokens, runs: cur.runs + r.run_count });
  }
  const [y, m, d] = today.split("-").map(Number);
  const out: Array<{ day: string; tokens: number; runs: number }> = [];
  for (let i = 6; i >= 0; i--) {
    const date = new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, (d ?? 1) - i));
    const key = date.toISOString().slice(0, 10);
    out.push({ day: key, ...(byDay.get(key) ?? { tokens: 0, runs: 0 }) });
  }
  return out;
}

/** The Europe/Budapest day key of a moment (the server's usage days). */
export function budapestDay(now: Date): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Budapest", year: "numeric", month: "2-digit", day: "2-digit" }).format(now);
}

/** Pure: the active delegation grants this agent is part of, soonest expiry first. */
export function grantsOf(grants: DelegationGrant[], agentId: string): DelegationGrant[] {
  return grants
    .filter((g) => !g.revoked_at && (g.agent_a_id === agentId || g.agent_b_id === agentId))
    .sort((a, b) => a.effective_expires_at.localeCompare(b.effective_expires_at));
}

/** Pure: the first non-empty lines of a text (the instructions preview). */
export function firstLines(text: string, n: number): { lines: string[]; more: boolean } {
  const all = text.split("\n").filter((l) => l.trim() !== "");
  return { lines: all.slice(0, n), more: all.length > n };
}

/** Pure: a duration as m:ss, or h:mm:ss from an hour. */
export function formatDuration(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(s / 3600);
  const mm = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(mm).padStart(2, "0")}:${ss}` : `${mm}:${ss}`;
}

/** Pure: a token count in short form (12.3k, 1.2M). */
export function shortTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

/** Pure: the link to a channel or a thread in it. */
export function placeHref(channelId: string, threadRootId?: string | null): string {
  return `/c/${channelId}${threadRootId ? `?thread=${threadRootId}` : ""}`;
}
