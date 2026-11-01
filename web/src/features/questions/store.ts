import { create } from "zustand";
import type { Question } from "./model";

/**
 * The "Válaszra vár" Sheet: open or not, the question a Q-12 chip asked
 * for, and the questions answered in it while open (they stay visible,
 * answered, until it closes).
 */
export const useQuestionsSheet = create<{ open: boolean; focus: string | null; answered: Record<string, Question> }>(() => ({
  open: false,
  focus: null,
  answered: {},
}));

export const openQuestions = (focus: string | null = null) => useQuestionsSheet.setState({ open: true, focus });
export const closeQuestions = () => useQuestionsSheet.setState({ open: false, focus: null, answered: {} });
export const rememberAnswer = (q: Question) => useQuestionsSheet.setState((s) => ({ answered: { ...s.answered, [q.id]: q } }));
