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

test("hu and en have the same keys", async () => {
  const hu = (await import("./locales/hu.json")).default;
  const en = (await import("./locales/en.json")).default;
  const keys = (o: object, p = ""): string[] =>
    Object.entries(o).flatMap(([k, v]) => (typeof v === "object" && v ? keys(v, `${p}${k}.`) : [`${p}${k}`]));
  expect(keys(en).sort()).toEqual(keys(hu).sort());
});
