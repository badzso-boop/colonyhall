// The Colonyhall mark, defined once: an ant seen from above (head, thorax,
// abdomen, two antennae, three pairs of legs) on a 24×24 grid. This script
// writes every brand file from it, so the SVGs and PNGs cannot drift:
//
//   node scripts/brand.mjs        (run from web/; the outputs are committed)
//
// PNGs are rasterised here with 4×4 supersampling and encoded with Node's
// zlib, so no image library (and no copyleft dependency) is needed.
import { mkdirSync, writeFileSync } from "node:fs";
import { deflateSync, crc32 } from "node:zlib";

const RED = "#e5383b";
const DARK = "#0b0a0a";

const stroke = 1.4;
const lines = [
  // antennae
  [10.6, 3.6, 8.6, 1.2],
  [13.4, 3.6, 15.4, 1.2],
  // legs, left then right
  [10.5, 9.2, 6.4, 7.2],
  [10.4, 10.8, 5.8, 10.8],
  [10.5, 12.3, 6.5, 14.9],
  [13.5, 9.2, 17.6, 7.2],
  [13.6, 10.8, 18.2, 10.8],
  [13.5, 12.3, 17.5, 14.9],
];
const circles = [[12, 5, 2.2]]; // head
const ellipses = [
  [12, 10.8, 2, 2.6], // thorax
  [12, 17.6, 3.6, 4.6], // abdomen
];

/* ---------- SVG ---------- */

function markShapes(color) {
  const path = lines.map(([x1, y1, x2, y2]) => `M${x1} ${y1}L${x2} ${y2}`).join("");
  return [
    `<path d="${path}" fill="none" stroke="${color}" stroke-width="${stroke}" stroke-linecap="round"/>`,
    ...circles.map(([cx, cy, r]) => `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${color}"/>`),
    ...ellipses.map(([cx, cy, rx, ry]) => `<ellipse cx="${cx}" cy="${cy}" rx="${rx}" ry="${ry}" fill="${color}"/>`),
  ].join("");
}

const svg = (viewBox, body, title = "Colonyhall") =>
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}"><title>${title}</title>${body}</svg>\n`;

const files = {
  "mark.svg": svg("0 0 24 24", markShapes(RED)),
  "mark-mono.svg": svg("0 0 24 24", markShapes("currentColor")),
  "favicon.svg": svg("0 0 32 32", `<rect width="32" height="32" rx="7" fill="${DARK}"/><g transform="translate(4 4)">${markShapes(RED)}</g>`),
  // The wordmark stays text (Inter 600 with fallbacks): it renders crisply and
  // stays small; where Inter is missing, a system sans is used. logo.svg is
  // for light backgrounds, logo-on-dark.svg for dark ones (an <img> cannot
  // inherit currentColor).
  "logo.svg": logo("#141111"),
  "logo-on-dark.svg": logo("#f2eeee"),
};

function logo(textColor) {
  return svg(
    "0 0 152 32",
    `<g transform="translate(2 4)">${markShapes(RED)}</g>` +
      `<text x="34" y="22.5" font-family="Inter Variable, Inter, system-ui, sans-serif" font-size="19" font-weight="600" fill="${textColor}">Colonyhall</text>`,
  );
}

/* ---------- PNG ---------- */

function hexRGB(hex) {
  return [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
}

function distToSegment(px, py, x1, y1, x2, y2) {
  const dx = x2 - x1;
  const dy = y2 - y1;
  const t = Math.max(0, Math.min(1, ((px - x1) * dx + (py - y1) * dy) / (dx * dx + dy * dy)));
  return Math.hypot(px - (x1 + t * dx), py - (y1 + t * dy));
}

// Is the point (in mark units) inside the mark?
function inMark(x, y) {
  for (const [cx, cy, r] of circles) if ((x - cx) ** 2 + (y - cy) ** 2 <= r * r) return true;
  for (const [cx, cy, rx, ry] of ellipses) if (((x - cx) / rx) ** 2 + ((y - cy) / ry) ** 2 <= 1) return true;
  for (const [x1, y1, x2, y2] of lines) if (distToSegment(x, y, x1, y1, x2, y2) <= stroke / 2) return true;
  return false;
}

function inRoundedRect(x, y, size, radius) {
  const cx = Math.min(Math.max(x, radius), size - radius);
  const cy = Math.min(Math.max(y, radius), size - radius);
  return (x - cx) ** 2 + (y - cy) ** 2 <= radius * radius;
}

/**
 * Renders a square icon: a dark background (rounded when radius > 0) and the
 * mark scaled to `markFraction` of the side, centred.
 */
function renderIcon(size, { markFraction, radius }) {
  const ss = 4;
  const [br, bg, bb] = hexRGB(DARK);
  const [mr, mg, mb] = hexRGB(RED);
  const scale = (size * markFraction) / 24;
  const offset = (size - 24 * scale) / 2;
  const rgba = Buffer.alloc(size * size * 4);
  for (let py = 0; py < size; py++) {
    for (let px = 0; px < size; px++) {
      let bgHits = 0;
      let markHits = 0;
      for (let sy = 0; sy < ss; sy++) {
        for (let sx = 0; sx < ss; sx++) {
          const x = px + (sx + 0.5) / ss;
          const y = py + (sy + 0.5) / ss;
          if (radius > 0 && !inRoundedRect(x, y, size, radius)) continue;
          bgHits++;
          if (inMark((x - offset) / scale, (y - offset) / scale)) markHits++;
        }
      }
      const n = ss * ss;
      const a = bgHits / n;
      const m = bgHits ? markHits / bgHits : 0;
      const i = (py * size + px) * 4;
      rgba[i] = Math.round(br + (mr - br) * m);
      rgba[i + 1] = Math.round(bg + (mg - bg) * m);
      rgba[i + 2] = Math.round(bb + (mb - bb) * m);
      rgba[i + 3] = Math.round(a * 255);
    }
  }
  return encodePNG(size, size, rgba);
}

function chunk(type, data) {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  const td = Buffer.concat([Buffer.from(type, "ascii"), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(td) >>> 0);
  return Buffer.concat([len, td, crc]);
}

function encodePNG(w, h, rgba) {
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 6; // RGBA
  const raw = Buffer.alloc(h * (w * 4 + 1));
  for (let y = 0; y < h; y++) {
    raw[y * (w * 4 + 1)] = 0; // filter: none
    rgba.copy(raw, y * (w * 4 + 1) + 1, y * w * 4, (y + 1) * w * 4);
  }
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", ihdr),
    chunk("IDAT", deflateSync(raw, { level: 9 })),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

const pngs = {
  // "any" icons: a rounded dark tile.
  "icon-192.png": renderIcon(192, { markFraction: 0.74, radius: 42 }),
  "icon-512.png": renderIcon(512, { markFraction: 0.74, radius: 112 }),
  // Maskable: full bleed, the mark inside the 80 % safe circle.
  "maskable-512.png": renderIcon(512, { markFraction: 0.56, radius: 0 }),
  // iOS rounds the corners itself.
  "apple-touch-icon.png": renderIcon(180, { markFraction: 0.68, radius: 0 }),
};

const out = new URL("../public/brand/", import.meta.url);
mkdirSync(out, { recursive: true });
for (const [name, content] of Object.entries({ ...files, ...pngs })) {
  writeFileSync(new URL(name, out), content);
}
console.log(`wrote ${Object.keys(files).length + Object.keys(pngs).length} files to public/brand/`);
