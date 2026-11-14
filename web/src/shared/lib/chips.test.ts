import { chipRefs, messageChipRef, splitChips } from "./chips";

const id = "0b5c2a8e-1f2d-4c3b-9a8e-7d6c5b4a3f21";

test("plain text stays one part", () => {
  expect(splitChips("nothing here")).toEqual([{ type: "text", text: "nothing here" }]);
  expect(splitChips("")).toEqual([]);
});

test("question and decision numbers become chips", () => {
  expect(splitChips("see Q-12 and D-3.")).toEqual([
    { type: "text", text: "see " },
    { type: "chip", kind: "question", ref: "Q-12", raw: "Q-12" },
    { type: "text", text: " and " },
    { type: "chip", kind: "decision", ref: "D-3", raw: "D-3" },
    { type: "text", text: "." },
  ]);
});

test("numbers inside words or identifiers are not chips", () => {
  for (const s of ["XQ-12", "Q-12a", "foo_D-4", "q-12", "Q-", "ABD-7", "Q-12_x", "x-Q-3"]) {
    expect(splitChips(s).every((p) => p.type === "text")).toBe(true);
  }
});

test("error and approval references need a full id", () => {
  expect(splitChips(`I ran into an error [error:${id}]`)).toEqual([
    { type: "text", text: "I ran into an error " },
    { type: "chip", kind: "error", ref: id, raw: `[error:${id}]` },
  ]);
  expect(splitChips("[error:123]")).toEqual([{ type: "text", text: "[error:123]" }]);
  expect(splitChips(`[approval:${id}]`)[0]).toMatchObject({ kind: "approval", ref: id });
});

test("chipRefs lists one kind once", () => {
  expect(chipRefs(`[approval:${id}] Q-1 [approval:${id}] Q-2 Q-1`, "question")).toEqual(["Q-1", "Q-2"]);
  expect(chipRefs(`[approval:${id}] again [approval:${id}]`, "approval")).toEqual([id]);
});

test("a message chip links to a message, with its thread when given", () => {
  const ch = "0b5c2a8e-1f2d-4c3b-9a8e-7d6c5b4a3f21";
  const msg = "1b5c2a8e-1f2d-4c3b-9a8e-7d6c5b4a3f22";
  const root = "2b5c2a8e-1f2d-4c3b-9a8e-7d6c5b4a3f23";
  const parts = splitChips(`⏰ Emlékeztető: x\n[message:${ch}/${msg}] és [message:${ch}/${msg}/${root}]`);
  const chips = parts.filter((p) => p.type === "chip");
  expect(chips.map((c) => c.type === "chip" && c.kind)).toEqual(["message", "message"]);
  expect(messageChipRef(chips[0]!.type === "chip" ? chips[0]!.ref : "")).toEqual({ channel_id: ch, id: msg });
  expect(messageChipRef(chips[1]!.type === "chip" ? chips[1]!.ref : "")).toEqual({ channel_id: ch, id: msg, thread_root_id: root });
  expect(splitChips("[message:not-a-uuid/x]")).toEqual([{ type: "text", text: "[message:not-a-uuid/x]" }]);
});
