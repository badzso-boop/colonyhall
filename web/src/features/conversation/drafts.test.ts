// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { describe, expect, it } from "vitest";
import { draftKey, isEmptyDraft, parseLocalDraft, pickDraft } from "./drafts";

const at = (s: string) => `2026-11-14T10:00:${s}Z`;

describe("drafts", () => {
  it("keys a place by conversation and thread", () => {
    expect(draftKey("c1")).toBe("c1:");
    expect(draftKey("c1", "r1")).toBe("c1:r1");
    expect(draftKey("c1", "r1")).not.toBe(draftKey("c1"));
  });

  it("treats whitespace without files as empty", () => {
    expect(isEmptyDraft("  \n", [])).toBe(true);
    expect(isEmptyDraft("  ", [{}])).toBe(false);
    expect(isEmptyDraft("x", [])).toBe(false);
  });

  it("restores the server copy unless the local one is newer", () => {
    const server = { body: "server", files: [], at: at("05") };
    expect(pickDraft(server, { body: "older local", files: [], at: at("01") })).toBe(server);
    expect(pickDraft(server, { body: "same time", files: [], at: at("05") })).toBe(server);
    expect(pickDraft(server, { body: "offline", files: [], at: at("09") })?.body).toBe("offline");
    expect(pickDraft(undefined, { body: "only local", files: [], at: at("01") })?.body).toBe("only local");
    expect(pickDraft(undefined, undefined)).toBeUndefined();
  });

  it("ignores a broken local copy", () => {
    expect(parseLocalDraft("not json")).toBeUndefined();
    expect(parseLocalDraft(JSON.stringify({ body: 1, at: at("01") }))).toBeUndefined();
    expect(parseLocalDraft(JSON.stringify({ body: "x", at: "never" }))).toBeUndefined();
    expect(parseLocalDraft(JSON.stringify({ body: "x", at: at("01"), files: [{ id: 1 }] }))?.files).toEqual([]);
  });
});
