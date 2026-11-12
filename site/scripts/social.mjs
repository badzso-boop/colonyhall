#!/usr/bin/env node
// Makes the social preview image (og:image, and the GitHub social preview)
// from the finished site's hero, with Playwright (docs/18, 5). The page
// shows only invented people and agents, so no demo seed is needed.
//
//   npm --prefix site install --no-save playwright@1.56.1
//   npx --prefix site playwright install chromium   (once)
//   npm --prefix site run build && npm --prefix site run social
//   npm --prefix site run build                      (to include the new image)
//
// CHROME_PATH may point at a Chromium binary to use instead of Playwright's.
// Writes site/src/img/social.jpg (1200x630).
import { createServer } from "node:http";
import { existsSync, mkdirSync, readFileSync, statSync } from "node:fs";
import { extname, join, normalize } from "node:path";

const HERE = new URL("..", import.meta.url).pathname;
const DIST = join(HERE, "dist");
const OUT = join(HERE, "src", "img", "social.jpg");
if (!existsSync(join(DIST, "en", "index.html"))) {
  console.error("social: build the site first (npm --prefix site run build)");
  process.exit(1);
}

let chromium;
try {
  ({ chromium } = await import("playwright"));
} catch {
  console.error("social: Playwright is not installed; run: npm --prefix site install --no-save playwright@1.56.1");
  process.exit(1);
}

const TYPES = { ".html": "text/html", ".css": "text/css", ".js": "text/javascript", ".svg": "image/svg+xml", ".woff2": "font/woff2", ".jpg": "image/jpeg", ".png": "image/png" };
const server = createServer((req, res) => {
  let p = normalize(decodeURIComponent(new URL(req.url, "http://x").pathname)).replace(/^(\.\.[/\\])+/, "");
  let f = join(DIST, p);
  if (existsSync(f) && statSync(f).isDirectory()) f = join(f, "index.html");
  if (!f.startsWith(DIST) || !existsSync(f)) { res.writeHead(404); res.end(); return; }
  res.writeHead(200, { "content-type": TYPES[extname(f)] || "application/octet-stream" });
  res.end(readFileSync(f));
}).listen(0, "127.0.0.1");
await new Promise((r) => server.once("listening", r));
const url = `http://127.0.0.1:${server.address().port}/en/`;

const browser = await chromium.launch(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {});
// 1600x840 CSS pixels at 0.75 = a 1200x630 image with the desktop layout
const page = await browser.newPage({ viewport: { width: 1600, height: 840 }, deviceScaleFactor: 0.75, colorScheme: "dark" });
await page.goto(url);
// wait until the hand-off has happened: the reviewer's answer is on the monitor
await page.waitForSelector('[data-msg="m3"].is-in', { timeout: 60000 });
await page.waitForTimeout(1500);
mkdirSync(join(HERE, "src", "img"), { recursive: true });
await page.screenshot({ path: OUT, type: "jpeg", quality: 82 });
await browser.close();
server.close();
console.log(`social: wrote ${OUT.slice(HERE.length)} (${(statSync(OUT).size / 1024).toFixed(1)} kB)`);
