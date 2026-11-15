// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { TooltipProvider } from "@/shared/ui";
import OperatorLoginPage from "./OperatorLoginPage";

const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });

function setup(respond: () => Response) {
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      bodies.push(await req.clone().json());
      return respond();
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TooltipProvider>
        <MemoryRouter initialEntries={["/login"]}>
          <Routes>
            <Route path="login" element={<OperatorLoginPage />} />
            <Route path="/" element={<p>customers</p>} />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByLabelText("Felhasználónév"), { target: { value: " Ops " } });
  fireEvent.change(screen.getByLabelText("Jelszó"), { target: { value: "operator-password" } });
  fireEvent.change(screen.getByLabelText(/^Kód/), { target: { value: "123 456" } });
  fireEvent.click(screen.getByRole("button", { name: "Belépés" }));
  return bodies;
}

afterEach(() => vi.unstubAllGlobals());

test("password and code go in one request; a failure says nothing about which was wrong", async () => {
  const bodies = setup(() => json({ title: "Unauthorized", status: 401 }, 401));
  await screen.findByText("Hibás felhasználónév, jelszó vagy kód.");
  expect(bodies).toEqual([{ handle: "ops", password: "operator-password", code: "123456" }]);
  expect((screen.getByLabelText(/^Kód/) as HTMLInputElement).value).toBe("");
});

test("a lockout is told apart", async () => {
  setup(() => json({ title: "Too many requests", status: 429 }, 429));
  await screen.findByText(/Túl sok sikertelen/);
});

test("a login goes to the customers", async () => {
  setup(() =>
    json({ operator: { id: "o", handle: "ops", display_name: "Ops" }, csrf_token: "c", expires_at: "2026-11-15T18:00:00Z", dev_access: false, backup_codes_left: 10 }),
  );
  await screen.findByText("customers");
});
