// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { expect, test } from "vitest";
import type { ActivityItem } from "./api";
import { activityTarget } from "./model";

const item = (over: Partial<ActivityItem>): ActivityItem => ({ id: "a", kind: "mention", created_at: "", read: false, available: true, ...over });

test("an item leads to its message, in its thread, or nowhere once unreadable", () => {
  expect(activityTarget(item({ channel_id: "c", message_id: "m" }))).toBe("/c/c?msg=m");
  expect(activityTarget(item({ kind: "thread_reply", channel_id: "c", message_id: "m", thread_root_id: "r" }))).toBe("/c/c?thread=r&msg=m");
  expect(activityTarget(item({ kind: "approval", channel_id: "c" }))).toBe("/c/c");
  expect(activityTarget(item({ available: false, channel_id: "c", message_id: "m" }))).toBeNull();
});

test("the operator's access leads to where the owner sees and closes it (round 51b)", () => {
  expect(activityTarget({ id: "i", kind: "operator_access", created_at: "", read: false, available: false })).toBe("/admin/settings?section=uzemelteto");
});
