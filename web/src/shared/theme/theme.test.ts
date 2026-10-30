import { applyTheme, isDark, readTheme } from "./theme";

test("isDark follows the choice, and the system only in system mode", () => {
  expect(isDark("dark", false)).toBe(true);
  expect(isDark("light", true)).toBe(false);
  expect(isDark("system", true)).toBe(true);
  expect(isDark("system", false)).toBe(false);
});

test("applyTheme stores the choice and sets the class", () => {
  applyTheme("dark");
  expect(document.documentElement.classList.contains("dark")).toBe(true);
  expect(readTheme()).toBe("dark");

  applyTheme("light");
  expect(document.documentElement.classList.contains("dark")).toBe(false);
  expect(readTheme()).toBe("light");

  applyTheme("system");
  expect(readTheme()).toBe("system");
});
