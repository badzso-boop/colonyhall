// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { expect, test } from "vitest";
import { forwardTargets, needsLeakConfirm } from "./forward";
import type { Channel } from "./model";

const ch = (id: string, kind: Channel["kind"], extra: Partial<Channel> = {}): Channel =>
  ({ id, kind, name: id, is_member: true, unread: 0, mentions: 0, ...extra }) as Channel;

test("targets are the viewer's conversations that are not archived", () => {
  const list = [ch("a", "public"), ch("b", "private", { is_member: false }), ch("c", "public", { archived_at: "2026-01-01T00:00:00Z" })];
  expect(forwardTargets(list).map((c) => c.id)).toEqual(["a"]);
});

test("forwarding out of a private channel or a DM asks first", () => {
  expect(needsLeakConfirm(ch("p", "private"), "x")).toBe(true);
  expect(needsLeakConfirm(ch("d", "dm"), "x")).toBe(true);
  expect(needsLeakConfirm(ch("p", "private"), "p")).toBe(false);
  expect(needsLeakConfirm(ch("g", "public"), "x")).toBe(false);
});
