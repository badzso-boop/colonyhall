import { act, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import App from "./App";
import i18n from "./i18n";

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  );
}

afterEach(async () => {
  await act(() => i18n.changeLanguage("hu"));
});

test("renders the scaffold in Hungarian by default, on any route", () => {
  renderAt("/c/some-channel");
  expect(screen.getByRole("heading", { name: "A PWA alapja kész" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "Fejlesztői oldal megnyitása" }).getAttribute("href")).toBe("/");
});

test("switches to English", async () => {
  renderAt("/");
  await act(() => i18n.changeLanguage("en"));
  expect(screen.getByRole("heading", { name: "The PWA foundation is ready" })).toBeTruthy();
});
