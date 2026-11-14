// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The emoji picker's data and search (round 55, part C). The data is
// bundled (data/emoji.json, from emojibase-data) and drawn with the system
// emoji font: nothing is loaded from another origin.

/** One row of data/emoji.json: [emoji, group, shortcode, en name, en keywords, hu name, hu keywords]. */
export type RawEmoji = [string, number, string, string, string, string, string];

export type Emoji = {
  char: string;
  group: number;
  /** GitHub-style shortcode (":thumbsup:"), empty when there is none. */
  code: string;
  name: { en: string; hu: string };
  /** Folded words to match: the shortcode, the names and the keywords in both languages. */
  words: string[];
};

/** The picker's groups in Unicode order (2 is the skin tone components, left out). */
export const groups = [0, 1, 3, 4, 5, 6, 7, 8, 9] as const;

/** Lower case without accents, so "hetfo" finds "hétfő". */
export function fold(s: string): string {
  return s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();
}

export function unpack(rows: RawEmoji[]): Emoji[] {
  return rows.map(([char, group, code, enName, enWords, huName, huWords]) => ({
    char,
    group,
    code,
    name: { en: enName, hu: huName },
    words: [...new Set(fold(`${code} ${enName} ${enWords} ${huName} ${huWords}`).split(/[\s:,]+/).filter(Boolean))],
  }));
}

/**
 * Emoji for a query, best first: the shortcode itself, then a shortcode or
 * name starting with it, then any word starting with it. Every word of a
 * query of several words must match.
 */
export function searchEmoji(list: Emoji[], query: string, limit = 64): Emoji[] {
  const parts = fold(query).split(/\s+/).filter(Boolean);
  if (!parts.length) return [];
  const scored: Array<{ e: Emoji; score: number; i: number }> = [];
  list.forEach((e, i) => {
    let score = 0;
    for (const q of parts) {
      let best = 0;
      if (e.code === q) best = 400;
      else if (e.code.startsWith(q)) best = 300;
      else if (fold(e.name.en).startsWith(q) || fold(e.name.hu).startsWith(q)) best = 200;
      else if (e.words.some((w) => w.startsWith(q))) best = 100;
      if (!best) return;
      score += best;
    }
    scored.push({ e, score, i });
  });
  scored.sort((a, b) => b.score - a.score || a.i - b.i);
  return scored.slice(0, limit).map((s) => s.e);
}

/** The ":word" being typed at the caret (two letters at least), if any. */
export function activeColon(text: string, caret: number): { start: number; query: string } | null {
  const m = /(^|\s):([\p{L}\p{N}_+-]{2,})$/u.exec(text.slice(0, caret));
  if (!m) return null;
  return { start: caret - m[2]!.length - 1, query: m[2]! };
}

/** Replaces the ":word" at the caret with the emoji and a space. */
export function insertAtColon(text: string, caret: number, char: string): { text: string; caret: number } {
  const at = activeColon(text, caret);
  if (!at) return { text, caret };
  const insert = `${char} `;
  return { text: text.slice(0, at.start) + insert + text.slice(caret), caret: at.start + insert.length };
}

/** The name in the interface language (Hungarian unless English). */
export function emojiName(e: Emoji, language: string): string {
  return language.startsWith("en") ? e.name.en : e.name.hu;
}
