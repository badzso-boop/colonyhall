// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { blocker, clampWidth, dragWidth, MIN_WIDTH, previewFor, previewForLink, type Preview } from "./model";

const base: Preview = {
  id: "p1",
  workspace_id: "w",
  agent_id: "ag",
  host_id: "h",
  channel_id: "c1",
  name: "web",
  compose_path: "compose.yaml",
  state: "running",
  url: "http://100.64.0.5:41000",
  proxy_url: "http://colony.example.com:8081/api/v1/previews/p1/proxy/",
  expires_at: "2026-11-23T12:00:00Z",
  created_at: "2026-11-23T10:00:00Z",
  updated_at: "2026-11-23T10:00:00Z",
};

test("the width stays between 320 px and 80% of the window", () => {
  expect(clampWidth(100, 1600)).toBe(MIN_WIDTH);
  expect(clampWidth(5000, 1600)).toBe(1280);
  expect(clampWidth(640, 1600)).toBe(640);
  expect(clampWidth(Number.NaN, 1600)).toBe(500);
  // A window too small for both: the minimum wins.
  expect(clampWidth(500, 300)).toBe(MIN_WIDTH);
});

test("dragging the divider left widens the column", () => {
  expect(dragWidth(500, 1000, 900, 1600)).toBe(600);
  expect(dragWidth(500, 1000, 1300, 1600)).toBe(MIN_WIDTH);
});

test("the conversation's preview is its newest live one", () => {
  const old = { ...base, id: "old", updated_at: "2026-11-23T09:00:00Z" };
  const stopped = { ...base, id: "stopped", state: "stopped" as const, updated_at: "2026-11-23T11:00:00Z" };
  const other = { ...base, id: "other", channel_id: "c2", updated_at: "2026-11-23T11:30:00Z" };
  expect(previewFor([old, base, stopped, other], "c1")?.id).toBe("p1");
  expect(previewFor([stopped], "c1")).toBeNull();
  expect(previewFor([{ ...base, thread_root_id: "t1" }], "c1", "t2")).toBeNull();
  expect(previewFor([{ ...base, thread_root_id: "t1" }], "c1", "t1")?.id).toBe("p1");
});

test("a link opens its preview by the proxy or the guard's address", () => {
  expect(previewForLink([base], "http://100.64.0.5:41000/")?.id).toBe("p1");
  expect(previewForLink([base], "http://100.64.0.5:41000/about")?.id).toBe("p1");
  expect(previewForLink([base], "http://colony.example.com:8081/api/v1/previews/p1/proxy/x.html")?.id).toBe("p1");
  expect(previewForLink([base], "http://100.64.0.5:41001/")).toBeNull();
  expect(previewForLink([base], "http://colony.example.com:8081/api/v1/previews/p2/proxy/")).toBeNull();
  expect(previewForLink([base], "not a url")).toBeNull();
});

test("the frame shows only for a running preview with a proxy address", () => {
  expect(blocker(base)).toBeNull();
  expect(blocker({ ...base, state: "requested" })).toBe("offline");
  expect(blocker({ ...base, proxy_url: undefined })).toBe("noProxy");
});
