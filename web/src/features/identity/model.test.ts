import type { Actor } from "./api";
import { draftOf, localTime, profileChanges, profileErrors, safeNext, timeZoneOptions } from "./model";

test("safeNext keeps in-app paths and refuses everything else", () => {
  expect(safeNext("/c/123?thread=9")).toBe("/c/123?thread=9");
  expect(safeNext(null)).toBe("/");
  expect(safeNext("https://evil.example")).toBe("/");
  expect(safeNext("//evil.example")).toBe("/");
  expect(safeNext("/login")).toBe("/");
});

const actor = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "admin", state: "active", created_at: "", status_text: "Itthon", timezone: "Europe/Budapest" } as Actor;

test("profileChanges sends only what changed, trimmed", () => {
  const d = draftOf(actor);
  expect(d).toEqual({ description: "", avatar_emoji: "", status_text: "Itthon", timezone: "Europe/Budapest" });
  expect(profileChanges(actor, d)).toBeNull();
  expect(profileChanges(actor, { ...d, status_text: "  Itthon " })).toBeNull();
  expect(profileChanges(actor, { ...d, avatar_emoji: "🦊", timezone: "UTC" })).toEqual({ avatar_emoji: "🦊", timezone: "UTC" });
  // Clearing a field is a change too.
  expect(profileChanges(actor, { ...d, status_text: "" })).toEqual({ status_text: "" });
  // No zone stored yet: the server's default counts as the current value.
  expect(draftOf({ ...actor, timezone: undefined }).timezone).toBe("Europe/Budapest");
});

test("profileErrors follows the server's limits", () => {
  const d = draftOf(actor);
  expect(profileErrors(d)).toEqual([]);
  expect(profileErrors({ ...d, status_text: "é".repeat(80) })).toEqual([]);
  expect(profileErrors({ ...d, status_text: "é".repeat(81), description: "x".repeat(301), timezone: "Mars/Olympus" })).toEqual(["description", "status_text", "timezone"]);
});

test("localTime uses the person's zone", () => {
  const now = new Date("2026-11-01T12:00:00Z");
  expect(localTime("Europe/Budapest", now, "hu-HU")).toBe("13:00");
  expect(localTime("Asia/Tokyo", now, "hu-HU")).toBe("21:00");
  expect(localTime("Nowhere/Land", now, "hu-HU")).toBeNull();
  expect(localTime(undefined, now, "hu-HU")).toBeNull();
});

test("timeZoneOptions always holds the current zone", () => {
  const list = timeZoneOptions("Etc/GMT+5");
  expect(list).toContain("Etc/GMT+5");
  expect(list).toContain("Europe/Budapest");
  expect(new Set(list).size).toBe(list.length);
});
