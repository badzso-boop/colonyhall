// Tests for ColonyMarkdown parser using Node's built-in test runner.
import test from "node:test";
import assert from "node:assert/strict";
import "./markdown.js";

const md = globalThis.ColonyMarkdown;

test("headings 1 to 6", () => {
  const input = `# H1
## H2
### H3
#### H4
##### H5
###### H6`;
  const blocks = md.parse(input);
  assert.equal(blocks.length, 6);
  for (let i = 0; i < 6; i++) {
    assert.equal(blocks[i].type, "heading");
    assert.equal(blocks[i].level, i + 1);
    assert.equal(blocks[i].children[0].text, `H${i + 1}`);
  }
});

test("inline formatting: bold, italic, strike, inline code", () => {
  const input = `**bold1** and __bold2__ and *italic1* and _italic2_ and ~~strike~~ and \`const x = 1\``;
  const blocks = md.parse(input);
  assert.equal(blocks.length, 1);
  const p = blocks[0];
  assert.equal(p.type, "paragraph");

  const types = p.children.map((c) => c.type);
  assert.ok(types.includes("strong"));
  assert.ok(types.includes("em"));
  assert.ok(types.includes("strike"));
  assert.ok(types.includes("code"));

  const strong1 = p.children.find((c) => c.type === "strong" && c.children[0].text === "bold1");
  assert.ok(strong1);
  const strong2 = p.children.find((c) => c.type === "strong" && c.children[0].text === "bold2");
  assert.ok(strong2);
  const em1 = p.children.find((c) => c.type === "em" && c.children[0].text === "italic1");
  assert.ok(em1);
  const em2 = p.children.find((c) => c.type === "em" && c.children[0].text === "italic2");
  assert.ok(em2);
  const strike = p.children.find((c) => c.type === "strike" && c.children[0].text === "strike");
  assert.ok(strike);
  const code = p.children.find((c) => c.type === "code" && c.text === "const x = 1");
  assert.ok(code);
});

test("word-boundary rule: identifiers and snake_case are NOT italic", () => {
  const inputs = [
    "PROFIT_TARGET",
    "run_id=9",
    "file_name.md",
    "2*3*4",
    "a_b_c",
    "foo_bar_baz"
  ];
  for (const str of inputs) {
    const blocks = md.parse(str);
    assert.equal(blocks.length, 1);
    const p = blocks[0];
    assert.equal(p.type, "paragraph");
    assert.equal(p.children.length, 1);
    assert.equal(p.children[0].type, "text");
    assert.equal(p.children[0].text, str);
  }

  // **PARTIAL** must be strong
  const partialBlocks = md.parse("**PARTIAL**");
  assert.equal(partialBlocks[0].children[0].type, "strong");
  assert.equal(partialBlocks[0].children[0].children[0].text, "PARTIAL");
});

test("link security: only http and https allowed, dangerous protocols stay plain text", () => {
  const dangerous = [
    "[bad](javascript:alert(1))",
    "[bad](JaVaScRiPt:alert(1))",
    "[bad]( javascript:alert(1))",
    "[bad](data:text/html,<script>alert(1)</script>)",
    "[bad](vbscript:msgbox)",
    "[bad](/relative/path/to/page)",
    "[bad](some/other/url)"
  ];
  for (const d of dangerous) {
    const blocks = md.parse(d);
    const p = blocks[0];
    assert.equal(p.type, "paragraph");
    // none of these should have type: "link"
    const hasLink = p.children.some((c) => c.type === "link");
    assert.equal(hasLink, false, `Dangerous link should not be parsed: ${d}`);
    assert.equal(p.children[0].type, "text");
    assert.equal(p.children[0].text, d);
  }

  // Safe links
  const safe = md.parse("[ok1](http://example.com) and [ok2](https://ujjweb.hu/test)");
  const p = safe[0];
  const links = p.children.filter((c) => c.type === "link");
  assert.equal(links.length, 2);
  assert.equal(links[0].href, "http://example.com");
  assert.equal(links[0].text, "ok1");
  assert.equal(links[1].href, "https://ujjweb.hu/test");
  assert.equal(links[1].text, "ok2");
});

test("images from URLs become links if http(s), else plain text (no remote img leaks)", () => {
  const blocks = md.parse("![my image](https://example.com/pic.png) and ![bad img](javascript:alert(1))");
  const p = blocks[0];
  const link = p.children.find((c) => c.type === "link");
  assert.ok(link);
  assert.equal(link.href, "https://example.com/pic.png");
  assert.equal(link.text, "my image");

  // Unsafe img stayed text
  const textNodes = p.children.filter((c) => c.type === "text").map((c) => c.text).join("");
  assert.ok(textNodes.includes("![bad img](javascript:alert(1))"));
});

test("raw HTML tags stay text nodes", () => {
  const input = `<script>alert(1)</script> and <img src=x onerror=alert(1)>`;
  const blocks = md.parse(input);
  const p = blocks[0];
  assert.equal(p.children.length, 1);
  assert.equal(p.children[0].type, "text");
  assert.equal(p.children[0].text, input);
});

test("file links: file:// and local paths become filelink nodes, but https urls do not", () => {
  const input = `Check file:///home/szmajli01/app.log and /var/log/syslog and ~/notes.txt, also [scheduler](file:///srv/scheduler.log).
Do not match https://example.com/home/user/test.`;
  const blocks = md.parse(input);
  const p = blocks[0];
  const filelinks = p.children.filter((c) => c.type === "filelink");
  assert.equal(filelinks.length, 4);

  assert.equal(filelinks[0].path, "/home/szmajli01/app.log");
  assert.equal(filelinks[0].name, "app.log");

  assert.equal(filelinks[1].path, "/var/log/syslog");
  assert.equal(filelinks[1].name, "syslog");

  assert.equal(filelinks[2].path, "~/notes.txt");
  assert.equal(filelinks[2].name, "notes.txt");

  assert.equal(filelinks[3].path, "/srv/scheduler.log");
  assert.equal(filelinks[3].name, "scheduler");

  // Check https url became a link, not a filelink
  const webLink = p.children.find((c) => c.type === "link");
  assert.ok(webLink);
  assert.equal(webLink.href, "https://example.com/home/user/test");
});

test("real agent answer with run_id=9 and file links", () => {
  const answer = `A futás (run_id=9) befejeződött. Az eredmények:
- PROFIT_TARGET elérve: **IGEN**
- Részletek: [scheduler.log](file:///home/szmajli01/homelab/trader/logs/scheduler.log)
- Statisztika: /home/szmajli01/homelab/trader/data/report.md`;

  const blocks = md.parse(answer);
  assert.equal(blocks.length, 2); // paragraph, then list

  assert.equal(blocks[0].type, "paragraph");
  // Check run_id=9 stayed text
  const pText = blocks[0].children.map((c) => c.text || "").join("");
  assert.ok(pText.includes("run_id=9"));

  assert.equal(blocks[1].type, "list");
  assert.equal(blocks[1].items.length, 3);

  // Item 1: PROFIT_TARGET and bold IGEN
  const item1 = blocks[1].items[0];
  const item1Text = item1.children.map((c) => c.text || "").join("");
  assert.ok(item1Text.includes("PROFIT_TARGET"));
  const strong = item1.children.find((c) => c.type === "strong");
  assert.ok(strong);
  assert.equal(strong.children[0].text, "IGEN");

  // Item 2: scheduler.log filelink
  const item2 = blocks[1].items[1];
  const file1 = item2.children.find((c) => c.type === "filelink");
  assert.ok(file1);
  assert.equal(file1.path, "/home/szmajli01/homelab/trader/logs/scheduler.log");
  assert.equal(file1.name, "scheduler.log");

  // Item 3: report.md filelink
  const item3 = blocks[1].items[2];
  const file2 = item3.children.find((c) => c.type === "filelink");
  assert.ok(file2);
  assert.equal(file2.path, "/home/szmajli01/homelab/trader/data/report.md");
  assert.equal(file2.name, "report.md");
});

test("fenced code blocks keep content verbatim with optional lang", () => {
  const code = `\`\`\`python
def calculate(a, b):
    # test_target = a * b
    return a * b
\`\`\``;
  const blocks = md.parse(code);
  assert.equal(blocks.length, 1);
  assert.equal(blocks[0].type, "codeblock");
  assert.equal(blocks[0].lang, "python");
  assert.equal(
    blocks[0].text,
    `def calculate(a, b):\n    # test_target = a * b\n    return a * b`
  );
});

test("tables (GitHub style)", () => {
  const tableMd = `| Oszlop 1 | Oszlop 2 |
| :--- | ---: |
| Érték 1 | Érték 2 |
| **Vastag** | \`kód\` |`;

  const blocks = md.parse(tableMd);
  assert.equal(blocks.length, 1);
  assert.equal(blocks[0].type, "table");
  assert.equal(blocks[0].headers.length, 2);
  assert.equal(blocks[0].headers[0][0].text, "Oszlop 1");
  assert.equal(blocks[0].headers[1][0].text, "Oszlop 2");

  assert.equal(blocks[0].rows.length, 2);
  assert.equal(blocks[0].rows[0][0][0].text, "Érték 1");
  assert.equal(blocks[0].rows[0][1][0].text, "Érték 2");
  assert.equal(blocks[0].rows[1][0][0].type, "strong");
  assert.equal(blocks[0].rows[1][1][0].type, "code");
});

test("nested lists with task items", () => {
  const listMd = `- [ ] Első feladat
  - [x] Alfeladat kész
  - Második alfeladat
- [x] Főfeladat befejezve`;

  const blocks = md.parse(listMd);
  assert.equal(blocks.length, 1);
  assert.equal(blocks[0].type, "list");
  assert.equal(blocks[0].items.length, 2);

  const item1 = blocks[0].items[0];
  assert.equal(item1.task, "todo");
  assert.equal(item1.children[0].text, "Első feladat");
  assert.ok(item1.sublist);
  assert.equal(item1.sublist.items.length, 2);
  assert.equal(item1.sublist.items[0].task, "done");
  assert.equal(item1.sublist.items[0].children[0].text, "Alfeladat kész");
  assert.equal(item1.sublist.items[1].task, null);

  const item2 = blocks[0].items[1];
  assert.equal(item2.task, "done");
  assert.equal(item2.children[0].text, "Főfeladat befejezve");
  assert.equal(item2.sublist, null);
});

test("mentions", () => {
  const input = `Hello @norbi and @agent-bot! Don't match test@example.com`;
  const blocks = md.parse(input);
  const p = blocks[0];
  const mentions = p.children.filter((c) => c.type === "mention");
  assert.equal(mentions.length, 2);
  assert.equal(mentions[0].handle, "norbi");
  assert.equal(mentions[1].handle, "agent-bot");

  // test@example.com was not matched as mention because '@' preceded by 't'
  const textAll = p.children.map((c) => c.text || "").join("");
  assert.ok(textAll.includes("test@example.com"));
});

test("pathological 20,000-character line finishes fast (< 2 s of CPU)", () => {
  const line = "*_".repeat(10000);
  // CPU time of this process, not wall time, and a wide bound: the parser
  // needs 20-250 ms, but 500 ms of wall time failed on loaded machines (make
  // ci, CI runners). A backtracking blow-up takes far longer than 2 s.
  const start = process.cpuUsage();
  const blocks = md.parse(line);
  const used = process.cpuUsage(start);
  const elapsed = (used.user + used.system) / 1000;
  assert.ok(elapsed < 2000, `Pathological line took too long: ${elapsed}ms of CPU`);
  assert.ok(blocks.length >= 1);
});

test("input over 20,000 characters is rendered as plain text", () => {
  const huge = "a".repeat(20005);
  const blocks = md.parse(huge);
  assert.equal(blocks.length, 1);
  assert.equal(blocks[0].type, "paragraph");
  assert.equal(blocks[0].children.length, 1);
  assert.equal(blocks[0].children[0].type, "text");
  assert.equal(blocks[0].children[0].text, huge);
});

test("ColonyMarkdown.plain strips markdown formatting cleanly", () => {
  const formatted = `# Főcím
Ez egy **vastag** és *dőlt* szöveg \`kóddal\`.
Itt egy [link](https://ujjweb.hu) és ![kép](https://ujjweb.hu/pic.png).
- lista 1
- [x] kész feladat
| A | B |
| --- | --- |
| 1 | 2 |
> Idézet
\`\`\`go
func main() {}
\`\`\``;

  const res = md.plain(formatted);
  assert.ok(!res.includes("#"));
  assert.ok(!res.includes("**"));
  assert.ok(!res.includes("[link]"));
  assert.ok(!res.includes("```"));
  assert.ok(!res.includes("|"));
  assert.ok(res.includes("Főcím"));
  assert.ok(res.includes("vastag"));
  assert.ok(res.includes("dőlt"));
  assert.ok(res.includes("kóddal"));
  assert.ok(res.includes("link"));
  assert.ok(res.includes("lista 1"));
  assert.ok(res.includes("kész feladat"));
  assert.ok(res.includes("func main() {}"));
});

test("file references: line numbers, ranges, agent handles, Windows paths", () => {
  const input = "Lásd: a/b.go:12 és a/b.go:12-30, továbbá @h:a/b.go:7 és C:\\project\\foo.go:12 vagy foo\\bar.go:12.";
  const blocks = md.parse(input);
  assert.equal(blocks.length, 1);
  const p = blocks[0];
  const filelinks = p.children.filter((c) => c.type === "filelink");
  assert.equal(filelinks.length, 5);

  // 1. a/b.go:12
  assert.equal(filelinks[0].path, "a/b.go");
  assert.equal(filelinks[0].name, "b.go:12");
  assert.equal(filelinks[0].lines, "12");
  assert.equal(filelinks[0].startLine, 12);
  assert.equal(filelinks[0].endLine, 12);
  assert.equal(filelinks[0].isRelative, true);

  // 2. a/b.go:12-30
  assert.equal(filelinks[1].path, "a/b.go");
  assert.equal(filelinks[1].name, "b.go:12-30");
  assert.equal(filelinks[1].lines, "12-30");
  assert.equal(filelinks[1].startLine, 12);
  assert.equal(filelinks[1].endLine, 30);
  assert.equal(filelinks[1].isRelative, true);

  // 3. @h:a/b.go:7
  assert.equal(filelinks[2].agent, "h");
  assert.equal(filelinks[2].path, "a/b.go");
  assert.equal(filelinks[2].name, "b.go:7");
  assert.equal(filelinks[2].lines, "7");
  assert.equal(filelinks[2].startLine, 7);
  assert.equal(filelinks[2].endLine, 7);
  assert.equal(filelinks[2].isRelative, true);

  // 4. C:\project\foo.go:12
  assert.equal(filelinks[3].path, "C:\\project\\foo.go");
  assert.equal(filelinks[3].name, "foo.go:12");
  assert.equal(filelinks[3].lines, "12");
  assert.equal(filelinks[3].startLine, 12);
  assert.equal(filelinks[3].endLine, 12);
  assert.equal(filelinks[3].isRelative, false);

  // 5. foo\bar.go:12
  assert.equal(filelinks[4].path, "foo\\bar.go");
  assert.equal(filelinks[4].name, "bar.go:12");
  assert.equal(filelinks[4].lines, "12");
  assert.equal(filelinks[4].startLine, 12);
  assert.equal(filelinks[4].endLine, 12);
});

test("non-references: http urls with port and times are not file references", () => {
  const input = "Megtekinthető itt: http://x/y:8080 és az időpont 10:42 legyen.";
  const blocks = md.parse(input);
  const p = blocks[0];
  const filelinks = p.children.filter((c) => c.type === "filelink");
  assert.equal(filelinks.length, 0);

  const weblinks = p.children.filter((c) => c.type === "link");
  assert.equal(weblinks.length, 1);
  assert.equal(weblinks[0].href, "http://x/y:8080");

  const fullText = p.children.map((c) => c.text || c.raw || "").join("");
  assert.ok(fullText.includes("10:42"));
});

