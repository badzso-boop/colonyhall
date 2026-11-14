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

test("a draft's uploads come back as done, ready to send", () => {
  const s = trayReducer([], { type: "restore", items: [{ key: 7, file: new File([], "plan.txt"), fileId: "f7", size: 2048 }] });
  expect(s[0]).toMatchObject({ status: "done", progress: 1, fileId: "f7", size: 2048 });
  expect(trayState(s)).toEqual({ busy: false, fileIds: ["f7"], failed: false });
});
