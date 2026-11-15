// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { TooltipProvider } from "@/shared/ui";
import InvitePage from "./InvitePage";

const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });

type Call = { path: string; body: Record<string, unknown> };

function setup(hash: string, respond: (c: Call) => Response) {
  window.location.hash = hash;
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const c = { path: new URL(req.url).pathname, body: (await req.clone().json()) as Record<string, unknown> };
      calls.push(c);
      return respond(c);
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TooltipProvider>
        <MemoryRouter initialEntries={["/invite"]}>
          <Routes>
            <Route path="invite" element={<InvitePage />} />
            <Route path="/" element={<p>home</p>} />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const info = { workspace_name: "Gamma Kft", expires_at: "2026-11-22T10:00:00Z" };
const session = { actor: { id: "a", handle: "gina", display_name: "Gina", kind: "human", role: "owner" }, csrf_token: "c" };

test("without a token the page says the link is not valid, and asks nothing", async () => {
  const calls = setup("", () => json(info));
  await screen.findByText("A meghívó nem érvényes");
  expect(calls).toEqual([]);
});

test("a used or expired invite says so", async () => {
  setup("#col_used", () => json({ type: "invite_invalid", title: "Gone", status: 410 }, 410));
  await screen.findByText(/már nem érvényes/);
});

test("the token goes in the body; the owner is created and logged in", async () => {
  const calls = setup("#col_token123", (c) => (c.path.endsWith("/invites/lookup") ? json(info) : json(session)));
  await screen.findByText("Meghívó: Gamma Kft");
  expect(calls[0]).toEqual({ path: "/api/v1/invites/lookup", body: { token: "col_token123" } });

  const submit = screen.getByRole("button", { name: "Tulajdonos leszek" });
  fireEvent.change(screen.getByLabelText(/^Felhasználónév/), { target: { value: "@Gina" } });
  const [password, again] = screen.getAllByLabelText(/^Jelszó/);
  fireEvent.change(password!, { target: { value: "gamma-owner-password" } });
  fireEvent.change(again!, { target: { value: "gamma-owner-passwor" } });
  expect((submit as HTMLButtonElement).disabled).toBe(true);
  await screen.findByText("A két jelszó nem egyezik.");
  fireEvent.change(again!, { target: { value: "gamma-owner-password" } });
  fireEvent.click(submit);
  await screen.findByText("home");
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]).toEqual({ path: "/api/v1/invites/accept", body: { token: "col_token123", handle: "gina", password: "gamma-owner-password" } });
});
