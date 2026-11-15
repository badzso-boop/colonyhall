// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Schemas } from "@/shared/api";

/**
 * Where to go after logging in: only a path inside the app, so a crafted
 * link cannot send the user to another site (?next=https://evil).
 */
export function safeNext(next: string | null): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/login")) return "/";
  return next;
}

/* ---------- Profiles (round 34, part 7) ---------- */

type Actor = Schemas["Actor"];
export type ProfileUpdate = Schemas["UpdateActorProfileRequest"];

/** The server's limits (internal/modules/identity/model.go). */
export const PROFILE_LIMITS = { displayName: 80, description: 300, statusText: 80, avatarEmoji: 32 } as const;
export const DEFAULT_TIMEZONE = "Europe/Budapest";

/** The editable part of a profile, as the form holds it. */
export type ProfileDraft = { display_name: string; description: string; avatar_emoji: string; status_text: string; timezone: string };

export function draftOf(a: Actor): ProfileDraft {
  return { display_name: a.display_name, description: a.description ?? "", avatar_emoji: a.avatar_emoji ?? "", status_text: a.status_text ?? "", timezone: a.timezone || DEFAULT_TIMEZONE };
}

/** Only the changed fields go to PATCH /actors/{id}/profile; null when nothing changed. */
export function profileChanges(a: Actor, d: ProfileDraft): ProfileUpdate | null {
  const before = draftOf(a);
  const out: ProfileUpdate = {};
  for (const k of ["display_name", "description", "avatar_emoji", "status_text", "timezone"] as const) {
    const v = k === "timezone" ? d[k] : d[k].trim();
    if (v !== before[k]) out[k] = v;
  }
  return Object.keys(out).length ? out : null;
}

const runes = (s: string) => [...s].length;

/** Which fields break the server's limits (by character, as the server counts). */
export function profileErrors(d: ProfileDraft): Array<keyof ProfileDraft> {
  const bad: Array<keyof ProfileDraft> = [];
  const name = d.display_name.trim();
  if (!name || runes(name) > PROFILE_LIMITS.displayName) bad.push("display_name");
  if (runes(d.description.trim()) > PROFILE_LIMITS.description) bad.push("description");
  if (runes(d.status_text.trim()) > PROFILE_LIMITS.statusText) bad.push("status_text");
  if (runes(d.avatar_emoji.trim()) > PROFILE_LIMITS.avatarEmoji) bad.push("avatar_emoji");
  if (!validTimeZone(d.timezone)) bad.push("timezone");
  return bad;
}

export function validTimeZone(tz: string): boolean {
  if (!tz) return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: tz });
    return true;
  } catch {
    return false;
  }
}

/** The person's local time, e.g. "14:05", or null for an unknown zone. */
export function localTime(tz: string | undefined, now: Date, locale: string): string | null {
  if (!tz || !validTimeZone(tz)) return null;
  return now.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", timeZone: tz });
}

/** The time zones to choose from: the browser's list, with the current one always in it. */
export function timeZoneOptions(current: string): string[] {
  let zones: string[];
  try {
    zones = Intl.supportedValuesOf("timeZone");
  } catch {
    zones = [];
  }
  const all = new Set([DEFAULT_TIMEZONE, "UTC", ...zones]);
  if (current) all.add(current);
  return [...all].sort((a, b) => a.localeCompare(b));
}

/** Round 51: the invite token from the link's fragment (#col_…); empty when there is none. */
export function inviteToken(hash: string): string {
  return decodeURIComponent(hash.replace(/^#/, "")).trim();
}

/** The handle rule (identity.ValidateHandle): 2–32 lower-case letters, digits and '-', not starting with '-'. */
export function validOwnerHandle(h: string): boolean {
  return /^[a-z0-9][a-z0-9-]{1,31}$/.test(h);
}

export const MIN_OWNER_PASSWORD = 12;
