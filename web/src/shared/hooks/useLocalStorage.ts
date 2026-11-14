// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useCallback, useState } from "react";

/**
 * A per-viewer convenience kept in localStorage (docs/08, 5.): it must work
 * without storage, so every access is guarded and falls back to the default.
 */
export function useLocalStorage<T>(key: string, initial: T): [T, (v: T) => void] {
  const [value, setValue] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(key);
      return raw === null ? initial : (JSON.parse(raw) as T);
    } catch {
      return initial;
    }
  });
  const set = useCallback(
    (v: T) => {
      setValue(v);
      try {
        localStorage.setItem(key, JSON.stringify(v));
      } catch {
        // storage blocked: the value lasts until reload
      }
    },
    [key],
  );
  return [value, set];
}
