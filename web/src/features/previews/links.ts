// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect } from "react";
import { registerLinkHandler } from "@/shared/lib";
import { usePreviews } from "./api";
import { previewForLink } from "./model";
import { usePreviewParam } from "./pane";

/**
 * A preview's address in a message opens the embedded preview instead of
 * a new tab (round 57). Mounted once, by the frame.
 */
export function usePreviewLinks(): void {
  const previews = usePreviews();
  const [, open] = usePreviewParam();
  const list = previews.data;
  useEffect(() => {
    if (!list?.length) return;
    return registerLinkHandler((href) => {
      const p = previewForLink(list, href);
      if (!p?.proxy_url) return false;
      open(p.id);
      return true;
    });
  }, [list, open]);
}
