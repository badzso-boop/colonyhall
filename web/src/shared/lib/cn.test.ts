// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { cn } from "./cn";

test("cn skips falsy parts", () => {
  expect(cn("a", false, null, undefined, "b")).toBe("a b");
  expect(cn()).toBe("");
});

test("a later Tailwind class overrides an earlier conflicting one", () => {
  expect(cn("w-full h-8", "w-36")).toBe("h-8 w-36");
  expect(cn("text-ui", "text-meta")).toBe("text-meta");
  expect(cn("bg-surface", "bg-accent")).toBe("bg-accent");
});

test("a type-scale size and a text colour are kept together", () => {
  expect(cn("text-ui text-fg-muted")).toBe("text-ui text-fg-muted");
  expect(cn("text-body text-accent-text", "text-meta")).toBe("text-accent-text text-meta");
  expect(cn("text-fg", "text-fg-muted")).toBe("text-fg-muted");
});
