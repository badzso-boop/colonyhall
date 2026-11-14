// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { create } from "zustand";
import { deserialize, reduce, serialize, type Viewport, type WinAction, type WindowKind, type Windows } from "./windows";

const storageKey = "colony.windows";

const viewport = (): Viewport =>
  typeof window === "undefined" ? { w: 1440, h: 900 } : { w: window.innerWidth, h: window.innerHeight };

function load(): Windows {
  let raw: string | null = null;
  try {
    raw = localStorage.getItem(storageKey);
  } catch {
    // storage blocked: the defaults
  }
  return deserialize(raw, viewport());
}

/** The floating windows' state, per viewer (localStorage), kept within the viewport. */
export const useWindows = create<{ wins: Windows }>(() => ({ wins: load() }));

export function dispatchWindow(a: WinAction): void {
  const wins = reduce(useWindows.getState().wins, a, viewport());
  useWindows.setState({ wins });
  try {
    localStorage.setItem(storageKey, serialize(wins));
  } catch {
    // storage blocked: the layout lasts until reload
  }
}

/** Pops a page out into its floating window (or brings it to the front). */
export function openWindow(kind: WindowKind): void {
  dispatchWindow({ type: "open", kind });
}

if (typeof window !== "undefined") {
  window.addEventListener("resize", () => dispatchWindow({ type: "viewport" }));
}
