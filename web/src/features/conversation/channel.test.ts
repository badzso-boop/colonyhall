// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { isMuted, muteUntil, notifyLevel, orderMembers, rowBadge, validChannelName } from "./channel";
import type { Actor, Channel } from "./model";

test("channel names follow the server's rule", () => {
  expect(validChannelName("ops-team")).toBe(true);
  expect(validChannelName("a")).toBe(false);
  expect(validChannelName("Ops")).toBe(false);
  expect(validChannelName("dm-x")).toBe(false);
  expect(validChannelName("-x")).toBe(false);
});

test("mute durations", () => {
  const now = new Date(2026, 10, 1, 21, 30);
  expect(muteUntil("1h", now)).toBe(new Date(2026, 10, 1, 22, 30).toISOString());
  expect(muteUntil("tomorrow", now)).toBe(new Date(2026, 10, 2, 8, 0).toISOString());
  expect(muteUntil("forever", now)).toBeNull();
});

test("an expired mute reads as unmuted", () => {
  const ch = { muted: true, muted_until: "2026-11-01T10:00:00Z" } as Channel;
  expect(isMuted(ch, new Date("2026-11-01T09:00:00Z"))).toBe(true);
  expect(isMuted(ch, new Date("2026-11-01T11:00:00Z"))).toBe(false);
  expect(isMuted({ muted: true } as Channel, new Date())).toBe(true);
  expect(isMuted({} as Channel, new Date())).toBe(false);
});

test("members: humans first, agents by colour", () => {
  const a = (handle: string, kind: "human" | "agent", effective_color?: string) => ({ actor: { handle, kind, effective_color, display_name: "" } as unknown as Actor });
  const order = orderMembers([a("zed", "agent", "teal"), a("bob", "human"), a("amy", "agent", "teal"), a("cat", "agent", "amber"), a("ann", "human")]);
  expect(order.map((r) => r.actor.handle)).toEqual(["ann", "bob", "cat", "amy", "zed"]);
});

describe("notification level and badge (round 55, part I)", () => {
  const now = new Date("2026-11-14T10:00:00Z");
  const base = { id: "c", name: "x", is_member: true, unread: 5, mentions: 1 } as const;
  const ch = (over: Record<string, unknown>) => ({ ...base, kind: "public", ...over }) as Parameters<typeof notifyLevel>[0];

  it("defaults to mentions in a channel and all in a DM", () => {
    expect(notifyLevel(ch({}), now)).toBe("mentions");
    expect(notifyLevel(ch({ kind: "dm" }), now)).toBe("all");
  });

  it("follows the choice; a mute is none, an expired one the default", () => {
    expect(notifyLevel(ch({ notify_level: "all" }), now)).toBe("all");
    expect(notifyLevel(ch({ muted: true, notify_level: "none" }), now)).toBe("none");
    expect(notifyLevel(ch({ muted: true, muted_until: "2026-11-14T09:00:00Z", notify_level: "none" }), now)).toBe("mentions");
  });

  it("counts only what would notify; a muted row keeps its mention badge but is not bold", () => {
    expect(rowBadge(ch({ notify_level: "all" }), now)).toEqual({ bold: true, count: 5 });
    expect(rowBadge(ch({}), now)).toEqual({ bold: true, count: 1 });
    expect(rowBadge(ch({ muted: true, notify_level: "none" }), now)).toEqual({ bold: false, count: 1 });
    expect(rowBadge(ch({ unread: 0, mentions: 0 }), now)).toEqual({ bold: false, count: 0 });
  });
});
