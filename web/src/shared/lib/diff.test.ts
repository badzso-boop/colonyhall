// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { foldDiff, lineDiff } from "./diff";

test("unchanged runs fold to a count around the changes", () => {
  const a = Array.from({ length: 20 }, (_, i) => `sor ${i}`).join("\n");
  const b = a.replace("sor 10", "SOR 10") + "\núj";
  const folded = foldDiff(lineDiff(a, b));
  expect(folded).toEqual([
    { op: "skip", count: 8 },
    { op: "same", text: "sor 8" },
    { op: "same", text: "sor 9" },
    { op: "del", text: "sor 10" },
    { op: "add", text: "SOR 10" },
    { op: "same", text: "sor 11" },
    { op: "same", text: "sor 12" },
    { op: "skip", count: 5 },
    { op: "same", text: "sor 18" },
    { op: "same", text: "sor 19" },
    { op: "add", text: "új" },
  ]);
  expect(foldDiff(lineDiff("x", "x"))).toEqual([{ op: "same", text: "x" }]);
});
