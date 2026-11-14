// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useCallback } from "react";
import { useSearchParams } from "react-router";
import type { SettingsTab } from "./model";

/**
 * Opens the agent settings dialog on a tab. The dialog follows the URL
 * (?agent=<handle>&settings=<tab>, docs/08 5.: shareable state lives in the
 * URL), so this only sets the two params and keeps the others.
 */
export function useOpenSettings(handle: string): (tab: SettingsTab) => void {
  const [, setParams] = useSearchParams();
  return useCallback(
    (tab: SettingsTab) =>
      setParams((prev) => {
        const next = new URLSearchParams(prev);
        next.set("agent", handle);
        next.set("settings", tab);
        return next;
      }),
    [handle, setParams],
  );
}
