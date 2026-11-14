import type { QueryClient } from "@tanstack/react-query";
import { onEvent, onResync } from "@/shared/api";
import { activityKeys } from "./api";

/** The viewer's feed grew or was read elsewhere: the list and the badge refetch. */
export function registerActivityEvents(qc: QueryClient): () => void {
  const refresh = () => void qc.invalidateQueries({ queryKey: activityKeys.all });
  const offs = [onEvent("activity.created", refresh), onEvent("activity.read", refresh), onResync(refresh)];
  return () => offs.forEach((off) => off());
}
