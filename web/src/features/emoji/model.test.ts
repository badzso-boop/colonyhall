import { loadEmoji } from "./load";
import { activeColon, fold, insertAtColon, searchEmoji, unpack } from "./model";

test("the bundled data has every group but the skin tone parts, with both languages", async () => {
  const list = await loadEmoji();
  expect(list.length).toBeGreaterThan(1800);
  expect(list.some((e) => e.group === 2)).toBe(false);
  const thumb = list.find((e) => e.char === "👍")!;
  expect(thumb.code).toBe("+1");
  expect(thumb.name.hu).toBe("felfelé mutató hüvelykujj");
});

test("search finds by shortcode, English and Hungarian words, without accents", async () => {
  const list = await loadEmoji();
  expect(searchEmoji(list, "thu")[0]?.char).toBe("👍");
  expect(searchEmoji(list, "thumbsup")[0]?.char).toBe("👍");
  expect(searchEmoji(list, "hüvelykujj").map((e) => e.char)).toContain("👍");
  expect(searchEmoji(list, "huvelykujj").map((e) => e.char)).toContain("👍");
  expect(searchEmoji(list, "szív").map((e) => e.char)).toContain("❤️");
  expect(searchEmoji(list, "zzzqqq")).toEqual([]);
  expect(searchEmoji(list, "  ")).toEqual([]);
});

test("every word of a query must match", () => {
  const list = unpack([
    ["🐶", 3, "dog", "dog face", "dog face pet", "kutyafej", "kutya"],
    ["🐱", 3, "cat", "cat face", "cat face pet", "macskafej", "macska"],
  ]);
  expect(searchEmoji(list, "pet").map((e) => e.char)).toEqual(["🐶", "🐱"]);
  expect(searchEmoji(list, "face cat").map((e) => e.char)).toEqual(["🐱"]);
  expect(searchEmoji(list, "kutya").map((e) => e.char)).toEqual(["🐶"]);
  expect(fold("Hétfő")).toBe("hetfo");
});

test(":word at the caret", () => {
  expect(activeColon("szia :thu", 9)).toEqual({ start: 5, query: "thu" });
  expect(activeColon(":t", 2)).toBeNull();
  expect(activeColon("10:30", 5)).toBeNull();
  expect(activeColon("http://x", 8)).toBeNull();
  expect(activeColon("ok :)", 5)).toBeNull();
  expect(insertAtColon("szia :thu mára", 9, "👍")).toEqual({ text: "szia 👍  mára", caret: 8 });
});
