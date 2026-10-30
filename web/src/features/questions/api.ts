import { useQuery, type QueryClient } from "@tanstack/react-query";
import { api, onResync, onUntypedEvent, unwrap } from "@/shared/api";

export const questionKeys = { open: ["questions", "open"] as const };

/** Open questions to humans ("Válaszra vár"); the header shows their count. */
export function useOpenQuestions() {
  return useQuery({
    queryKey: questionKeys.open,
    queryFn: async () => unwrap(await api.GET("/questions", { params: { query: { state: "open" } } })).questions,
  });
}

export function registerQuestionEvents(qc: QueryClient): () => void {
  const refetch = () => void qc.invalidateQueries({ queryKey: questionKeys.open });
  const offs = ["question.asked", "question.answered", "question.escalated", "question.expired", "question.withdrawn"].map((t) =>
    onUntypedEvent(t, refetch),
  );
  offs.push(onResync(refetch));
  return () => offs.forEach((off) => off());
}
