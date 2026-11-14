// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, onResync, onUntypedEvent, unwrap } from "@/shared/api";
import { waitingForHumans, type Question } from "./model";

export const questionKeys = {
  all: ["questions"] as const,
  open: ["questions", "open"] as const,
  answered: ["questions", "answered"] as const,
};

async function list(state: string): Promise<Question[]> {
  return unwrap(await api.GET("/questions", { params: { query: { state } } })).questions;
}

/** "Válaszra vár": open questions to humans and escalated ones; the header counts them. */
export function useOpenQuestions() {
  return useQuery({
    queryKey: questionKeys.open,
    queryFn: async () => {
      const [open, escalated] = await Promise.all([list("open"), list("escalated")]);
      return waitingForHumans(open, escalated);
    },
  });
}

/** Answered questions (the Sheet's second tab keeps the last 7 days). */
export function useAnsweredQuestions(enabled: boolean) {
  return useQuery({ queryKey: questionKeys.answered, enabled, queryFn: () => list("answered") });
}

export function registerQuestionEvents(qc: QueryClient): () => void {
  const refetch = () => void qc.invalidateQueries({ queryKey: questionKeys.all });
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
    onSuccess: () => void qc.invalidateQueries({ queryKey: questionKeys.all }),
  });
}
