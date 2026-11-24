// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { expect, test } from "vitest";
import { messageHref, messageURL } from "./links";

test("a message link opens the conversation at the message, and its thread", () => {
  expect(messageHref({ channel_id: "c1", id: "m1" })).toBe("/c/c1?msg=m1");
  expect(messageHref({ channel_id: "c1", id: "m2", thread_root_id: "r1" })).toBe("/c/c1?thread=r1&msg=m2");
  expect(messageURL({ channel_id: "c1", id: "m1" }, "https://chat.example.com")).toBe("https://chat.example.com/agent/c/c1?msg=m1");
});
