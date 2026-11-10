import { formatScore, groupHits, hitHref, scoreParts, searchPath, snippetParts, type SearchHit } from "./model";

const hit = (over: Partial<SearchHit>): SearchHit => ({
  kind: "message",
  source_id: "00000000-0000-0000-0000-000000000001",
  part: 0,
  at: "2026-11-05T10:00:00Z",
  header: "#fejlesztes · @norbi · 2026-11-05",
  snippet: "szöveg",
  ...over,
});

describe("snippet highlighting", () => {
  test("text without markers is one plain part", () => {
    expect(snippetParts("nincs benne jelölés")).toEqual([{ text: "nincs benne jelölés", mark: false }]);
    expect(snippetParts("")).toEqual([]);
  });
  test("«…» pairs become marked parts, the markers themselves disappear", () => {
    expect(snippetParts("a «postgres» és «pgx» között")).toEqual([
      { text: "a ", mark: false },
      { text: "postgres", mark: true },
      { text: " és ", mark: false },
      { text: "pgx", mark: true },
      { text: " között", mark: false },
    ]);
    expect(snippetParts("«egész»")).toEqual([{ text: "egész", mark: true }]);
  });
  test("an unclosed « and a stray » stay as plain text, nothing is lost", () => {
    expect(snippetParts("eleje «nyitott vége")).toEqual([{ text: "eleje «nyitott vége", mark: false }]);
    expect(snippetParts("csak » záró")).toEqual([{ text: "csak » záró", mark: false }]);
    expect(snippetParts("«jó» és «rossz")).toEqual([
      { text: "jó", mark: true },
      { text: " és «rossz", mark: false },
    ]);
  });
  test("an empty «» marks nothing", () => {
    expect(snippetParts("a «» b")).toEqual([{ text: "a  b", mark: false }]);
  });
});

test("hits are grouped by kind in a fixed order, keeping the server's ranking inside a group", () => {
  const hits = [
    hit({ kind: "wiki", source_id: "w1" }),
    hit({ kind: "message", source_id: "m1" }),
    hit({ kind: "summary", source_id: "s1" }),
    hit({ kind: "message", source_id: "m2" }),
  ];
  expect(groupHits(hits).map((g) => [g.kind, g.hits.map((h) => h.source_id)])).toEqual([
    ["message", ["m1", "m2"]],
    ["wiki", ["w1"]],
    ["summary", ["s1"]],
  ]);
  expect(groupHits([])).toEqual([]);
});

test("each kind links to its place in the app", () => {
  expect(hitHref(hit({ channel_id: "c1" }))).toBe("/c/c1");
  expect(hitHref(hit({ channel_id: "c1", thread_root_id: "t1" }))).toBe("/c/c1?thread=t1");
  expect(hitHref(hit({}))).toBeNull();
  expect(hitHref(hit({ kind: "summary", channel_id: "c2" }))).toBe("/c/c2");
  expect(hitHref(hit({ kind: "wiki", ref: "deploy notes" }))).toBe("/wiki/deploy%20notes");
  expect(hitHref(hit({ kind: "decision", ref: "D-12" }))).toBe("/decisions#D-12");
  expect(hitHref(hit({ kind: "decision" }))).toBe("/decisions");
});

test("stage scores: only the stages present, in pipeline order", () => {
  expect(scoreParts(undefined)).toEqual([]);
  expect(scoreParts({ fused: 0.032 })).toEqual([{ stage: "fused", value: 0.032 }]);
  expect(scoreParts({ rerank: 0.9, fused: 0.032, semantic: 0.81, keyword: 0.12 }).map((p) => p.stage)).toEqual(["keyword", "semantic", "fused", "rerank"]);
  expect([0.12, 0.81, 0.032, 0.9, 0, 0.0001234, 12.5].map(formatScore)).toEqual(["0.12", "0.81", "0.032", "0.9", "0", "0.0001", "12.5"]);
});

test("the search path keeps the default mode out of the URL", () => {
  expect(searchPath("  postgres verzió ")).toBe("/search?q=postgres+verzi%C3%B3");
  expect(searchPath("pgx", "keyword")).toBe("/search?q=pgx&mode=keyword");
  expect(searchPath("")).toBe("/search");
});
