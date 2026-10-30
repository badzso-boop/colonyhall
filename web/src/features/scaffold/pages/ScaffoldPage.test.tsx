import { act, render, screen } from "@testing-library/react";
import { i18n } from "@/shared/i18n";
import ScaffoldPage from "./ScaffoldPage";

afterEach(async () => {
  await act(() => i18n.changeLanguage("hu"));
});

test("shows the page in Hungarian by default, with a link to the dev page", () => {
  render(<ScaffoldPage />);
  expect(screen.getByRole("heading", { name: "A PWA alapja kész" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "Fejlesztői oldal megnyitása" }).getAttribute("href")).toBe("/");
  expect(screen.getByText("Colonyhall")).toBeTruthy();
});

test("switches to English", async () => {
  render(<ScaffoldPage />);
  await act(() => i18n.changeLanguage("en"));
  expect(screen.getByRole("heading", { name: "The PWA foundation is ready" })).toBeTruthy();
});
