import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { CommandPalette } from "./CommandPalette";
import { openOverlay } from "./overlays";

const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });

function Where() {
  const l = useLocation();
  return <p data-testid="where">{l.pathname + l.search}</p>;
}

function setup() {
  vi.stubGlobal("fetch", vi.fn(async () => json({})));
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route
            path="*"
            element={
              <>
                <CommandPalette />
                <Where />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  act(() => openOverlay("palette"));
}

afterEach(() => {
  act(() => openOverlay(null));
  vi.unstubAllGlobals();
});

const field = () => screen.getByRole("combobox", { name: /Ugrás beszélgetésre/ });

test("typed text offers a search item, and Enter on it opens the search page with the text", () => {
  setup();
  expect(screen.queryByRole("option", { name: /Keresés:/ })).toBeNull();
  fireEvent.change(field(), { target: { value: "postgres verzió" } });
  expect(screen.getByRole("option", { name: /Keresés: postgres verzió/ })).toBeTruthy();
  fireEvent.keyDown(field(), { key: "Enter" });
  expect(screen.getByTestId("where").textContent).toBe("/search?q=postgres+verzi%C3%B3");
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("Ctrl+Enter searches even when a page matches first", () => {
  setup();
  fireEvent.change(field(), { target: { value: "wiki" } });
  expect(screen.getAllByRole("option")[0]!.textContent).toContain("Wiki");
  fireEvent.keyDown(field(), { key: "Enter", ctrlKey: true });
  expect(screen.getByTestId("where").textContent).toBe("/search?q=wiki");
});

test("plain Enter still jumps to the best match", () => {
  setup();
  fireEvent.change(field(), { target: { value: "döntések" } });
  fireEvent.keyDown(field(), { key: "Enter" });
  expect(screen.getByTestId("where").textContent).toBe("/decisions");
});
