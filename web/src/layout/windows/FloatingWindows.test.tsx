import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { dispatchWindow, initial, useWindows } from "@/shared/windows";
import { FloatingWindows } from "./FloatingWindows";

function setup(matchesWide = true) {
  vi.stubGlobal("matchMedia", (q: string) => ({ matches: matchesWide || q.includes("768") === false ? matchesWide : false, addEventListener() {}, removeEventListener() {} }));
  useWindows.setState({ wins: initial({ w: 1440, h: 900 }) });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/c/x"]}>
        <Routes>
          <Route path="/tasks" element={<p>Feladatok oldal</p>} />
          <Route path="*" element={<FloatingWindows />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

test("a window opens, fades by mode, minimises to a circle and comes back", () => {
  setup();
  act(() => dispatchWindow({ type: "open", kind: "tasks" }));
  const dialog = screen.getByRole("dialog", { name: "Feladatok" });
  expect(dialog.getAttribute("aria-modal")).toBe("false");
  expect(dialog.className).toContain("opacity-55");

  fireEvent.click(screen.getByRole("button", { name: "Halványítás: Áttetsző" }));
  expect(screen.getByRole("dialog").className).toContain("opacity-20");
  fireEvent.click(screen.getByRole("button", { name: "Halványítás: Halvány" }));
  expect(screen.getByRole("dialog").className).not.toMatch(/opacity-(55|20)/);

  fireEvent.click(screen.getByRole("button", { name: "Kis méret" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  const bubble = screen.getByRole("button", { name: "Feladatok visszaállítása" });
  fireEvent.keyDown(bubble, { key: "Enter" });
  expect(screen.getByRole("dialog", { name: "Feladatok" })).toBeTruthy();
});

test("Teljes oldal goes to the page and closes the window", () => {
  setup();
  act(() => dispatchWindow({ type: "open", kind: "tasks" }));
  fireEvent.click(screen.getByRole("button", { name: "Teljes oldal" }));
  expect(screen.getByText("Feladatok oldal")).toBeTruthy();
  expect(useWindows.getState().wins.tasks.state).toBe("closed");
});

test("Alt+arrows move the window from its focused title bar", () => {
  setup();
  act(() => dispatchWindow({ type: "open", kind: "notes" }));
  act(() => dispatchWindow({ type: "move", kind: "notes", x: 400, y: 300 }));
  const title = screen.getByTitle(/Alt\+nyilak/);
  fireEvent.keyDown(title, { key: "ArrowRight", altKey: true });
  fireEvent.keyDown(title, { key: "ArrowDown", altKey: true, shiftKey: true });
  const w = useWindows.getState().wins.notes;
  expect(w.rect.x).toBe(416);
  expect(w.rect.h).toBe(420 + 16);
});

test("on a phone an open window is a sheet", () => {
  setup(false);
  act(() => dispatchWindow({ type: "open", kind: "calendar" }));
  expect(screen.getByRole("dialog", { name: "Naptár" }).getAttribute("aria-modal")).not.toBe("false");
});

test("bringing a window to the front keeps the DOM order, so the click that did it still lands", () => {
  setup();
  act(() => {
    dispatchWindow({ type: "open", kind: "tasks" });
    dispatchWindow({ type: "open", kind: "calendar" });
  });
  const order = () => screen.getAllByRole("dialog").map((d) => d.getAttribute("aria-labelledby"));
  const before = order();
  const tasks = screen.getByRole("dialog", { name: "Feladatok" });
  fireEvent.pointerDown(tasks);
  expect(useWindows.getState().wins.tasks.z).toBeGreaterThan(useWindows.getState().wins.calendar.z);
  expect(order()).toEqual(before);
  expect(screen.getByRole("dialog", { name: "Feladatok" })).toBe(tasks);
});
