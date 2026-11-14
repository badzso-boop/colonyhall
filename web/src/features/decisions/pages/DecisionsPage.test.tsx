// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import DecisionsPage from "./DecisionsPage";

const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });

test("decisions with rationale, superseded marks and a search", async () => {
  const asked: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      asked.push(u);
      return json({
        decisions: [
          { id: "1", number: 2, formatted_number: "D-2", workspace_id: "w", author_id: "a", channel_id: "c", thread_root_id: "t", text: "Postgres 16", rationale: "Stabil.", supersedes: 1, created_at: "2026-10-30T08:00:00Z" },
          { id: "0", number: 1, formatted_number: "D-1", workspace_id: "w", author_id: "a", channel_id: "c", text: "SQLite", rationale: "", superseded_by: 2, created_at: "2026-10-29T08:00:00Z" },
        ],
      });
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <DecisionsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("Postgres 16")).toBeTruthy();
  expect(screen.getByText("Felülírja: D-1")).toBeTruthy();
  expect(screen.getByText("Felülírta: D-2")).toBeTruthy();
  expect(screen.getAllByRole("link", { name: "Megnyitás a beszélgetésben" })[0]!.getAttribute("href")).toBe("/c/c?thread=t");
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "postgres" } });
  await waitFor(() => expect(asked.at(-1)!.searchParams.get("query")).toBe("postgres"));
  vi.unstubAllGlobals();
});
