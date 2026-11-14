// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient } from "@tanstack/react-query";

// One client for the app. Live updates come from the WebSocket (features'
// events.ts update the cache), so data stays fresh without polling.
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { staleTime: 30_000, refetchOnWindowFocus: false, retry: 1 },
    },
  });
}
