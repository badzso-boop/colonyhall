import { lineDiff, quote, tagCounts, wikiPosts, type Revision } from "./posts";

const rev = (rev: number, kind: Revision["kind"], body: string, author = "a"): Revision => ({ page_id: "p", rev, kind, body, author_id: author, created_at: `2026-10-30T0${rev}:00:00Z` });
const header = (h: string, rev: number) => `\n\n---\n*@${h}, 2026-10-30 0${rev}:00:*\n\n`;

test("create and appends: the opening post and the replies without their header", () => {
  const b1 = "# Téma\n\nNyitó.";
  const b2 = b1 + header("backend", 2) + "Első válasz.";
  const b3 = b2 + header("norbi", 3) + "Második\n\nválasz.";
  const posts = wikiPosts([rev(3, "append", b3, "h"), rev(1, "create", b1), rev(2, "append", b2, "b")]);
  expect(posts).toEqual([
    { kind: "post", rev: 1, authorId: "a", at: "2026-10-30T01:00:00Z", body: b1, opening: true },
    { kind: "post", rev: 2, authorId: "b", at: "2026-10-30T02:00:00Z", body: "Első válasz.", opening: false },
    { kind: "post", rev: 3, authorId: "h", at: "2026-10-30T03:00:00Z", body: "Második\n\nválasz.", opening: false },
  ]);
});

test("a human edit in the middle is a line, and later appends diff against it", () => {
  const b1 = "Nyitó.";
  const b2 = b1 + header("b", 2) + "Válasz.";
  const edited = "Átírt nyitó.\n\nÖsszefoglaló.";
  const b4 = edited + header("c", 4) + "Új válasz.";
  const posts = wikiPosts([rev(1, "create", b1), rev(2, "append", b2), rev(3, "edit", edited, "h"), rev(4, "append", b4, "c")]);
  expect(posts.map((p) => [p.kind, p.rev, "body" in p ? p.body : null])).toEqual([
    ["post", 1, "Nyitó."],
    ["post", 2, "Válasz."],
    ["edited", 3, null],
    ["post", 4, "Új válasz."],
  ]);
});

test("deleted, then restored, then appended", () => {
  const b1 = "Nyitó.";
  const b4 = b1 + header("b", 4) + "Visszatértem.";
  const posts = wikiPosts([rev(1, "create", b1), rev(2, "delete", b1, "h"), rev(3, "restore", "", "h"), rev(4, "append", b4, "b")]);
  expect(posts.map((p) => p.kind)).toEqual(["post", "deleted", "restored", "post"]);
  expect(posts[3]).toMatchObject({ body: "Visszatértem." });
});

test("an append that does not start with the previous body falls back to an edited full body", () => {
  const posts = wikiPosts([rev(1, "create", "Egy."), rev(2, "append", "Valami egészen más.")]);
  expect(posts[1]).toEqual({ kind: "edited", rev: 2, authorId: "a", at: "2026-10-30T02:00:00Z", body: "Valami egészen más." });
});

test("line diff, tag counts, quoting", () => {
  expect(lineDiff("a\nb\nc", "a\nx\nc\nd")).toEqual([
    { op: "same", text: "a" },
    { op: "del", text: "b" },
    { op: "add", text: "x" },
    { op: "same", text: "c" },
    { op: "add", text: "d" },
  ]);
  expect(tagCounts([{ tags: ["ops", "db"] }, { tags: ["db"] }, { tags: [] }])).toEqual([["db", 2], ["ops", 1]]);
  expect(quote("a\nb", "backend")).toBe("> @backend:\n> a\n> b\n\n");
});
