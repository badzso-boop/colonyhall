// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect } from "react";

/** "(3) Colonyhall" and a red dot on the favicon while there are unread mentions. */
export function useDocumentTitle(mentions: number, appName: string): void {
  useEffect(() => {
    document.title = mentions > 0 ? `(${mentions}) ${appName}` : appName;
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (link) link.href = `${import.meta.env.BASE_URL}brand/${mentions > 0 ? "favicon-alert.svg" : "favicon.svg"}`;
  }, [mentions, appName]);
}
