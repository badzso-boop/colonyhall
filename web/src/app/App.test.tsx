import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { App } from "./App";

test("any route renders a feature page through the lazy routes", async () => {
  render(
    <MemoryRouter initialEntries={["/c/some-channel"]}>
      <App />
    </MemoryRouter>,
  );
  expect(await screen.findByRole("heading", { name: "A PWA alapja kész" })).toBeTruthy();
});
