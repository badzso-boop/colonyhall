// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// @ autocomplete ranking, ported from the dev page's mention.js (round 26).

export type MentionCandidate = { id: string; handle: string; display_name: string; halted?: boolean };

export function fuzzyScore(target: string, query: string): number {
  if (!query) return 1;
  if (!target) return 0;
  const t = target.toLowerCase();
  const q = query.toLowerCase();
  if (t === q) return 1200;
  if (t.startsWith(q)) return 1000 + (100 - Math.min(100, t.length));
  const escaped = q.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  if (new RegExp(`(?:^|[\\s_\\-.])${escaped}`, "i").test(t)) return 800 + (100 - Math.min(100, t.length));
  let ti = 0;
  let qi = 0;
  let gaps = 0;
  while (ti < t.length && qi < q.length) {
    if (t[ti] === q[qi]) qi++;
    else gaps++;
    ti++;
  }
  return qi === q.length ? 400 - Math.min(200, gaps) : 0;
}

/** Up to 8 actors for "@query": channel members first, halted ones left out. */
export function rankMentions<A extends MentionCandidate>(query: string, actors: A[], memberIds: Set<string>): A[] {
  const q = query.trim().toLowerCase();
  const scored: Array<{ a: A; score: number }> = [];
  for (const a of actors) {
    if (a.halted) continue;
    const member = memberIds.has(a.id) ? 10000 : 0;
    if (!q) {
      scored.push({ a, score: member });
      continue;
    }
    const base = Math.max(fuzzyScore(a.handle, q), fuzzyScore(a.display_name, q));
    if (base > 0) scored.push({ a, score: member + base });
  }
  scored.sort((x, y) => y.score - x.score || x.a.handle.localeCompare(y.a.handle));
  return scored.slice(0, 8).map((s) => s.a);
}

/** The "@word" being typed at the caret, if any: its start index and the query. */
export function activeMention(text: string, caret: number): { start: number; query: string } | null {
  const before = text.slice(0, caret);
  const m = /(^|\s)@([\w.-]*)$/.exec(before);
  if (!m) return null;
  return { start: caret - m[2]!.length - 1, query: m[2]! };
}

/** Replaces the mention at the caret with "@handle ", returning the new text and caret. */
export function insertMention(text: string, caret: number, handle: string): { text: string; caret: number } {
  const at = activeMention(text, caret);
  if (!at) return { text, caret };
  const insert = `@${handle} `;
  return { text: text.slice(0, at.start) + insert + text.slice(caret), caret: at.start + insert.length };
}
