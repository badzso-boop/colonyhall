// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, useRef } from "react";
import { create } from "zustand";

/**
 * The one keyboard shortcut registry (docs/08, 6.; round 31 part 8). Every
 * shortcut is declared here through useShortcut; nothing else listens to
 * keydown. Keys are written like "mod+k", "?", "alt+shift+arrowdown",
 * "escape"; "mod" is Ctrl, or ⌘ on a Mac.
 */
export type Scope = "global" | "conversation" | "composer" | "pane" | "page";

export type Shortcut = {
  keys: string;
  scope: Scope;
  /** An i18n key (namespace "common", under "shortcuts."). */
  description: string;
  /** Composer-scoped shortcuts may fire inside text fields; the others never do. */
  handler?: (e: KeyboardEvent) => void;
};

type State = { list: Array<Shortcut & { id: number }> };
export const useShortcuts = create<State>(() => ({ list: [] }));

let nextId = 1;

/** "Ctrl+Shift+ArrowDown" → "mod+shift+arrowdown" (the registry's form). */
export function comboOf(e: Pick<KeyboardEvent, "key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey">): string {
  const key = e.key.toLowerCase();
  const parts: string[] = [];
  if (e.ctrlKey || e.metaKey) parts.push("mod");
  if (e.altKey) parts.push("alt");
  // "?" is already shifted; do not add shift to printable symbols.
  if (e.shiftKey && (key.length > 1 || /[a-z0-9]/.test(key))) parts.push("shift");
  parts.push(key === " " ? "space" : key);
  return parts.join("+");
}

/** Registers a shortcut; throws on a duplicate key in the same scope. Returns the unregister. */
export function registerShortcut(s: Shortcut): () => void {
  const clash = useShortcuts.getState().list.find((x) => x.keys === s.keys && x.scope === s.scope);
  if (clash) throw new Error(`shortcut ${s.keys} is already registered in scope ${s.scope}`);
  const id = nextId++;
  useShortcuts.setState((st) => ({ list: [...st.list, { ...s, id }] }));
  return () => useShortcuts.setState((st) => ({ list: st.list.filter((x) => x.id !== id) }));
}

function inTextField(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el) return false;
  return el.isContentEditable || el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT";
}

/** The single keydown listener. The newest registration of a key wins. */
export function handleKey(e: KeyboardEvent): boolean {
  const combo = comboOf(e);
  const typing = inTextField(e.target);
  const list = useShortcuts.getState().list;
  for (let i = list.length - 1; i >= 0; i--) {
    const s = list[i]!;
    if (s.keys !== combo || !s.handler) continue;
    if (typing && s.scope !== "composer" && !combo.startsWith("mod+")) continue;
    e.preventDefault();
    s.handler(e);
    return true;
  }
  return false;
}

export function installShortcuts(): () => void {
  const listener = (e: KeyboardEvent) => void handleKey(e);
  window.addEventListener("keydown", listener);
  return () => window.removeEventListener("keydown", listener);
}

/** Declares a shortcut while the component is mounted. The handler may change between renders. */
export function useShortcut(keys: string, scope: Scope, description: string, handler?: (e: KeyboardEvent) => void, enabled = true): void {
  const ref = useRef(handler);
  useEffect(() => {
    ref.current = handler;
  }, [handler]);
  const hasHandler = handler !== undefined;
  useEffect(() => {
    if (!enabled) return;
    return registerShortcut({ keys, scope, description, handler: hasHandler ? (e) => ref.current?.(e) : undefined });
  }, [keys, scope, description, enabled, hasHandler]);
}

/** How a combo is shown: ["Ctrl", "K"] (or ["⌘", "K"] on a Mac). */
export function keyLabels(keys: string, mac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform)): string[] {
  const names: Record<string, string> = { mod: mac ? "⌘" : "Ctrl", alt: mac ? "⌥" : "Alt", shift: "Shift", arrowup: "↑", arrowdown: "↓", arrowleft: "←", arrowright: "→", escape: "Esc", enter: "Enter", space: "Space" };
  return keys.split("+").map((k) => names[k] ?? k.toUpperCase());
}
