// Components use semantic tokens only (docs/08-frontend-architektura.md, 6.):
// no hex/rgb/hsl colours and no Tailwind default palette outside tokens.css.
// Read from disk: Vitest imports CSS as empty, even with ?raw.
// @vitest-environment node
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const srcDir = fileURLToPath(new URL("../..", import.meta.url));
const sources: Record<string, string> = Object.fromEntries(
  readdirSync(srcDir, { recursive: true, encoding: "utf8" })
    .filter((f) => /\.(ts|tsx|css)$/.test(f))
    .map((f) => [f.replaceAll("\\", "/"), readFileSync(join(srcDir, f), "utf8")]),
);

const rawColour = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch)\(/;
const palette =
  /\b(?:bg|text|border|ring|fill|stroke|from|via|to|outline|shadow|decoration|accent|caret|divide|placeholder)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b/;

test("no raw colours or default palette classes outside tokens.css", () => {
  expect(Object.keys(sources).some((p) => p.endsWith("index.css"))).toBe(true);
  const offenders: string[] = [];
  for (const [path, text] of Object.entries(sources)) {
    if (path.endsWith("/tokens.css") || path.endsWith(".d.ts") || path.includes(".test.")) continue;
    text.split("\n").forEach((line, i) => {
      if (rawColour.test(line) || palette.test(line)) offenders.push(`${path}:${i + 1}: ${line.trim()}`);
    });
  }
  expect(offenders).toEqual([]);
});

test("the check catches what it should", () => {
  expect(rawColour.test('style={{ color: "#e5383b" }}')).toBe(true);
  expect(palette.test('className="bg-red-500"')).toBe(true);
  expect(palette.test('className="bg-accent text-fg-muted"')).toBe(false);
});
