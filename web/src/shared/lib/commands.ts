// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The composer's slash commands (from the dev page). They are understood in
// the client: /schedule, /daily and /cron create a schedule, /answer answers
// a question; anything else is sent as a message.

export type Command =
  | { kind: "schedule"; body: string; at?: string; cron?: string }
  | { kind: "answer"; question: string; answer: string }
  | { kind: "invalid"; name: "schedule" | "answer" }
  | { kind: "message" };

/** The commands, for the composer's "/" menu (descriptions are i18n keys). */
export const commandList = [
  { name: "schedule", usage: "/schedule 2026-11-02 09:00 …" },
  { name: "daily", usage: "/daily 09:00 …" },
  { name: "cron", usage: '/cron "0 9 * * 1-5" …' },
  { name: "answer", usage: "/answer Q-12 …" },
] as const;

export function parseCommand(text: string): Command {
  const body = text.trim();
  if (/^\/(schedule|daily|cron)\b/.test(body)) {
    let m = /^\/schedule\s+(\d{4}[-.]\d{2}[-.]\d{2}\.?)\s+(\d{1,2}:\d{2})\s+([\s\S]+)$/.exec(body);
    if (m) return { kind: "schedule", at: `${m[1]!.replace(/\./g, "-").replace(/-$/, "")} ${m[2]!.padStart(5, "0")}`, body: m[3]! };
    m = /^\/daily\s+(\d{1,2}):(\d{2})\s+([\s\S]+)$/.exec(body);
    if (m) return { kind: "schedule", cron: `${+m[2]!} ${+m[1]!} * * *`, body: m[3]! };
    m = /^\/cron\s+"([^"]+)"\s+([\s\S]+)$/.exec(body);
    if (m) return { kind: "schedule", cron: m[1]!, body: m[2]! };
    return { kind: "invalid", name: "schedule" };
  }
  if (/^\/answer\b/.test(body)) {
    const m = /^\/answer\s+([a-zA-Z0-9_-]+)\s+([\s\S]+)$/.exec(body);
    return m ? { kind: "answer", question: m[1]!, answer: m[2]!.trim() } : { kind: "invalid", name: "answer" };
  }
  return { kind: "message" };
}
