import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/shared/api";

export function useDecisions(query: string, limit: number) {
  return useQuery({
    queryKey: ["decisions", query, limit],
    queryFn: async () => unwrap(await api.GET("/decisions", { params: { query: { limit, ...(query ? { query } : {}) } } })).decisions,
    placeholderData: keepPreviousData,
  });
}

/** How many decisions a D-12 chip looks through: the API has no lookup by number. */
const CHIP_LIMIT = 1000;

/** One decision by its number ("D-12"), for a chip; null when there is none. */
export function useDecision(number: string, enabled: boolean) {
  return useQuery({
    queryKey: ["decisions", "chip"],
    enabled,
    staleTime: 60_000,
    queryFn: async () => unwrap(await api.GET("/decisions", { params: { query: { limit: CHIP_LIMIT } } })).decisions,
    select: (list) => list.find((d) => d.formatted_number === number) ?? null,
  });
}
