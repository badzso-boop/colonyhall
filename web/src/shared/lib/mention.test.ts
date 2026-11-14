// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { activeMention, fuzzyScore, insertMention, rankMentions } from "./mention";

const a = (id: string, handle: string, display_name = handle, halted = false) => ({ id, handle, display_name, halted });

test("fuzzyScore ranks exact, prefix, word start, subsequence", () => {
  expect(fuzzyScore("backend-claude", "backend-claude")).toBeGreaterThan(fuzzyScore("backend-claude", "back"));
  expect(fuzzyScore("backend-claude", "back")).toBeGreaterThan(fuzzyScore("backend-claude", "claude"));
  expect(fuzzyScore("backend-claude", "claude")).toBeGreaterThan(fuzzyScore("backend-claude", "bcl"));
  expect(fuzzyScore("backend-claude", "xyz")).toBe(0);
});

test("rankMentions: members first, halted left out, at most 8", () => {
  const actors = [a("1", "backend-claude"), a("2", "backup", "Backup"), a("3", "bob", "Bob", true), ...Array.from({ length: 10 }, (_, i) => a(`x${i}`, `b${i}`))];
  const r = rankMentions("ba", actors, new Set(["2"]));
  expect(r[0]!.handle).toBe("backup");
  expect(r.map((x) => x.handle)).not.toContain("bob");
  expect(rankMentions("", actors, new Set())).toHaveLength(8);
});

test("activeMention and insertMention work at the caret", () => {
  expect(activeMention("szia @bac", 9)).toEqual({ start: 5, query: "bac" });
  expect(activeMention("e-mail@x", 8)).toBeNull();
  expect(insertMention("szia @bac és", 9, "backend-claude")).toEqual({ text: "szia @backend-claude  és", caret: 21 });
});
