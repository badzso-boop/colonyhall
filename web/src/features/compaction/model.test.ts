// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { applyCompactionEvent, liveHere, parseCompactionEvent, previewLines, type Live } from "./model";

test("parseCompactionEvent checks the payload", () => {
  expect(parseCompactionEvent(null)).toBeNull();
  expect(parseCompactionEvent({ channel_id: "c" })).toBeNull();
  expect(parseCompactionEvent({ compaction_id: 1, channel_id: "c" })).toBeNull();
  expect(parseCompactionEvent({ compaction_id: "k", channel_id: "c", thread_root_id: "", error_code: "timeout" })).toEqual({
    compactionId: "k", channelId: "c", threadRootId: null, errorCode: "timeout",
  });
  expect(parseCompactionEvent({ compaction_id: "k", channel_id: "c", thread_root_id: "t" })?.threadRootId).toBe("t");
});

test("created → pending, failed → error in the same place, done → gone", () => {
  let live: Record<string, Live> = {};
  live = applyCompactionEvent(live, "compaction.created", { compactionId: "k", channelId: "c", threadRootId: "t", errorCode: null });
  expect(live.k).toMatchObject({ state: "pending", threadRootId: "t" });
  const failed = applyCompactionEvent(live, "compaction.failed", { compactionId: "k", channelId: "c", threadRootId: null, errorCode: "timeout" });
  expect(failed.k).toMatchObject({ state: "failed", threadRootId: "t", errorCode: "timeout" });
  expect(applyCompactionEvent(live, "compaction.done", { compactionId: "k", channelId: "c", threadRootId: "t", errorCode: null })).toEqual({});
  expect(applyCompactionEvent(live, "compaction.other", { compactionId: "k", channelId: "c", threadRootId: null, errorCode: null })).toBe(live);
});

test("liveHere separates the main stream from threads", () => {
  const live: Record<string, Live> = {
    a: { id: "a", channelId: "c", threadRootId: null, state: "pending", errorCode: null },
    b: { id: "b", channelId: "c", threadRootId: "t", state: "pending", errorCode: null },
    x: { id: "x", channelId: "other", threadRootId: null, state: "pending", errorCode: null },
  };
  expect(liveHere(live, "c", null).map((l) => l.id)).toEqual(["a"]);
  expect(liveHere(live, "c", "t").map((l) => l.id)).toEqual(["b"]);
});

test("previewLines drops heading marks and empty lines", () => {
  expect(previewLines("# Összefoglaló\n\n## Döntések\n- egy\n- kettő\n- három", 3)).toEqual(["Összefoglaló", "Döntések", "- egy"]);
});
