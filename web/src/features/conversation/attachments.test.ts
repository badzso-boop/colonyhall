// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { trayReducer, trayState, type Attachment } from "./attachments";

const f = (name: string) => new File(["x"], name);

test("the tray: add, progress, done, error, retry, remove", () => {
  let s: Attachment[] = trayReducer([], { type: "add", items: [{ key: 1, file: f("a.png") }, { key: 2, file: f("b.txt") }] });
  expect(trayState(s)).toEqual({ busy: true, fileIds: [], failed: false });
  s = trayReducer(s, { type: "progress", key: 1, progress: 0.64 });
  expect(s[0]!.progress).toBe(0.64);
  s = trayReducer(s, { type: "done", key: 1, fileId: "f1" });
  s = trayReducer(s, { type: "error", key: 2, error: "too_large" });
  expect(trayState(s)).toEqual({ busy: false, fileIds: ["f1"], failed: true });
  s = trayReducer(s, { type: "retry", key: 2 });
  expect(s[1]).toMatchObject({ status: "uploading", progress: 0, error: undefined });
  s = trayReducer(s, { type: "remove", key: 2 });
  expect(trayState(s)).toEqual({ busy: false, fileIds: ["f1"], failed: false });
  expect(trayReducer(s, { type: "clear" })).toEqual([]);
});
