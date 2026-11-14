// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Section } from "./model";
import { dropIndex, itemOrder, moveConversation, moveSection, sectionOf, sectionOrder, upsertSection, validSectionName } from "./sections";

const sec = (id: string, position: number, items: string[] = []): Section => ({
  id,
  name: id,
  position,
  collapsed: false,
  items: items.map((channel_id, i) => ({ channel_id, position: i })),
});
const base = [sec("b", 1, ["c3"]), sec("a", 0, ["c1", "c2"])];

test("sectionOf and the orders follow the positions", () => {
  expect(sectionOf(base, "c2")).toBe("a");
  expect(sectionOf(base, "zz")).toBeNull();
  expect(sectionOrder(base)).toEqual(["a", "b"]);
  expect(itemOrder(base, "a")).toEqual(["c1", "c2"]);
  expect(itemOrder(base, "missing")).toEqual([]);
});

test("moveSection reorders and renumbers, clamping the index", () => {
  expect(sectionOrder(moveSection(base, "a", 1))).toEqual(["b", "a"]);
  expect(moveSection(base, "a", 9).map((s) => [s.id, s.position])).toEqual([
    ["b", 0],
    ["a", 1],
  ]);
  expect(sectionOrder(moveSection(base, "b", -3))).toEqual(["b", "a"]);
  expect(moveSection(base, "nope", 0)).toBe(base);
});

test("moveConversation within a section", () => {
  const out = moveConversation(base, "c2", "a", 0);
  expect(itemOrder(out, "a")).toEqual(["c2", "c1"]);
  expect(out.find((s) => s.id === "a")!.items.map((i) => i.position)).toEqual([0, 1]);
});

test("moveConversation between sections takes it out of the old one", () => {
  const out = moveConversation(base, "c1", "b");
  expect(itemOrder(out, "a")).toEqual(["c2"]);
  expect(itemOrder(out, "b")).toEqual(["c3", "c1"]);
});

test("moveConversation from a default group into a section, and back out", () => {
  const into = moveConversation(base, "new", "b", 0);
  expect(itemOrder(into, "b")).toEqual(["new", "c3"]);
  const out = moveConversation(into, "new", null);
  expect(sectionOf(out, "new")).toBeNull();
  expect(itemOrder(out, "b")).toEqual(["c3"]);
  // Untouched sections keep their identity (cheap re-renders).
  expect(out.find((s) => s.id === "a")).toBe(into.find((s) => s.id === "a"));
});

test("dropIndex: before the target from elsewhere, after it when moving down in the same section", () => {
  const order = ["c1", "c2", "c3"];
  expect(dropIndex(order, "x", "c2")).toBe(1);
  const down = moveConversation([sec("a", 0, order)], "c1", "a", dropIndex(order, "c1", "c3"));
  expect(itemOrder(down, "a")).toEqual(["c2", "c3", "c1"]);
  const up = moveConversation([sec("a", 0, order)], "c3", "a", dropIndex(order, "c3", "c1"));
  expect(itemOrder(up, "a")).toEqual(["c3", "c1", "c2"]);
  expect(dropIndex(order, "c1", "missing")).toBe(2);
});

test("upsertSection and the name rule", () => {
  expect(upsertSection(base, sec("c", 2)).map((s) => s.id)).toEqual(["b", "a", "c"]);
  expect(upsertSection(base, { ...sec("a", 0), name: "Új" }).find((s) => s.id === "a")!.name).toBe("Új");
  expect(validSectionName("  ")).toBe(false);
  expect(validSectionName(" Munka ")).toBe(true);
  expect(validSectionName("a".repeat(40))).toBe(true);
  expect(validSectionName("á".repeat(21))).toBe(false);
});
