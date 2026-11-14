// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Writes src/features/emoji/data/emoji.json (and its LICENSE) from emojibase-data (MIT; the
// CLDR names and keywords in English and Hungarian). Run after updating the
// package: `npm run gen:emoji`. The PWA bundles the result in the picker's
// lazy chunk and loads nothing from another origin (round 55, part C).
import { readFileSync, writeFileSync } from "node:fs";

const read = (p) => JSON.parse(readFileSync(new URL(`../node_modules/emojibase-data/${p}`, import.meta.url), "utf8"));
const en = read("en/compact.json");
const hu = new Map(read("hu/compact.json").map((e) => [e.hexcode, e]));
const github = read("en/shortcodes/github.json");
const words = (...parts) => [...new Set(parts.flat().filter(Boolean).map((w) => w.toLowerCase()))].join(" ");

// Group 2 is the skin tone components; no group: the regional indicator letters.
const list = en
  .filter((e) => e.group !== undefined && e.group !== 2)
  .sort((a, b) => a.order - b.order)
  .map((e) => {
    const h = hu.get(e.hexcode);
    const codes = [github[e.hexcode] ?? []].flat();
    // [emoji, group, shortcode, English name, English keywords, Hungarian name, Hungarian keywords]
    return [e.unicode, e.group, codes[0] ?? "", e.label, words(codes, e.tags), h?.label ?? e.label, words(h?.tags)];
  });

const out = new URL("../src/features/emoji/data/emoji.json", import.meta.url);
writeFileSync(new URL("../src/features/emoji/data/LICENSE", import.meta.url), readFileSync(new URL("../node_modules/emojibase-data/LICENSE", import.meta.url)));
writeFileSync(out, JSON.stringify({ source: "emojibase-data 18.0.0 (MIT), Unicode CLDR", emoji: list }) + "\n");
console.log(`emoji.json: ${list.length} emoji`);
