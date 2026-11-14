import { latestMessage, remindAt } from "./slash";
import type { MessagePages } from "./api";

const msg = (id: string, author: string, extra: object = {}) => ({ id, author_id: author, channel_id: "c", kind: "text", body: id, created_at: "2026-11-14T10:00:00Z", ...extra });

test("latestMessage looks from the newest page down and skips retracted and system messages", () => {
  const pages = {
    pages: [
      [msg("m3", "me"), msg("m4", "bob"), msg("m5", "me", { deleted_at: "2026-11-14T10:01:00Z" }), msg("m6", "sys", { kind: "system" })],
      [msg("m1", "me"), msg("m2", "bob")],
    ],
    pageParams: [undefined, "m3"],
  } as unknown as MessagePages;
  expect(latestMessage(pages, () => true)?.id).toBe("m4");
  expect(latestMessage(pages, (m) => m.author_id === "me")?.id).toBe("m3");
  expect(latestMessage(pages, (m) => m.author_id === "carol")).toBeUndefined();
  expect(latestMessage(undefined, () => true)).toBeUndefined();
});

test("remindAt in the person's zone", () => {
  // Saturday 2026-11-14 15:30 in Budapest (UTC+1).
  const now = new Date("2026-11-14T14:30:00Z");
  const tz = "Europe/Budapest";
  expect(remindAt({ kind: "in", minutes: 20 }, now, tz).toISOString()).toBe("2026-11-14T14:50:00.000Z");
  expect(remindAt({ kind: "at", days: 1, hour: 9, minute: 0 }, now, tz).toISOString()).toBe("2026-11-15T08:00:00.000Z");
  expect(remindAt({ kind: "at", days: "monday", hour: 9, minute: 0 }, now, tz).toISOString()).toBe("2026-11-16T08:00:00.000Z");
  // A time later today stays today; one already past moves to tomorrow.
  expect(remindAt({ kind: "at", days: 0, hour: 18, minute: 0 }, now, tz).toISOString()).toBe("2026-11-14T17:00:00.000Z");
  expect(remindAt({ kind: "at", days: 0, hour: 8, minute: 0 }, now, tz).toISOString()).toBe("2026-11-15T07:00:00.000Z");
});
