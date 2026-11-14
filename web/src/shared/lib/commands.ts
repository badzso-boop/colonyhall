// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The composer's slash commands (round 55, part B; the first four came from
// the dev page). They are understood in the client and call existing APIs;
// nothing typed as a command is sent as a message. The registry is a plain
// list so later rounds can add commands with registerCommand.

/** When a /remind fires: in some minutes, or a wall-clock time some days on. */
export type RemindWhen =
  | { kind: "in"; minutes: number }
  /** days: 0 is today (tomorrow when the time has passed), "monday" the next Monday. */
  | { kind: "at"; days: number | "monday"; hour: number; minute: number };

export type NotifyLevel = "all" | "mentions" | "none";

export type Command =
  | { kind: "schedule"; body: string; at?: string; cron?: string }
  | { kind: "answer"; question: string; answer: string }
  | { kind: "remind"; when: RemindWhen; text: string }
  | { kind: "remindThis" }
  | { kind: "pin" }
  | { kind: "save" }
  | { kind: "mute" }
  | { kind: "unmute" }
  | { kind: "notify"; level: NotifyLevel }
  | { kind: "status"; text: string }
  | { kind: "stop"; handle: string }
  | { kind: "emergency" }
  | { kind: "search"; query: string }
  | { kind: "thread" }
  | { kind: "open"; view: "drafts" | "saved" | "activity" }
  | { kind: "help" }
  /** A known command with arguments it cannot use: the composer shows its usage. */
  | { kind: "invalid"; name: string }
  | { kind: "unknown"; name: string }
  /** A plain message; body is set when it differs from the text ("//x" sends "/x"). */
  | { kind: "message"; body?: string };

export type CommandSpec = {
  name: string;
  /** Shown in the menu and in the usage hint. */
  usage: string;
  /** Only admins see and run it. */
  admin?: boolean;
};

const registry: CommandSpec[] = [
  { name: "remind", usage: "/remind 20m|1h|tomorrow 9:00|monday … <text>" },
  { name: "pin", usage: "/pin" },
  { name: "save", usage: "/save" },
  { name: "mute", usage: "/mute" },
  { name: "unmute", usage: "/unmute" },
  { name: "notify", usage: "/notify all|mentions|none" },
  { name: "status", usage: "/status <text>|clear" },
  { name: "search", usage: "/search <query>" },
  { name: "thread", usage: "/thread" },
  { name: "drafts", usage: "/drafts" },
  { name: "saved", usage: "/saved" },
  { name: "activity", usage: "/activity" },
  { name: "schedule", usage: "/schedule 2026-11-02 09:00 …" },
  { name: "daily", usage: "/daily 09:00 …" },
  { name: "cron", usage: '/cron "0 9 * * 1-5" …' },
  { name: "answer", usage: "/answer Q-12 …" },
  { name: "stop", usage: "/stop @agent", admin: true },
  { name: "emergency", usage: "/emergency", admin: true },
  { name: "help", usage: "/help" },
];

/** Adds a command to the menu (its parsing is the caller's: parseCommand reports it as unknown otherwise). */
export function registerCommand(spec: CommandSpec): void {
  if (!registry.some((c) => c.name === spec.name)) registry.push(spec);
}

/** The commands this person may run, in menu order. */
export function availableCommands({ admin }: { admin: boolean }): CommandSpec[] {
  return registry.filter((c) => admin || !c.admin);
}

/** The menu for the composer's text: every command starting with what is typed after "/". */
export function matchCommands(text: string, who: { admin: boolean }): CommandSpec[] {
  const m = /^\/(\w*)$/.exec(text);
  if (!m) return [];
  const prefix = m[1]!.toLowerCase();
  return availableCommands(who).filter((c) => c.name.startsWith(prefix));
}

/** The usage line of a command, for the hint after a wrong use. */
export function commandUsage(name: string): string {
  return registry.find((c) => c.name === name)?.usage ?? `/${name}`;
}

const time = /^(\d{1,2})[:.](\d{2})$/;

/** "20m", "1h", "tomorrow", "holnap 8:30", "monday", "hétfő", "14:00" (optionally after "in"). */
export function parseWhen(words: string[]): { when: RemindWhen; used: number } | null {
  const i = words[0]?.toLowerCase() === "in" ? 1 : 0;
  const w = words[i]?.toLowerCase() ?? "";
  const dur = /^(\d{1,4})(m|min|p|perc|h|ó|óra|d|nap)$/.exec(w);
  if (dur) {
    const n = Number(dur[1]);
    const unit = dur[2]!;
    const minutes = /^(h|ó|óra)$/.test(unit) ? n * 60 : /^(d|nap)$/.test(unit) ? n * 1440 : n;
    return minutes > 0 && minutes <= 366 * 1440 ? { when: { kind: "in", minutes }, used: i + 1 } : null;
  }
  const at = (days: number | "monday") => {
    const tm = time.exec(words[i + 1] ?? "");
    const hour = tm ? Number(tm[1]) : 9;
    const minute = tm ? Number(tm[2]) : 0;
    if (hour > 23 || minute > 59) return null;
    return { when: { kind: "at" as const, days, hour, minute }, used: i + (tm ? 2 : 1) };
  };
  if (w === "tomorrow" || w === "holnap") return at(1);
  if (w === "monday" || w === "hétfő" || w === "hetfo") return at("monday");
  const tm = time.exec(w);
  if (tm && i === 0) {
    const hour = Number(tm[1]);
    const minute = Number(tm[2]);
    return hour <= 23 && minute <= 59 ? { when: { kind: "at", days: 0, hour, minute }, used: 1 } : null;
  }
  return null;
}

function parseKnown(name: string, rest: string): Command {
  const words = rest.split(/\s+/).filter(Boolean);
  switch (name) {
    case "schedule":
    case "daily":
    case "cron": {
      const body = `/${name} ${rest}`.trim();
      let m = /^\/schedule\s+(\d{4}[-.]\d{2}[-.]\d{2}\.?)\s+(\d{1,2}:\d{2})\s+([\s\S]+)$/.exec(body);
      if (m) return { kind: "schedule", at: `${m[1]!.replace(/\./g, "-").replace(/-$/, "")} ${m[2]!.padStart(5, "0")}`, body: m[3]! };
      m = /^\/daily\s+(\d{1,2}):(\d{2})\s+([\s\S]+)$/.exec(body);
      if (m) return { kind: "schedule", cron: `${+m[2]!} ${+m[1]!} * * *`, body: m[3]! };
      m = /^\/cron\s+"([^"]+)"\s+([\s\S]+)$/.exec(body);
      if (m) return { kind: "schedule", cron: m[1]!, body: m[2]! };
      return { kind: "invalid", name: "schedule" };
    }
    case "answer": {
      const m = /^([a-zA-Z0-9_-]+)\s+([\s\S]+)$/.exec(rest);
      return m ? { kind: "answer", question: m[1]!, answer: m[2]!.trim() } : { kind: "invalid", name: "answer" };
    }
    case "remind": {
      if (/^me\s+about\s+this$/i.test(rest) || /^(this|ezt)$/i.test(rest)) return { kind: "remindThis" };
      const w = parseWhen(words);
      if (!w) return { kind: "invalid", name };
      // The text keeps its own spacing and lines: cut the time words off the front.
      let text = rest;
      for (let k = 0; k < w.used; k++) text = text.replace(/^\s*\S+/, "");
      text = text.trim();
      return text ? { kind: "remind", when: w.when, text } : { kind: "invalid", name };
    }
    case "pin":
    case "save":
    case "mute":
    case "unmute":
    case "emergency":
    case "thread":
    case "help":
      return words.length ? { kind: "invalid", name } : { kind: name };
    case "drafts":
    case "saved":
    case "activity":
      return words.length ? { kind: "invalid", name } : { kind: "open", view: name };
    case "notify": {
      const level = words[0]?.toLowerCase();
      return words.length === 1 && (level === "all" || level === "mentions" || level === "none") ? { kind: "notify", level } : { kind: "invalid", name };
    }
    case "status":
      if (!rest) return { kind: "invalid", name };
      return { kind: "status", text: /^clear$/i.test(rest) ? "" : rest };
    case "stop": {
      const m = /^@?([\w.-]+)$/.exec(rest);
      return m ? { kind: "stop", handle: m[1]! } : { kind: "invalid", name };
    }
    case "search":
      return rest ? { kind: "search", query: rest } : { kind: "invalid", name };
  }
  return { kind: "unknown", name };
}

/**
 * Reads the composer's text. "/word" followed by a space or the end is a
 * command (an unknown one is refused, never sent); "//text" sends "/text";
 * anything else, "/tmp/x is full" too, is a message.
 */
export function parseCommand(text: string): Command {
  const body = text.trim();
  if (body.startsWith("//")) return { kind: "message", body: body.slice(1) };
  const m = /^\/([A-Za-z]+)(?:\s+([\s\S]*))?$/.exec(body);
  if (!m) return { kind: "message" };
  const name = m[1]!.toLowerCase();
  const rest = (m[2] ?? "").trim();
  return parseKnown(name, rest);
}
