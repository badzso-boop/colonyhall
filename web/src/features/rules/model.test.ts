import { lengthState, renderRules, segments } from "./model";

test("render fills the placeholders like the server and keeps unknown ones", () => {
  expect(renderRules("You are @{handle}. Admins: {admins}. {workspace} {other}", "@backend", ["norbi", "@anna", ""], "Colonyhall")).toBe(
    "You are @backend. Admins: @norbi, @anna. Colonyhall {other}",
  );
});

test("segments mark the placeholders", () => {
  expect(segments("Hi {handle}, ask {admins}{x}")).toEqual([{ text: "Hi " }, { placeholder: "handle" }, { text: ", ask " }, { placeholder: "admins" }, { text: "{x}" }]);
});

test("the counter turns amber from 90 % and counts characters, not bytes", () => {
  expect(lengthState("é".repeat(7199))).toBe("normal");
  expect(lengthState("é".repeat(7200))).toBe("near");
  expect(lengthState("é".repeat(8001))).toBe("over");
});
