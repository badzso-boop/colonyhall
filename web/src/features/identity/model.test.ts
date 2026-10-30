import { safeNext } from "./model";

test("safeNext keeps in-app paths and refuses everything else", () => {
  expect(safeNext("/c/123?thread=9")).toBe("/c/123?thread=9");
  expect(safeNext(null)).toBe("/");
  expect(safeNext("https://evil.example")).toBe("/");
  expect(safeNext("//evil.example")).toBe("/");
  expect(safeNext("/login")).toBe("/");
});
