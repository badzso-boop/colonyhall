// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { parseCommand } from "./commands";

test("schedule commands", () => {
  expect(parseCommand("/schedule 2026.11.02. 9:00 standup")).toEqual({ kind: "schedule", at: "2026-11-02 09:00", body: "standup" });
  expect(parseCommand("/daily 07:30 reggeli összefoglaló")).toEqual({ kind: "schedule", cron: "30 7 * * *", body: "reggeli összefoglaló" });
  expect(parseCommand('/cron "0 9 * * 1-5" hétköznap')).toEqual({ kind: "schedule", cron: "0 9 * * 1-5", body: "hétköznap" });
  expect(parseCommand("/daily soon")).toEqual({ kind: "invalid", name: "schedule" });
});

test("answer and plain messages", () => {
  expect(parseCommand("/answer Q-12 igen, mehet")).toEqual({ kind: "answer", question: "Q-12", answer: "igen, mehet" });
  expect(parseCommand("/answer Q-12")).toEqual({ kind: "invalid", name: "answer" });
  expect(parseCommand("szia /daily")).toEqual({ kind: "message" });
  expect(parseCommand("/unknown x")).toEqual({ kind: "message" });
});
