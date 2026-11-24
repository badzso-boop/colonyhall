// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useCallback } from "react";
import { useSearchParams } from "react-router";

/**
 * The open preview lives in the URL (?preview=<id>), so a link, a reload
 * and Back bring it back. Opening adds a history entry; closing too, so
 * Back reopens it. Other parameters (the panes) are left alone.
 */
export function usePreviewParam(): [string | null, (id: string) => void, () => void] {
  const [params, setParams] = useSearchParams();
  const id = params.get("preview");
  const set = useCallback(
    (next: string | null) =>
      setParams((prev) => {
        const p = new URLSearchParams(prev);
        if (next) p.set("preview", next);
        else p.delete("preview");
        return p;
      }),
    [setParams],
  );
  const open = useCallback((next: string) => set(next), [set]);
  const close = useCallback(() => set(null), [set]);
  return [id, open, close];
}
