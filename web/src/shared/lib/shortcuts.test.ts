import { comboOf, handleKey, keyLabels, registerShortcut } from "./shortcuts";

const key = (k: string, mods: Partial<KeyboardEvent> = {}, target?: HTMLElement) =>
  ({ key: k, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, target: target ?? document.body, preventDefault: vi.fn(), ...mods }) as unknown as KeyboardEvent;

test("comboOf normalises modifiers; ? stays ?", () => {
  expect(comboOf(key("K", { ctrlKey: true }))).toBe("mod+k");
  expect(comboOf(key("k", { metaKey: true }))).toBe("mod+k");
  expect(comboOf(key("ArrowDown", { altKey: true, shiftKey: true }))).toBe("alt+shift+arrowdown");
  expect(comboOf(key("?", { shiftKey: true }))).toBe("?");
});

test("duplicates in one scope are refused; another scope is fine", () => {
  const off = registerShortcut({ keys: "x", scope: "global", description: "a" });
  expect(() => registerShortcut({ keys: "x", scope: "global", description: "b" })).toThrow(/already registered/);
  const off2 = registerShortcut({ keys: "x", scope: "pane", description: "c" });
  off();
  off2();
});

test("shortcuts do not fire while typing, except mod+ and composer ones", () => {
  const plain = vi.fn();
  const mod = vi.fn();
  const composer = vi.fn();
  const offs = [
    registerShortcut({ keys: "r", scope: "conversation", description: "r", handler: plain }),
    registerShortcut({ keys: "mod+k", scope: "global", description: "k", handler: mod }),
    registerShortcut({ keys: "arrowup", scope: "composer", description: "u", handler: composer }),
  ];
  const input = document.createElement("textarea");
  expect(handleKey(key("r", {}, input))).toBe(false);
  expect(handleKey(key("r"))).toBe(true);
  expect(handleKey(key("k", { ctrlKey: true }, input))).toBe(true);
  expect(handleKey(key("ArrowUp", {}, input))).toBe(true);
  expect([plain, mod, composer].map((f) => f.mock.calls.length)).toEqual([1, 1, 1]);
  offs.forEach((o) => o());
});

test("keyLabels", () => {
  expect(keyLabels("mod+shift+f", false)).toEqual(["Ctrl", "Shift", "F"]);
  expect(keyLabels("mod+k", true)).toEqual(["⌘", "K"]);
  expect(keyLabels("alt+arrowup", false)).toEqual(["Alt", "↑"]);
});
