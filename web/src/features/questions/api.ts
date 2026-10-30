import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
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

export function useAnswerQuestion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, answer }: { id: string; answer: string }) =>
      unwrap(await api.POST("/questions/{questionId}/answer", { params: { path: { questionId: id } }, body: { answer } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: questionKeys.open }),
  });
}
