import { delegationOf, grantsHere, inputSummary, prettyInput, timeLeft, type Delegation, type Grant } from "./model";

const del = (over: Partial<Delegation>): Delegation => ({
  id: "d1", workspace_id: "w", message_id: "m1", channel_id: "c", from_agent_id: "a", to_agent_id: "b", question_id: "q1",
  state: "pending", created_at: "2026-11-01T10:00:00Z", ...over,
});

const grant = (over: Partial<Grant>): Grant => ({
  id: "g1", workspace_id: "w", channel_id: "c", agent_a_id: "a", agent_a_handle: "a", agent_b_id: "b", agent_b_handle: "b",
  delegation_id: "d1", approved_by: "h", approver_handle: "norbi", approved_at: "2026-11-01T10:00:00Z", last_used_at: "2026-11-01T10:00:00Z",
  expires_at: "2026-11-01T22:00:00Z", effective_expires_at: "2026-11-01T11:00:00Z", ...over,
});

test("delegationOf finds the source message or the Q message by client id", () => {
  const list = [del({ id: "x", message_id: "m9", question_id: "q9" }), del({})];
  expect(delegationOf(list, { id: "m1" })?.id).toBe("d1");
  expect(delegationOf(list, { id: "other", client_msg_id: "q-q9" })?.id).toBe("x");
  expect(delegationOf(list, { id: "none" })).toBeUndefined();
});

test("grantsHere: per thread, or the DM's own; never expired or revoked", () => {
  const now = new Date("2026-11-01T10:30:00Z");
  const dm = grant({ id: "dm" });
  const th = grant({ id: "th", thread_root_id: "t1" });
  const old = grant({ id: "old", effective_expires_at: "2026-11-01T10:00:00Z" });
  const rev = grant({ id: "rev", revoked_at: "2026-11-01T10:10:00Z" });
  const other = grant({ id: "other", channel_id: "c2" });
  const all = [dm, th, old, rev, other];
  expect(grantsHere(all, "c", null, true, now).map((g) => g.id)).toEqual(["dm"]);
  expect(grantsHere(all, "c", null, false, now)).toEqual([]);
  expect(grantsHere(all, "c", "t1", false, now).map((g) => g.id)).toEqual(["th"]);
});

test("inputSummary picks a telling field and shortens", () => {
  expect(inputSummary('{"command":"rm -rf build","description":"clean"}')).toBe("rm -rf build");
  expect(inputSummary('{"file_path":"/srv/x.go","content":"..."}')).toBe("/srv/x.go");
  expect(inputSummary('{"a":1}')).toBe('{"a":1}');
  expect(inputSummary("plain\n  text")).toBe("plain text");
  expect(inputSummary("x".repeat(200), 10)).toBe(`${"x".repeat(9)}…`);
});

test("prettyInput indents JSON and keeps other text", () => {
  expect(prettyInput('{"a":1}')).toBe('{\n  "a": 1\n}');
  expect(prettyInput("not json")).toBe("not json");
});

test("timeLeft", () => {
  const now = new Date("2026-11-01T10:00:00Z");
  expect(timeLeft("2026-11-01T10:01:05Z", now)).toBe("1:05");
  expect(timeLeft("2026-11-01T09:00:00Z", now)).toBe("0:00");
});
