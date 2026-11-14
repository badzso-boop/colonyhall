// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/shared/api";
import type { SearchMode } from "./model";

export const SEARCH_LIMIT = 20;

export const searchKeys = {
  all: ["search"] as const,
  query: (q: string, mode: SearchMode) => ["search", q, mode] as const,
};

/** One search (GET /search) over every source kind; idle while the query is empty. */
export function useSearch(q: string, mode: SearchMode) {
  return useQuery({
    queryKey: searchKeys.query(q, mode),
    enabled: q.length > 0,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
    queryFn: async () => unwrap(await api.GET("/search", { params: { query: { q, mode, limit: SEARCH_LIMIT } } })),
  });
}
