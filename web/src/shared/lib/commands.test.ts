import { availableCommands, matchCommands, parseCommand, parseWhen, registerCommand } from "./commands";

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
});

test("an unknown command is refused, never sent", () => {
  expect(parseCommand("/unknown x")).toEqual({ kind: "unknown", name: "unknown" });
  expect(parseCommand("/Foo")).toEqual({ kind: "unknown", name: "foo" });
});

test("// sends a message starting with /, and a path is a message", () => {
  expect(parseCommand("//pin this")).toEqual({ kind: "message", body: "/pin this" });
  expect(parseCommand("/tmp/x is full")).toEqual({ kind: "message" });
});

test("remind", () => {
  expect(parseCommand("/remind 20m nézd meg a buildet")).toEqual({ kind: "remind", when: { kind: "in", minutes: 20 }, text: "nézd meg a buildet" });
  expect(parseCommand("/remind in 2h deploy")).toEqual({ kind: "remind", when: { kind: "in", minutes: 120 }, text: "deploy" });
  expect(parseCommand("/remind tomorrow standup")).toEqual({ kind: "remind", when: { kind: "at", days: 1, hour: 9, minute: 0 }, text: "standup" });
  expect(parseCommand("/remind holnap 8:30 standup\nmásodik sor")).toEqual({
    kind: "remind",
    when: { kind: "at", days: 1, hour: 8, minute: 30 },
    text: "standup\nmásodik sor",
  });
  expect(parseCommand("/remind hétfő heti terv")).toEqual({ kind: "remind", when: { kind: "at", days: "monday", hour: 9, minute: 0 }, text: "heti terv" });
  expect(parseCommand("/remind 14:00 hívás")).toEqual({ kind: "remind", when: { kind: "at", days: 0, hour: 14, minute: 0 }, text: "hívás" });
  expect(parseCommand("/remind me about this")).toEqual({ kind: "remindThis" });
  expect(parseCommand("/remind 20m")).toEqual({ kind: "invalid", name: "remind" });
  expect(parseCommand("/remind soon x")).toEqual({ kind: "invalid", name: "remind" });
  expect(parseCommand("/remind 25:00 x")).toEqual({ kind: "invalid", name: "remind" });
  expect(parseWhen(["0m"])).toBeNull();
});

test("the other commands", () => {
  expect(parseCommand("/pin")).toEqual({ kind: "pin" });
  expect(parseCommand("/pin x")).toEqual({ kind: "invalid", name: "pin" });
  expect(parseCommand("/save")).toEqual({ kind: "save" });
  expect(parseCommand("/mute")).toEqual({ kind: "mute" });
  expect(parseCommand("/unmute")).toEqual({ kind: "unmute" });
  expect(parseCommand("/notify mentions")).toEqual({ kind: "notify", level: "mentions" });
  expect(parseCommand("/notify loud")).toEqual({ kind: "invalid", name: "notify" });
  expect(parseCommand("/status Szabadságon")).toEqual({ kind: "status", text: "Szabadságon" });
  expect(parseCommand("/status clear")).toEqual({ kind: "status", text: "" });
  expect(parseCommand("/status")).toEqual({ kind: "invalid", name: "status" });
  expect(parseCommand("/stop @coder")).toEqual({ kind: "stop", handle: "coder" });
  expect(parseCommand("/stop")).toEqual({ kind: "invalid", name: "stop" });
  expect(parseCommand("/emergency")).toEqual({ kind: "emergency" });
  expect(parseCommand("/search  rag terv ")).toEqual({ kind: "search", query: "rag terv" });
  expect(parseCommand("/thread")).toEqual({ kind: "thread" });
  expect(parseCommand("/drafts")).toEqual({ kind: "open", view: "drafts" });
  expect(parseCommand("/saved")).toEqual({ kind: "open", view: "saved" });
  expect(parseCommand("/activity")).toEqual({ kind: "open", view: "activity" });
  expect(parseCommand("/help")).toEqual({ kind: "help" });
});

test("admin commands are listed for admins only", () => {
  const names = (admin: boolean) => availableCommands({ admin }).map((c) => c.name);
  expect(names(false)).not.toContain("emergency");
  expect(names(false)).not.toContain("stop");
  expect(names(true)).toContain("emergency");
  expect(matchCommands("/em", { admin: false })).toEqual([]);
  expect(matchCommands("/em", { admin: true }).map((c) => c.name)).toEqual(["emergency"]);
  expect(matchCommands("/s", { admin: false }).map((c) => c.name)).toEqual(["save", "status", "search", "saved", "schedule"]);
  expect(matchCommands("/save x", { admin: false })).toEqual([]);
});

test("later rounds register commands", () => {
  registerCommand({ name: "poll", usage: "/poll …" });
  expect(matchCommands("/po", { admin: false }).map((c) => c.name)).toEqual(["poll"]);
});
