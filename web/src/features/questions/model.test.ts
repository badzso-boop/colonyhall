import { answerSuggestions, answeredSince, countdown, isQuestion, waitingForHumans, type Question } from "./model";

const q = (over: Partial<Question>): Question => ({
  id: "q1", number: 1, formatted_number: "Q-1", workspace_id: "w", asked_by: "a", asked_by_handle: "agent",
  to_human: true, channel_id: "c", channel_name: "general", thread_root_id: "t", text: "?", options: [],
  state: "open", created_at: "2026-10-30T08:00:00Z", ...over,
});

test("waiting: open to humans plus escalated, once each, oldest first", () => {
  const a = q({ id: "a", created_at: "2026-10-30T09:00:00Z" });
  const toAgent = q({ id: "b", to_human: false });
  const esc = q({ id: "c", to_human: false, state: "escalated", created_at: "2026-10-30T07:00:00Z" });
  expect(waitingForHumans([a, toAgent], [esc, a]).map((x) => x.id)).toEqual(["c", "a"]);
});

test("answered in the last 7 days, newest first", () => {
  const now = new Date("2026-11-01T12:00:00Z");
  const list = [
    q({ id: "old", state: "answered", answered_at: "2026-10-20T00:00:00Z" }),
    q({ id: "x", state: "answered", answered_at: "2026-10-30T00:00:00Z" }),
    q({ id: "y", state: "answered", answered_at: "2026-10-31T00:00:00Z" }),
    q({ id: "open" }),
  ];
  expect(answeredSince(list, now).map((x) => x.id)).toEqual(["y", "x"]);
});

test("countdown", () => {
  const now = new Date("2026-11-01T12:00:00Z");
  expect(countdown(null, now)).toBeNull();
  expect(countdown("2026-11-01T12:04:05Z", now)).toEqual({ text: "4:05", overdue: false });
  expect(countdown("2026-11-01T13:02:03Z", now)).toEqual({ text: "1:02:03", overdue: false });
  expect(countdown("2026-11-01T11:00:00Z", now)).toEqual({ text: "0:00", overdue: true });
});

test("isQuestion accepts the id and the number forms", () => {
  const x = q({ id: "uuid", number: 12 });
  for (const r of ["uuid", "Q-12", "q12", "12"]) expect(isQuestion(x, r)).toBe(true);
  for (const r of ["Q-1", "D-12", "x"]) expect(isQuestion(x, r)).toBe(false);
});

test("answer suggestions while typing the number", () => {
  const open = [q({ id: "a", number: 12 }), q({ id: "b", number: 13 }), q({ id: "c", number: 2 })];
  expect(answerSuggestions("/answer ", open).map((x) => x.number)).toEqual([12, 13, 2]);
  expect(answerSuggestions("/answer Q-1", open).map((x) => x.number)).toEqual([12, 13]);
  expect(answerSuggestions("/answer q2", open).map((x) => x.number)).toEqual([2]);
  expect(answerSuggestions("/answer Q-12 yes", open)).toEqual([]);
  expect(answerSuggestions("/answer", open)).toEqual([]);
  expect(answerSuggestions("hello", open)).toEqual([]);
});
