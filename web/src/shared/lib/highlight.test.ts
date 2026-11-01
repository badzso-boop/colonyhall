import { matchRanges } from "./highlight";

test("a substring first, else the letters of a subsequence; nothing when it does not match", () => {
  expect(matchRanges("backend-claude", "CLAU")).toEqual([[8, 12]]);
  expect(matchRanges("backend-claude", "bkc")).toEqual([[0, 1], [3, 4], [8, 9]]);
  expect(matchRanges("backend", "xyz")).toEqual([]);
  expect(matchRanges("backend", " ")).toEqual([]);
});
