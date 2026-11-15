// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Schemas } from "@/shared/api";

// The platform operator (round 51, docs/24-hosztolt-ajanlat.md): the
// console on admin.<domain>. Pure rules only.

export type Customer = Schemas["Customer"];
export type OperatorSession = Schemas["OperatorSessionInfo"];
export type AuditEntry = Schemas["OperatorAuditEntry"];

/** The operator console lives on admin.<domain>; every other host is a workspace. */
export function isOperatorHost(hostname: string): boolean {
  return hostname.toLowerCase().startsWith("admin.");
}

export type InviteStatus = "owner" | "none" | "open" | "expired" | "used" | "revoked";

/** Where a customer's owner stands: already there, or the newest invite's state. */
export function inviteStatus(c: Pick<Customer, "has_owner" | "invite">, now: Date): InviteStatus {
  if (c.has_owner) return "owner";
  const inv = c.invite;
  if (!inv) return "none";
  if (inv.used) return "used";
  if (inv.revoked) return "revoked";
  return new Date(inv.expires_at).getTime() <= now.getTime() ? "expired" : "open";
}

/** A new invite can be made while the workspace has no owner. */
export function canInvite(c: Pick<Customer, "has_owner">): boolean {
  return !c.has_owner;
}

/** The storage use in percent of the quota, or null without a quota. */
export function storagePercent(s: Customer["storage"]): number | null {
  if (s.quota_bytes <= 0) return null;
  return Math.min(100, Math.round((s.used_bytes / s.quota_bytes) * 100));
}

/** The server's subdomain rule (identity.ValidSlug): 3–40 characters, a-z, 0-9 and single hyphens, not at the ends. */
const slugRe = /^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$/;
export const reservedSlugs = new Set([
  "admin", "api", "app", "www", "mail", "smtp", "imap", "pop", "ftp", "ns1", "ns2", "static", "assets", "cdn", "status", "docs",
  "help", "support", "billing", "auth", "login", "sso", "id", "operator", "root", "system", "localhost", "test", "dev",
]);

export function validSlug(s: string): boolean {
  return slugRe.test(s) && !s.includes("--") && !reservedSlugs.has(s);
}

/** A subdomain suggestion from a customer's name (accents dropped). */
export function slugFromName(name: string): string {
  return name
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 40)
    .replace(/-$/, "");
}

/** The audit entries' workspaces by id, to show slugs instead of ids. */
export function slugsById(list: Pick<Customer, "id" | "slug">[]): Map<string, string> {
  return new Map(list.map((c) => [c.id, c.slug]));
}

/** A 6-digit TOTP code or a backup code (XXXXX-XXXXX); spaces dropped. */
export function normaliseCode(code: string): string {
  return code.replace(/\s+/g, "");
}

// Round 51b: plans.

const GiB = 1024 ** 3;

/** "15" or "15,5" GB → bytes; null when it is not a positive number. */
export function gbToBytes(text: string): number | null {
  const s = text.trim().replace(",", ".");
  if (!/^\d+(\.\d+)?$/.test(s)) return null;
  const n = Number(s);
  if (!Number.isFinite(n) || n <= 0 || n > 1024 * 1024) return null;
  return Math.round(n * GiB);
}

/** Bytes → a GB figure for an input ("15", "15.5"). */
export function bytesToGb(n: number): string {
  return String(+(n / GiB).toFixed(2));
}

/** A whole number of CPU cores, 1–1024; null otherwise. */
export function parseCores(text: string): number | null {
  const s = text.trim();
  if (!/^\d+$/.test(s)) return null;
  const n = Number(s);
  return n >= 1 && n <= 1024 ? n : null;
}

/** Break-glass limits (operator.BreakGlassMinReason …). */
export const breakGlassMinReason = 20;
export const breakGlassMaxMinutes = 240;
export const breakGlassDevMaxMinutes = 720;

/** Whether a break-glass request may be sent: a reason of 20+ characters (any in development mode), 1–240 minutes (720). */
export function breakGlassReady(reason: string, minutes: number, dev: boolean): boolean {
  const max = dev ? breakGlassDevMaxMinutes : breakGlassMaxMinutes;
  return Number.isInteger(minutes) && minutes >= 1 && minutes <= max && (dev || reason.trim().length >= breakGlassMinReason);
}
