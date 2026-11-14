// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Schemas } from "@/shared/api";
import { cardTone, claudePausedAgents, isRateLimitNote, repoOf, terminalNotice } from "./messageParts";

test("terminalNotice reads the guard's terminal messages", () => {
  expect(terminalNotice("⚙️ build kész (exit 0)")).toBe("build");
  expect(terminalNotice("⚠️ e2e-tests hibával állt le (exit 1)")).toBe("e2e-tests");
  expect(terminalNotice("⛔ dev leállítva")).toBe("dev");
  expect(terminalNotice("🖥 watch elindult")).toBe("watch");
  expect(terminalNotice("the build kész")).toBeNull();
  expect(terminalNotice("⚙️ Build kész")).toBeNull();
});

test("a rate-limit note is an agent's ⏸ message", () => {
  expect(isRateLimitNote("⏸ A Claude 5 órás kerete 95%-on, 17:25-kor folytatom.", "agent")).toBe(true);
  expect(isRateLimitNote("⏸ szünet", "human")).toBe(false);
  expect(isRateLimitNote("A ⏸ jel", "agent")).toBe(false);
});

test("claudePausedAgents: claude-code agents of paused machines", () => {
  const host = (paused: boolean, agents: Array<[string, "claude-code" | "antigravity"]>) =>
    ({ report: { roots: [], clients: [], rejected: [], claude_limits: { paused } }, agents: agents.map(([agent_id, client]) => ({ agent_id, client })) }) as unknown as Schemas["Host"];
  const set = claudePausedAgents([host(true, [["a", "claude-code"], ["b", "antigravity"]]), host(false, [["c", "claude-code"]])]);
  expect([...set]).toEqual(["a"]);
});

test("repoOf and cardTone", () => {
  expect(repoOf("https://gitea.example/owner/repo/pulls/3")).toBe("owner/repo");
  expect(repoOf("https://gitea.example/owner")).toBeNull();
  expect(repoOf("not a url")).toBeNull();
  expect(cardTone("open")).toBe("success");
  expect(cardTone("Merged")).toBe("info");
  expect(cardTone("closed")).toBe("danger");
  expect(cardTone("")).toBe("neutral");
});
