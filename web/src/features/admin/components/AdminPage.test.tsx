// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { AdminPage } from "./AdminPage";

const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200, headers: { "Content-Type": "application/json" } });

function show(role: string, ownerOnly = false) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => json({ actor: { id: "me", kind: "human", handle: "me", display_name: "Me", role, state: "active", created_at: "2026-11-01T00:00:00Z" } })),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <AdminPage icon={<span />} title="Oldal" ownerOnly={ownerOnly}>
          <p>titkos tartalom</p>
        </AdminPage>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

test.each([
  ["owner", false, true],
  ["admin", false, true],
  ["member", false, false],
  ["observer", false, false],
  ["owner", true, true],
  ["admin", true, false],
])("%s on an owner-only=%s page: content %s", async (role, ownerOnly, visible) => {
  show(role, ownerOnly);
  await screen.findByRole("heading", { name: "Oldal" });
  expect(screen.queryByText("titkos tartalom") !== null).toBe(visible);
  expect(screen.queryByText("Ez az oldal nem található") !== null).toBe(!visible);
});
