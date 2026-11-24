// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { describe, expect, it } from "vitest";
import { MIN_MAIN, MIN_SLOT, clampSlot, defaultSlotWidth } from "./sizes";

describe("slot sizes", () => {
  it("opens a file browser wider than a thread", () => {
    expect(defaultSlotWidth("files", "wide")).toBeGreaterThan(defaultSlotWidth("thread", "wide"));
    expect(defaultSlotWidth("files", "medium")).toBeGreaterThan(defaultSlotWidth("thread", "medium"));
  });

  it("keeps the main column its room", () => {
    expect(clampSlot(2000, 1200)).toBe(1200 - MIN_MAIN);
    expect(clampSlot(500, 1200)).toBe(500);
  });

  it("never goes below the minimum, even on a small screen", () => {
    expect(clampSlot(100, 1200)).toBe(MIN_SLOT);
    expect(clampSlot(800, 500)).toBe(MIN_SLOT);
  });
});
