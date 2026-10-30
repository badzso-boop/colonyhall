import { cn } from "./cn";

test("cn skips falsy parts", () => {
  expect(cn("a", false, null, undefined, "b")).toBe("a b");
  expect(cn()).toBe("");
});
