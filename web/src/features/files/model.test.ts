// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { gitLetter, gitMarks, languageOf, lineReference, parseRef, rangeFor } from "./model";

test("git letters and dirty folders", () => {
  expect([" M", "??", "A ", "R ", " D", "MM", "  "].map(gitLetter)).toEqual(["M", "?", "A", "R", "D", "M", null]);
  const { letters, dirty } = gitMarks([
    { path: "web/src/app/App.tsx", status: " M" },
    { path: "docs/new.md", status: "??" },
  ]);
  expect(letters.get("web/src/app/App.tsx")).toBe("M");
  expect([...dirty].sort()).toEqual(["docs", "web", "web/src", "web/src/app"]);
});

test("languageOf", () => {
  expect(languageOf("internal/server.go")).toBe("go");
  expect(languageOf("web/src/App.tsx")).toBe("typescript");
  expect(languageOf("deploy/Dockerfile")).toBe("dockerfile");
  expect(languageOf("notes.txt")).toBe("plaintext");
});

test("references", () => {
  expect(parseRef("src/main.go:12-20")).toEqual({ path: "src/main.go", start: 12, end: 20 });
  expect(parseRef("src/main.go:7")).toEqual({ path: "src/main.go", start: 7, end: 7 });
  expect(parseRef("README.md")).toEqual({ path: "README.md" });
  expect(lineReference("backend", "src/main.go", 12, 20)).toBe("@backend:src/main.go:12-20");
  expect(lineReference("backend", "src/main.go", 7, 7)).toBe("@backend:src/main.go:7");
});

test("ranges for large files", () => {
  expect(rangeFor(5 * 1024 * 1024, "start")).toBe("bytes=0-1048575");
  expect(rangeFor(5 * 1024 * 1024, "end")).toBe(`bytes=${4 * 1024 * 1024}-${5 * 1024 * 1024 - 1}`);
});

test("My files: the message link and the channel filter", async () => {
  const { channelOptions, messageLink } = await import("./model");
  const ch = { id: "c1", name: "general", kind: "public" };
  expect(messageLink({ channel: ch, message_id: "m1" })).toBe("/c/c1");
  expect(messageLink({ channel: ch, message_id: "m2", thread_root_id: "r1" })).toBe("/c/c1?thread=r1");
  expect(messageLink({ message_id: "m1" })).toBeNull();
  expect(messageLink({ channel: ch })).toBeNull();
  const opts = channelOptions([{ channel: ch }, { channel: { id: "d1", name: "dm-abc", kind: "dm" } }, {}], new Map([["c0", "#alpha"]]), "DM");
  expect(new Map(opts)).toEqual(new Map([["c0", "#alpha"], ["d1", "DM"], ["c1", "#general"]]));
  const names = [...opts.values()];
  expect(names).toEqual([...names].sort((a, b) => a.localeCompare(b)));
});
