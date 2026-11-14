// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// WCAG AA for the token pairs the components use (docs/07-design-system.md):
// text ≥ 4.5:1, icons/placeholders/control borders ≥ 3:1, in both themes.
// The values are read from tokens.css, so a change cannot slip past.
// @vitest-environment node
import { readFileSync } from "node:fs";

const tokensCss = readFileSync(new URL("./tokens.css", import.meta.url), "utf8");

function block(css: string, start: RegExp): string {
  const m = start.exec(css);
  if (!m) throw new Error(`block not found: ${start}`);
  let depth = 0;
  for (let i = m.index + m[0].length - 1; i < css.length; i++) {
    if (css[i] === "{") depth++;
    if (css[i] === "}" && --depth === 0) return css.slice(m.index, i);
  }
  throw new Error("unbalanced block");
}

function colours(css: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of css.matchAll(/--color-([a-z-]+):\s*(#[0-9a-fA-F]{6})\b/g)) out[m[1]!] = m[2]!;
  return out;
}

const light = colours(block(tokensCss, /@theme[^{]*\{/));
const dark = { ...light, ...colours(block(tokensCss, /\.dark\s*\{/)) };

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const f = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return 0.2126 * f(r!) + 0.7152 * f(g!) + 0.0722 * f(b!);
}

export function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi! + 0.05) / (lo! + 0.05);
}

const surfaces = ["bg", "surface", "surface-raised", "surface-sunken"];
const hues = ["red", "orange", "amber", "lime", "green", "teal", "cyan", "blue", "violet", "pink"];
// [foreground, backgrounds, minimum]
const pairs: Array<[string, string[], number]> = [
  ["fg", [...surfaces, "accent-soft"], 4.5],
  ["fg-muted", surfaces, 4.5],
  ["fg-subtle", surfaces, 3],
  ["border-strong", ["bg", "surface", "surface-raised"], 3],
  ["accent-text", ["bg", "surface", "surface-raised", "accent-soft"], 4.5],
  ["danger", ["surface", "danger-soft"], 4.5],
  ["accent-fg", ["accent", "accent-hover"], 4.5],
  ["warning", ["surface", "warning-soft"], 4.5],
  ["success", ["surface", "success-soft"], 4.5],
  ["info", ["surface", "info-soft"], 4.5],
  ["terminal-fg", ["terminal-bg"], 4.5],
  // The machine and agent palette (round 33): text on its tint, marks on surfaces.
  ...hues.flatMap((h): Array<[string, string[], number]> => [
    [`hue-${h}-fg`, [`hue-${h}-soft`], 4.5],
    [`hue-${h}-solid`, surfaces, 3],
  ]),
];

for (const [theme, t] of [["light", light], ["dark", dark]] as const) {
  describe(`${theme} theme`, () => {
    for (const [fg, bgs, min] of pairs) {
      for (const bg of bgs) {
        test(`${fg} on ${bg} ≥ ${min}:1`, () => {
          const a = t[fg];
          const b = t[bg];
          expect(a, `missing --color-${fg}`).toBeDefined();
          expect(b, `missing --color-${bg}`).toBeDefined();
          expect(contrast(a!, b!)).toBeGreaterThanOrEqual(min);
        });
      }
    }
  });
}

test("the ratio is computed correctly", () => {
  expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
  expect(contrast("#777777", "#ffffff")).toBeCloseTo(4.48, 2);
});
