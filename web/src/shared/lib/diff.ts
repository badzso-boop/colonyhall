// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// A line diff for the wiki history and the agent rules (round 32): our own
// longest-common-subsequence, no dependency.

export type DiffLine = { op: "same" | "add" | "del"; text: string };

/** A line diff (longest common subsequence), for the history sheet. */
export function lineDiff(a: string, b: string): DiffLine[] {
  const x = a.split("\n");
  const y = b.split("\n");
  const n = x.length;
  const m = y.length;
  // Too large for the quadratic table: show the whole change.
  if (n * m > 4_000_000) return [...x.map((text) => ({ op: "del" as const, text })), ...y.map((text) => ({ op: "add" as const, text }))];
  const lcs = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) lcs[i]![j] = x[i] === y[j] ? lcs[i + 1]![j + 1]! + 1 : Math.max(lcs[i + 1]![j]!, lcs[i]![j + 1]!);
  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      out.push({ op: "same", text: x[i]! });
      i++;
      j++;
    } else if (lcs[i + 1]![j]! >= lcs[i]![j + 1]!) out.push({ op: "del", text: x[i++]! });
    else out.push({ op: "add", text: y[j++]! });
  }
  while (i < n) out.push({ op: "del", text: x[i++]! });
  while (j < m) out.push({ op: "add", text: y[j++]! });
  return out;
}

export type DiffItem = DiffLine | { op: "skip"; count: number };

/** Long unchanged runs folded to a count, keeping `context` lines around each change. */
export function foldDiff(lines: DiffLine[], context = 2): DiffItem[] {
  const keep = lines.map(() => false);
  lines.forEach((l, i) => {
    if (l.op === "same") return;
    for (let j = Math.max(0, i - context); j <= Math.min(lines.length - 1, i + context); j++) keep[j] = true;
  });
  const out: DiffItem[] = [];
  for (let i = 0; i < lines.length; ) {
    if (keep[i]) {
      out.push(lines[i++]!);
      continue;
    }
    let j = i;
    while (j < lines.length && !keep[j]) j++;
    // Folding a single line saves nothing.
    if (j - i === 1) out.push(lines[i]!);
    else out.push({ op: "skip", count: j - i });
    i = j;
  }
  return out;
}
