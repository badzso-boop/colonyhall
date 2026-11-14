// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

/**
 * The parts of `text` a query matched, for highlighting search hits: the
 * first case-insensitive substring if there is one, otherwise the letters
 * of a subsequence match (as fuzzyScore accepts it). Empty when no match.
 */
export function matchRanges(text: string, query: string): [number, number][] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  const t = text.toLowerCase();
  const at = t.indexOf(q);
  if (at >= 0) return [[at, at + q.length]];
  const out: [number, number][] = [];
  let qi = 0;
  for (let i = 0; i < t.length && qi < q.length; i++) {
    if (t[i] !== q[qi]) continue;
    qi++;
    const last = out[out.length - 1];
    if (last && last[1] === i) last[1] = i + 1;
    else out.push([i, i + 1]);
  }
  return qi === q.length ? out : [];
}
