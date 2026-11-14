// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Schemas } from "@/shared/api";

type Host = Schemas["Host"];
type Actor = Schemas["Actor"];

/** A guard counts as online when the machine was seen within this window. */
export const OnlineMs = 90_000;

export function isOnline(host: Pick<Host, "last_seen_at">, now: Date): boolean {
  if (!host.last_seen_at) return false;
  const seen = new Date(host.last_seen_at).getTime();
  return !Number.isNaN(seen) && now.getTime() - seen < OnlineMs;
}

export type HostWarning =
  | { kind: "revoked" }
  | { kind: "rejected"; count: number }
  | { kind: "exchange"; dir: string }
  | { kind: "oldGuard"; version: string };

/**
 * What needs the admin's attention on a machine. The old-guard check needs
 * the server's version, which the API does not expose yet: it only runs when
 * one is given.
 */
export function hostWarnings(host: Host, serverVersion?: string): HostWarning[] {
  const out: HostWarning[] = [];
  if (host.state === "revoked") out.push({ kind: "revoked" });
  const rejected = host.report.rejected?.length ?? 0;
  if (rejected) out.push({ kind: "rejected", count: rejected });
  if (host.report.exchange_dir && !host.report.exchange_ok) out.push({ kind: "exchange", dir: host.report.exchange_dir });
  const gv = host.report.guard_version;
  if (serverVersion && gv && compareVersions(gv, serverVersion) < 0) out.push({ kind: "oldGuard", version: gv });
  return out;
}

/** Compares dotted versions ("v1.2.10" > "1.2.9"); non-numeric parts compare as 0. */
export function compareVersions(a: string, b: string): number {
  const parts = (v: string) => v.replace(/^v/, "").split(/[.+-]/).map((p) => Number.parseInt(p, 10) || 0);
  const pa = parts(a);
  const pb = parts(b);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d) return Math.sign(d);
  }
  return 0;
}

/** One agent with a folder grant. */
export type GrantHolder = { agentId: string; handle: string; mode: "read" | "write" };

/** Folder path → the agents with a grant on exactly that folder, by handle. */
export function grantsByPath(host: Pick<Host, "agents">): Map<string, GrantHolder[]> {
  const out = new Map<string, GrantHolder[]>();
  for (const a of host.agents) {
    for (const g of a.grants) {
      const list = out.get(g.path) ?? [];
      list.push({ agentId: a.agent_id, handle: a.handle, mode: g.mode });
      out.set(g.path, list);
    }
  }
  for (const list of out.values()) list.sort((x, y) => x.handle.localeCompare(y.handle));
  return new Map([...out.entries()].sort((x, y) => x[0].localeCompare(y[0])));
}

/**
 * The colour an agent shows: its own, else the machine's. `hostColor` lets
 * the settings tab preview a colour that is not saved yet.
 */
export function agentColor(actor: Pick<Actor, "color"> | undefined, hostColor: string): string {
  return actor?.color || hostColor;
}

/** A shell-safe word: quoted when it has anything but plain characters. */
function shellWord(s: string): string {
  return /^[\w@%+=:,./-]+$/.test(s) ? s : `'${s.replaceAll("'", `'\\''`)}'`;
}

/** The command a machine runs to enroll with a pairing code. */
export function enrollCommand(server: string, code: string, name: string): string {
  return `colony guard enroll -server ${shellWord(server)} -code ${shellWord(code)} -name ${shellWord(name.trim() || "gep")}`;
}

/** Where the releases come from; while the repository is private the
 * installer is fetched with the GitHub CLI (Norbi, 2026-11-05). */
export const releaseRepo = "badzso-boop/colonyhall";
export const releasesPrivate = true;

export type InstallPlatform = "linux" | "wsl" | "macos";

/**
 * The one-line install for a new machine: install.sh downloads and verifies
 * colony, then `colony setup --guard` pairs it with this code.
 */
export function installCommand(server: string, code: string, name: string): string {
  const setup = `--guard --server-url ${shellWord(server)} --pair-code ${shellWord(code)} --name ${shellWord(name.trim() || "gep")}`;
  return releasesPrivate
    ? `gh release download -R ${releaseRepo} -p install.sh -O - | sh -s -- ${setup}`
    : `curl -fsSL https://github.com/${releaseRepo}/releases/latest/download/install.sh | sh -s -- ${setup}`;
}

/** Seconds until `expiresAt` (never negative). */
export function secondsLeft(expiresAt: string, now: Date): number {
  return Math.max(0, Math.floor((new Date(expiresAt).getTime() - now.getTime()) / 1000));
}

/** 125 → "2:05". */
export function mmss(secs: number): string {
  return `${Math.floor(secs / 60)}:${String(secs % 60).padStart(2, "0")}`;
}

/**
 * The host the wizard waits for: one that was not there when the code was
 * asked for, preferring the name the wizard suggested.
 */
export function newlyEnrolled(hosts: Host[], known: ReadonlySet<string>, name: string): Host | null {
  const fresh = hosts.filter((h) => !known.has(h.id));
  return fresh.find((h) => h.name === name.trim()) ?? fresh[0] ?? null;
}

/** A minimal guard.yaml for a freshly enrolled machine. */
export function guardYaml(server: string): string {
  return `server: ${server}\nhost_token_file: ~/.config/colonyhall/host.token\nroots:\n  - /home/me/Projects\n`;
}
