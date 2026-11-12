// Builds the site into site/dist and checks the rules every version of the
// site keeps, whatever its design (round 48, part 5.2):
//   - nothing loaded from another origin at runtime: no external scripts,
//     stylesheets, fonts, images or frames (self-host everything; no
//     tracking, no cookies);
//   - a size budget for the whole site;
//   - every page has a language, a title and a description.
// No dependencies: plain Node. `npm run check` only checks src/.
import { cpSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const SRC = new URL("./src/", import.meta.url).pathname;
const DIST = new URL("./dist/", import.meta.url).pathname;
const BUDGET_BYTES = 500 * 1024; // the whole site, before compression

const files = (dir) =>
  readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    return statSync(p).isDirectory() ? files(p) : [p];
  });

// Loads a resource: src= on anything, href= on <link>, url() in CSS,
// @import. Plain links to other sites (<a href>) are fine.
const external = /(?:\ssrc\s*=\s*["']?\s*(?:https?:)?\/\/)|(?:<link\b[^>]*\bhref\s*=\s*["']?\s*(?:https?:)?\/\/)|(?:url\(\s*["']?\s*(?:https?:)?\/\/)|(?:@import\s+(?:url\()?\s*["']?\s*(?:https?:)?\/\/)/i;

const problems = [];
let total = 0;
for (const f of files(SRC)) {
  const rel = relative(SRC, f);
  total += statSync(f).size;
  if (![".html", ".css", ".js", ".mjs", ".svg"].includes(extname(f))) continue;
  const text = readFileSync(f, "utf8");
  if (external.test(text)) problems.push(`${rel}: loads something from another origin; self-host it`);
  if (/document\.cookie|localStorage\s*\.\s*setItem\(\s*["'](?:_ga|_gid)/i.test(text)) problems.push(`${rel}: cookies or tracking`);
  if (extname(f) === ".html") {
    if (!/<html[^>]*\blang=/i.test(text)) problems.push(`${rel}: <html> needs a lang attribute`);
    if (!/<title>[^<]+<\/title>/i.test(text)) problems.push(`${rel}: needs a <title>`);
    if (!/<meta\s+name=["']description["']/i.test(text)) problems.push(`${rel}: needs a meta description`);
  }
}
if (total > BUDGET_BYTES) problems.push(`the site is ${total} bytes, over the ${BUDGET_BYTES} byte budget`);

if (problems.length) {
  console.error(problems.join("\n"));
  process.exit(1);
}
console.log(`site: ${files(SRC).length} files, ${total} bytes (budget ${BUDGET_BYTES}); checks passed`);
if (!process.argv.includes("--check-only")) {
  rmSync(DIST, { recursive: true, force: true });
  mkdirSync(DIST, { recursive: true });
  cpSync(SRC, DIST, { recursive: true });
  console.log(`site: built into ${relative(process.cwd(), DIST) || "dist"}/`);
}
