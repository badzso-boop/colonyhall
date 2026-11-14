// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useSyncExternalStore } from "react";
import type { Width } from "@/shared/panes";

// The width classes of the pane rules (round 31, part 4–5).
const WIDE = "(min-width: 1280px)";
const MEDIUM = "(min-width: 768px)";

function current(): Width {
  if (typeof window === "undefined" || !window.matchMedia) return "wide";
  if (window.matchMedia(WIDE).matches) return "wide";
  return window.matchMedia(MEDIUM).matches ? "medium" : "narrow";
}

function subscribe(cb: () => void): () => void {
  if (typeof window === "undefined" || !window.matchMedia) return () => {};
  const lists = [window.matchMedia(WIDE), window.matchMedia(MEDIUM)];
  lists.forEach((l) => l.addEventListener("change", cb));
  return () => lists.forEach((l) => l.removeEventListener("change", cb));
}

export function useWidth(): Width {
  return useSyncExternalStore(subscribe, current, () => "wide");
}
