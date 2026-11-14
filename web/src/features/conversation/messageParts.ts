// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Pure rules for the parts of a message the dev page showed and the PWA
// ports (round 34, part 6). The texts they read are written by the
// server or the guard, so the patterns follow those (Hungarian) bodies.
import type { Schemas } from "@/shared/api";

/**
 * A terminal notice of an agent (round 26), as the guard words it:
 * "⚙️ build kész …", "⚠️ build hibával állt le", "⛔ build leállítva",
 * "🖥 build elindult". Returns the terminal's name.
 */
export function terminalNotice(body: string): string | null {
  const m = /^(?:⚙️|⚠️|⛔|🖥)\s+([a-z0-9][a-z0-9-]{0,31})\s+(?:kész|hibával állt le|leállítva|elindult)/u.exec(body);
  return m ? m[1]! : null;
}

/** The guard's rate-limit note (round 27, POST /agent/ratelimit-note): "⏸ A Claude … kerete …". */
export function isRateLimitNote(body: string, authorKind: string | undefined): boolean {
  return authorKind === "agent" && /^⏸\s/u.test(body);
}

/** Agents whose machine reports the Claude limits paused (only claude-code agents wait for them). */
export function claudePausedAgents(hosts: Schemas["Host"][]): Set<string> {
  const out = new Set<string>();
  for (const h of hosts) {
    if (!h.report.claude_limits?.paused) continue;
    for (const a of h.agents) if (a.client === "claude-code") out.add(a.agent_id);
  }
  return out;
}

/** The reactions offered on a message (the dev page's set); the tooltip says what an agent reads from them. */
export const REACTIONS = ["👍", "👎"] as const;

/** "owner/repo" of a Gitea link card's URL (…/owner/repo/pulls/3 → owner/repo). */
export function repoOf(url: string): string | null {
  try {
    const parts = new URL(url).pathname.split("/").filter(Boolean);
    return parts.length >= 2 ? `${parts[0]}/${parts[1]}` : null;
  } catch {
    return null;
  }
}

/** The badge tone of a Gitea state. */
export function cardTone(state: string): "success" | "info" | "danger" | "neutral" {
  const s = state.toLowerCase();
  if (s === "open" || s === "opened") return "success";
  if (s === "merged") return "info";
  if (s === "closed") return "danger";
  return "neutral";
}
