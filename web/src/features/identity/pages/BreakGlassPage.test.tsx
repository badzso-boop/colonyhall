// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import BreakGlassPage from "./BreakGlassPage";
import { BreakGlassBanner } from "../components/BreakGlassBanner";

const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
const actor = { id: "a1", kind: "human", handle: "colonyhall-operator", display_name: "Op", role: "admin", state: "active", created_at: "2026-11-15T09:00:00Z" };

function setup(hash: string, respond: (path: string, body: unknown) => Response) {
  window.location.hash = hash;
  const calls: Array<{ path: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      const body = req.method === "POST" ? await req.clone().json() : null;
      calls.push({ path, body });
      return respond(path, body);
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={["/break-glass"]}>
        <Routes>
          <Route path="break-glass" element={<BreakGlassPage />} />
          <Route path="/" element={<BreakGlassBanner />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  window.location.hash = "";
});

test("the link opens the read-only session once and the app shows the banner (round 51b)", async () => {
  const session = { actor, csrf_token: "c", break_glass: { id: "b1", expires_at: "2026-11-15T14:30:00Z", operator_handle: "Ops" } };
  const calls = setup("#tok123", (path) => (path.endsWith("/break-glass/redeem") || path.endsWith("/auth/session") ? json(session) : json({})));
  const banner = await screen.findByRole("status");
  expect(banner.textContent).toContain("(Ops)");
  expect(banner.textContent).toContain("csak olvasás");
  const redeems = calls.filter((c) => c.path.endsWith("/break-glass/redeem"));
  expect(redeems).toEqual([{ path: "/api/v1/break-glass/redeem", body: { token: "tok123" } }]);
  expect(window.location.hash).toBe("");
});

test("a used or expired link says so", async () => {
  setup("#old", () => json({ type: "break_glass_invalid", title: "Gone", status: 410 }, 410));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.getByRole("alert").textContent).toContain("60 másodpercig");
});

test("an ordinary session shows no banner", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => json({ actor, csrf_token: "c" })));
  render(
    <QueryClientProvider client={new QueryClient()}>
      <BreakGlassBanner />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
  expect(screen.queryByRole("status")).toBeNull();
});
