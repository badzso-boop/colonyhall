// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

export const themes = ["system", "light", "dark"] as const;
export type Theme = (typeof themes)[number];

// Keep in sync with the inline script in index.html.
const storageKey = "colony.theme";

export function readTheme(): Theme {
  try {
    const v = localStorage.getItem(storageKey);
    if (v === "light" || v === "dark") return v;
  } catch {
    // storage blocked: use the system setting
  }
  return "system";
}

export function isDark(theme: Theme, systemDark: boolean): boolean {
  return theme === "dark" || (theme === "system" && systemDark);
}

export function applyTheme(theme: Theme): void {
  try {
    if (theme === "system") localStorage.removeItem(storageKey);
    else localStorage.setItem(storageKey, theme);
  } catch {
    // storage blocked: the choice lasts until reload
  }
  const systemDark = window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
  document.documentElement.classList.toggle("dark", isDark(theme, systemDark));
}
