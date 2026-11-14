// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { isMuted, muteUntil, orderMembers, validChannelName } from "./channel";
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
