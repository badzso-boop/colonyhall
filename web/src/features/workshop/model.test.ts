// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { describe, expect, it } from "vitest";
import { slugFrom, validSlug, validURL } from "./model";

describe("workshop model", () => {
  it("validates short names like the server", () => {
    expect(validSlug("jira")).toBe(true);
    expect(validSlug("j")).toBe(false);
    expect(validSlug("Jira")).toBe(false);
    expect(validSlug("ji__ra")).toBe(false);
    expect(validSlug("1jira")).toBe(false);
    expect(validSlug("a".repeat(17))).toBe(false);
  });
  it("suggests a short name from a title", () => {
    expect(slugFrom("Jira Cloud")).toBe("jira_cloud");
    expect(slugFrom("Könyvelő API")).toBe("konyvelo_api");
    expect(slugFrom("2. Gitea")).toBe("gitea");
    expect(validSlug(slugFrom("Nagyon hosszú kapcsolatnév itt"))).toBe(true);
  });
  it("accepts http(s) addresses only", () => {
    expect(validURL("https://mcp.example.com/mcp")).toBe(true);
    expect(validURL("ftp://x.example.com")).toBe(false);
    expect(validURL("https://user:pw@x.example.com")).toBe(false);
    expect(validURL("nope")).toBe(false);
  });
});
