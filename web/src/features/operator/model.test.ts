// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { breakGlassReady, bytesToGb, canInvite, gbToBytes, inviteStatus, isOperatorHost, normaliseCode, parseCores, slugFromName, slugsById, storagePercent, validSlug } from "./model";

const now = new Date("2026-11-15T10:00:00Z");

test("the operator console is on admin.<domain> only", () => {
  expect(isOperatorHost("admin.colonyhall.example.com")).toBe(true);
  expect(isOperatorHost("ADMIN.localhost")).toBe(true);
  expect(isOperatorHost("acme.colonyhall.example.com")).toBe(false);
  expect(isOperatorHost("administrator.example.com")).toBe(false);
  expect(isOperatorHost("localhost")).toBe(false);
});

test("the invite status of a customer", () => {
  const inv = (x: Partial<{ used: boolean; revoked: boolean; expires_at: string }>) => ({
    created_at: "2026-11-10T10:00:00Z", expires_at: "2026-11-17T10:00:00Z", used: false, revoked: false, ...x,
  });
  expect(inviteStatus({ has_owner: true, invite: inv({ used: true }) }, now)).toBe("owner");
  expect(inviteStatus({ has_owner: false }, now)).toBe("none");
  expect(inviteStatus({ has_owner: false, invite: inv({}) }, now)).toBe("open");
  expect(inviteStatus({ has_owner: false, invite: inv({ expires_at: "2026-11-15T10:00:00Z" }) }, now)).toBe("expired");
  expect(inviteStatus({ has_owner: false, invite: inv({ revoked: true }) }, now)).toBe("revoked");
  expect(inviteStatus({ has_owner: false, invite: inv({ used: true }) }, now)).toBe("used");
  expect(canInvite({ has_owner: true })).toBe(false);
  expect(canInvite({ has_owner: false })).toBe(true);
});

test("storage percent", () => {
  const s = { used_bytes: 0, quota_bytes: 0, files_bytes: 0, db_bytes: 0, state: "ok" as const };
  expect(storagePercent(s)).toBeNull();
  expect(storagePercent({ ...s, used_bytes: 15, quota_bytes: 20 })).toBe(75);
  expect(storagePercent({ ...s, used_bytes: 30, quota_bytes: 20 })).toBe(100);
});

test("subdomains follow the server's rule", () => {
  for (const ok of ["acme", "gamma-kft", "a1b", "x".repeat(40)]) expect(validSlug(ok)).toBe(true);
  for (const bad of ["ab", "-acme", "acme-", "a--b", "Acme", "admin", "www", "x".repeat(41), "ac me"]) expect(validSlug(bad)).toBe(false);
  expect(slugFromName("Gamma Kft.")).toBe("gamma-kft");
  expect(slugFromName("Árvíztűrő Tükörfúró Zrt")).toBe("arvizturo-tukorfuro-zrt");
  expect(slugFromName("  --  ")).toBe("");
  expect(slugFromName("a".repeat(39) + " b")).toBe("a".repeat(39));
});

test("helpers", () => {
  expect(slugsById([{ id: "1", slug: "acme" }]).get("1")).toBe("acme");
  expect(normaliseCode(" 123 456 ")).toBe("123456");
});

test("plan numbers: GB and cores (round 51b)", () => {
  expect(gbToBytes("15")).toBe(15 * 1024 ** 3);
  expect(gbToBytes("1,5")).toBe(Math.round(1.5 * 1024 ** 3));
  expect(gbToBytes("0")).toBeNull();
  expect(gbToBytes("x")).toBeNull();
  expect(bytesToGb(20 * 1024 ** 3)).toBe("20");
  expect(parseCores("4")).toBe(4);
  expect(parseCores("0")).toBeNull();
  expect(parseCores("2.5")).toBeNull();
});

test("break-glass needs a reason and at most 4 hours, unless in development mode (round 51b)", () => {
  const reason = "Hibakeresés: nem tölt be a csatorna";
  expect(breakGlassReady(reason, 60, false)).toBe(true);
  expect(breakGlassReady("rövid", 60, false)).toBe(false);
  expect(breakGlassReady(reason, 300, false)).toBe(false);
  expect(breakGlassReady("", 300, true)).toBe(true);
  expect(breakGlassReady("", 0, true)).toBe(false);
});
