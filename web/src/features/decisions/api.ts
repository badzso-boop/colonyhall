import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/shared/api";

export function useDecisions(query: string, limit: number) {
  return useQuery({
    queryKey: ["decisions", query, limit],
    queryFn: async () => unwrap(await api.GET("/decisions", { params: { query: { limit, ...(query ? { query } : {}) } } })).decisions,
    placeholderData: keepPreviousData,
  });
}
