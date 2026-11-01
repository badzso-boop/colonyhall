import type { Schemas } from "@/shared/api";

export type Question = Schemas["Question"];

/**
 * "Válaszra vár": the open questions to humans and the ones escalated to
 * humans, each once, oldest first (the dev page's rule).
 */
export function waitingForHumans(open: Question[], escalated: Question[]): Question[] {
  const seen = new Set<string>();
  const out: Question[] = [];
  for (const q of [...open, ...escalated]) {
    if (seen.has(q.id) || !(q.to_human || q.state === "escalated")) continue;
    seen.add(q.id);
    out.push(q);
  }
  return out.sort((a, b) => a.created_at.localeCompare(b.created_at));
}

/** Answered questions of the last `days` days, newest answer first. */
export function answeredSince(list: Question[], now: Date, days = 7): Question[] {
  const from = now.getTime() - days * 86_400_000;
  return list
    .filter((q) => q.state === "answered" && new Date(q.answered_at ?? q.created_at).getTime() >= from)
    .sort((a, b) => (b.answered_at ?? b.created_at).localeCompare(a.answered_at ?? a.created_at));
}

/** The time left until a deadline as "1:02:03" / "4:05", or null when there is none; overdue at zero. */
export function countdown(deadline: string | null | undefined, now: Date): { text: string; overdue: boolean } | null {
  if (!deadline) return null;
  const ms = new Date(deadline).getTime() - now.getTime();
  if (Number.isNaN(ms)) return null;
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return { text: h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`, overdue: ms <= 0 };
}

/** Whether a reference (id, "Q-12", "q12", "12") names this question. */
export function isQuestion(q: Question, ref: string): boolean {
  const r = ref.trim();
  if (r === q.id) return true;
  const m = /^(?:q-?)?(\d+)$/i.exec(r);
  return m !== null && Number(m[1]) === q.number;
}

/**
 * The composer's /answer autocomplete: while the text is "/answer" and a
 * partial number, the open questions whose number starts with it.
 */
export function answerSuggestions(text: string, open: Question[]): Question[] {
  const m = /^\/answer\s+(\S*)$/.exec(text);
  if (!m) return [];
  const digits = m[1]!.replace(/^q-?/i, "");
  if (!/^\d*$/.test(digits)) return [];
  return open.filter((q) => String(q.number).startsWith(digits)).slice(0, 8);
}
